package federationhttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	federationapp "github.com/dujiao-next/internal/modules/identity/federation/application"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/oidcprovider"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/shopauth"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	usertotpapp "github.com/dujiao-next/internal/modules/identity/userauth/totp/application"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixtureCaptcha struct {
	calls int
	deny  bool
}

type fixtureClientChallenge struct {
	receipt, id, state string
	calls              int
}

func (c *fixtureClientChallenge) Verify(_ context.Context, id, state, receipt, email string) error {
	c.calls++
	if id != c.id || state != c.state || receipt != c.receipt || c.receipt == "" || email != "fixture@example.invalid" {
		return errors.New("denied")
	}
	c.receipt = ""
	return nil
}

func TestClientChallengeCannotFallBackToNativeCaptcha(t *testing.T) {
	f := newBrowserFixture(t)
	nonce, verifier, state := randomSecret(t), randomSecret(t), randomSecret(t)
	party := f.party(t, "main", nonce)
	id, _ := f.begin(t, party, state, nonce, verifier)
	check := &fixtureClientChallenge{id: id, state: state, receipt: randomSecret(t)}
	f.handler.options.Challenges = map[string]ChallengeVerifier{"main": check}
	form := url.Values{"authRequestID": {id}, "csrf": {state}, "email": {"fixture@example.invalid"}, "password": {"fixture-password"}, "captcha_code": {"passed"}}
	_, status, _ := f.post(t, "/login/password", "https://main.example.invalid", form)
	if status != http.StatusForbidden || check.calls != 0 || f.captcha.calls != 0 {
		t.Fatal("missing receipt fell back to native captcha")
	}
	form.Set("challenge_receipt", randomSecret(t))
	_, status, _ = f.post(t, "/login/password", "https://main.example.invalid", form)
	if status != http.StatusForbidden || f.captcha.calls != 0 {
		t.Fatal("wrong receipt fell back")
	}
	form.Set("challenge_receipt", check.receipt)
	// Authority browser binding must be checked before the receipt is redeemed.
	old := f.browser.Jar
	f.browser.Jar, _ = cookiejar.New(nil)
	before := check.calls
	_, status, _ = f.post(t, "/login/password", "https://main.example.invalid", form)
	if status != http.StatusForbidden || check.calls != before {
		t.Fatal("unbound browser redeemed receipt")
	}
	f.browser.Jar = old
	_, status, cookies := f.post(t, "/login/password", "https://main.example.invalid", form)
	if status != http.StatusSeeOther || f.captcha.calls != 0 {
		t.Fatal("verified client challenge did not reach real password login")
	}
	issued := false
	for _, cookie := range cookies {
		if cookie.Name == SessionCookieName {
			issued = true
		}
	}
	if !issued {
		t.Fatal("real authority session was not issued")
	}
}

func (c *fixtureCaptcha) Verify(_ string, payload captchahttp.CaptchaPayloadRequest, _ string) error {
	c.calls++
	if c.deny || payload.CaptchaCode != "passed" {
		return errors.New("captcha denied")
	}
	return nil
}

type browserFixture struct {
	server       *httptest.Server
	browser, rpc *http.Client
	handler      *Server
	store        *federationstore.Store
	db           *gorm.DB
	issuer       string
	logs         *bytes.Buffer
	captcha      *fixtureCaptcha
	denyRate     bool
	config       *config.Config
}

func randomSecret(t *testing.T) string {
	t.Helper()
	value, err := federationstore.Secret()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newBrowserFixture(t *testing.T) *browserFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "browser.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	if err := db.AutoMigrate(&userdomain.User{}); err != nil {
		t.Fatal(err)
	}
	store := federationstore.New(db)
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("fixture-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	user := userdomain.User{ID: 1, Email: "fixture@example.invalid", PasswordHash: string(hash), Status: "active", EmailVerifiedAt: &now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	t.Cleanup(server.Close)
	issuer := "https://" + server.Listener.Addr().String() + "/api/identity"
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := oidcprovider.New(store, []oidcprovider.ClientConfig{
		{ID: "main", Secret: strings.Repeat("m", 32), RedirectURI: "https://main.example.invalid/member/callback", PostLogoutRedirectURI: "https://main.example.invalid/logout/complete", LoginURL: issuer + "/login/continue"},
		{ID: "shop", Secret: strings.Repeat("s", 32), RedirectURI: "https://shop.example.invalid/member/callback", PostLogoutRedirectURI: "https://shop.example.invalid/logout/complete", LoginURL: issuer + "/login/continue"},
	}, key, "browser-test-key")
	if err != nil {
		t.Fatal(err)
	}
	var cryptoKey [32]byte
	if _, err := rand.Read(cryptoKey[:]); err != nil {
		t.Fatal(err)
	}
	provider, err := op.NewProvider(&op.Config{CryptoKey: cryptoKey, CryptoKeyId: "test-only", CodeMethodS256: true}, protocol, op.StaticIssuer(issuer))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{SecretKey: "synthetic-browser-totp-encryption-key"}}
	password := userauthapp.NewService(cfg, userstore.New(db), nil, nil, nil, nil, nil)
	auth, err := federationapp.New(store, password, shopauth.SecondFactor{Config: cfg, Store: store}, time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	logs := &bytes.Buffer{}
	captcha := &fixtureCaptcha{}
	f := &browserFixture{server: server, store: store, db: db, issuer: issuer, logs: logs, captcha: captcha, config: cfg}
	handler, err := New(Options{Issuer: issuer, Provider: provider, Protocol: protocol, Store: store, Auth: auth, Clients: []UIClient{{ID: "main", FormURL: "https://main.example.invalid/login"}, {ID: "shop", FormURL: "https://shop.example.invalid/login"}}, Captcha: captcha, ClientIP: func(*http.Request) string { return "127.0.0.1" }, RateLimit: func(*http.Request, string, string) error {
		if f.denyRate {
			return errors.New("limited")
		}
		return nil
	}, Logger: slog.New(slog.NewJSONHandler(logs, nil))})
	if err != nil {
		t.Fatal(err)
	}
	f.handler = handler
	server.Config.Handler = handler
	server.StartTLS()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	f.browser = server.Client()
	f.browser.Jar = jar
	f.browser.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	f.rpc = server.Client()
	f.rpc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return f
}

func (f *browserFixture) party(t *testing.T, id, nonce string) rp.RelyingParty {
	t.Helper()
	secret := strings.Repeat("m", 32)
	if id == "shop" {
		secret = strings.Repeat("s", 32)
	}
	party, err := rp.NewRelyingPartyOIDC(t.Context(), f.issuer, id, secret, "https://"+id+".example.invalid/member/callback", []string{oidc.ScopeOpenID, oidc.ScopeEmail}, rp.WithHTTPClient(f.rpc), rp.WithVerifierOpts(rp.WithNonce(func(context.Context) string { return nonce }), rp.WithSupportedSigningAlgorithms("RS256")))
	if err != nil {
		t.Fatal(err)
	}
	return party
}
func (f *browserFixture) get(t *testing.T, target string) (*url.URL, int, []*http.Cookie) {
	t.Helper()
	if strings.HasPrefix(target, "/") {
		target = f.server.URL + target
	}
	response, err := f.browser.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return location, response.StatusCode, response.Cookies()
}
func (f *browserFixture) begin(t *testing.T, party rp.RelyingParty, state, nonce, verifier string) (string, *url.URL) {
	t.Helper()
	location, status, cookies := f.get(t, party.OAuthConfig().AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier)))
	if status != http.StatusFound || location.Path != "/api/identity/login/continue" {
		t.Fatalf("authorization did not reach authority continuation: %d", status)
	}
	for _, cookie := range cookies {
		if cookie.Name == BrowserCookieName && (!cookie.Secure || !cookie.HttpOnly || cookie.Domain != "" || cookie.Path != "/" || cookie.SameSite != http.SameSiteLaxMode) {
			t.Fatal("browser cookie is not secure and host-only")
		}
	}
	id := location.Query().Get("authRequestID")
	form, status, _ := f.get(t, location.String())
	if status != http.StatusSeeOther {
		t.Fatalf("bound continuation failed: %d", status)
	}
	return id, form
}
func (f *browserFixture) post(t *testing.T, path, origin string, form url.Values) (*url.URL, int, []*http.Cookie) {
	t.Helper()
	r, err := http.NewRequest(http.MethodPost, f.issuer+path, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	response, err := f.browser.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return location, response.StatusCode, response.Cookies()
}
func passwordForm(id, state string) url.Values {
	return url.Values{"authRequestID": {id}, "csrf": {state}, "email": {"fixture@example.invalid"}, "password": {"fixture-password"}, "captcha_code": {"passed"}}
}

func TestHTTPSBrowserLoginSilentSSOAndCookieLogout(t *testing.T) {
	f := newBrowserFixture(t)
	state, nonce, verifier := randomSecret(t), randomSecret(t), randomSecret(t)
	party := f.party(t, "main", nonce)
	id, form := f.begin(t, party, state, nonce, verifier)
	if form.Host != "main.example.invalid" || form.Path != "/login" || form.Query().Get("state") != state || form.Query().Get("authRequestID") != id {
		t.Fatal("original main form did not receive correlated interaction")
	}
	location, status, cookies := f.post(t, "/login/password", "https://main.example.invalid", passwordForm(id, state))
	if status != http.StatusSeeOther || location.Path != "/api/identity/authorize/callback" {
		t.Fatalf("real password form failed: %d", status)
	}
	found := false
	for _, cookie := range cookies {
		if cookie.Name == SessionCookieName {
			found = true
			if !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 0 {
				t.Fatal("session cookie scope/persistence incorrect")
			}
		}
	}
	if !found {
		t.Fatal("missing authority session cookie")
	}
	callback, status, _ := f.get(t, location.String())
	if status != http.StatusFound || callback.Query().Get("state") != state || callback.Query().Get("code") == "" {
		t.Fatalf("browser callback failed: %d", status)
	}
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), callback.Query().Get("code"), party, rp.WithCodeVerifier(verifier))
	if err != nil {
		t.Fatal(err)
	}
	// A second client uses the authority continuation and never displays a form.
	state2, nonce2, verifier2 := randomSecret(t), randomSecret(t), randomSecret(t)
	shop := f.party(t, "shop", nonce2)
	id2, continueURL := f.begin(t, shop, state2, nonce2, verifier2)
	if id2 == "" || continueURL.Path != "/api/identity/authorize/callback" {
		t.Fatal("existing browser did not silently continue SSO")
	}
	callback2, status, _ := f.get(t, continueURL.String())
	if status != http.StatusFound {
		t.Fatal("second relying party callback failed")
	}
	shopTokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), callback2.Query().Get("code"), shop, rp.WithCodeVerifier(verifier2))
	if err != nil {
		t.Fatal(err)
	}
	if tokens.IDTokenClaims.GetSubject() != shopTokens.IDTokenClaims.GetSubject() || tokens.IDTokenClaims.SessionID != shopTokens.IDTokenClaims.SessionID {
		t.Fatal("browser SSO identity diverged")
	}
	logoutForm := url.Values{"id_token_hint": {tokens.IDToken}, "client_id": {"main"}, "post_logout_redirect_uri": {"https://main.example.invalid/logout/complete"}, "state": {randomSecret(t)}}
	if _, status, _ := f.post(t, f.handler.options.Provider.EndSessionEndpoint().Relative(), "https://attacker.example.invalid", logoutForm); status != http.StatusForbidden {
		t.Fatal("foreign origin logged out current browser")
	}
	location, status, cookies = f.post(t, f.handler.options.Provider.EndSessionEndpoint().Relative(), "https://main.example.invalid", logoutForm)
	if status != http.StatusFound || location.Host != "main.example.invalid" {
		t.Fatalf("browser logout failed: %d", status)
	}
	cleared := false
	for _, cookie := range cookies {
		if cookie.Name == SessionCookieName && cookie.MaxAge < 0 && cookie.Domain == "" {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("successful logout did not clear host-only cookie")
	}
	if strings.Contains(f.logs.String(), "fixture-password") || strings.Contains(f.logs.String(), state) || strings.Contains(f.logs.String(), tokens.IDToken) {
		t.Fatal("authentication logs leaked a secret")
	}
}

func TestHTTPSReauthenticationRotatesOnlyCurrentBrowser(t *testing.T) {
	f := newBrowserFixture(t)
	login := func(prompt string) *oidc.Tokens[*oidc.IDTokenClaims] {
		t.Helper()
		state, nonce, verifier := randomSecret(t), randomSecret(t), randomSecret(t)
		party := f.party(t, "main", nonce)
		location, status, _ := f.get(t, party.OAuthConfig().AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("prompt", prompt)))
		if status != http.StatusFound {
			t.Fatal("authorization did not start")
		}
		id := location.Query().Get("authRequestID")
		location, status, _ = f.get(t, location.String())
		if status != http.StatusSeeOther || location.Path != "/login" {
			t.Fatal("reauthentication did not require original form")
		}
		location, status, _ = f.post(t, "/login/password", "https://main.example.invalid", passwordForm(id, state))
		if status != http.StatusSeeOther {
			t.Fatal("reauthentication failed")
		}
		callback, status, _ := f.get(t, location.String())
		if status != http.StatusFound {
			t.Fatal("authorization callback failed")
		}
		tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), callback.Query().Get("code"), party, rp.WithCodeVerifier(verifier))
		if err != nil {
			t.Fatal(err)
		}
		return tokens
	}
	first := login("login")
	otherJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	currentJar := f.browser.Jar
	f.browser.Jar = otherJar
	other := login("login")
	f.browser.Jar = currentJar
	rotated := login("login")
	if first.IDTokenClaims.SessionID == rotated.IDTokenClaims.SessionID || first.IDTokenClaims.Subject != rotated.IDTokenClaims.Subject {
		t.Fatal("reauthentication did not rotate sid for same subject")
	}
	if _, err := f.store.SessionByID(t.Context(), first.IDTokenClaims.SessionID); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("old browser session remains active")
	}
	for _, sid := range []string{rotated.IDTokenClaims.SessionID, other.IDTokenClaims.SessionID} {
		if _, err := f.store.SessionByID(t.Context(), sid); err != nil {
			t.Fatal("rotation revoked new or other-device session")
		}
	}
}

func TestHTTPSFormsRejectWrongOriginCSRFCookieCaptchaAndSize(t *testing.T) {
	f := newBrowserFixture(t)
	state, nonce, verifier := randomSecret(t), randomSecret(t), randomSecret(t)
	party := f.party(t, "main", nonce)
	id, _ := f.begin(t, party, state, nonce, verifier)
	for _, origin := range []string{"https://attacker.example.invalid", "null", ""} {
		if _, status, _ := f.post(t, "/login/password", origin, passwordForm(id, state)); status != http.StatusForbidden {
			t.Fatal("foreign/missing origin accepted")
		}
	}
	if _, status, _ := f.post(t, "/login/password", "https://main.example.invalid", passwordForm(id, randomSecret(t))); status != http.StatusForbidden {
		t.Fatal("wrong CSRF accepted")
	}
	if f.captcha.calls != 0 {
		t.Fatal("credential checks ran before binding/origin validation")
	}
	f.captcha.deny = true
	if _, status, _ := f.post(t, "/login/password", "https://main.example.invalid", passwordForm(id, state)); status != http.StatusForbidden {
		t.Fatal("captcha rejection bypassed")
	}
	f.captcha.deny = false
	oversized := passwordForm(id, state)
	oversized.Set("password", strings.Repeat("x", maxBody+1))
	if _, status, _ := f.post(t, "/login/password", "https://main.example.invalid", oversized); status != http.StatusBadRequest {
		t.Fatal("oversized form accepted")
	}
	f.denyRate = true
	if _, status, _ := f.post(t, "/login/password", "https://main.example.invalid", passwordForm(id, state)); status != http.StatusTooManyRequests {
		t.Fatal("shared limiter bypassed")
	}
	f.denyRate = false
	r, err := http.NewRequest(http.MethodPost, f.issuer+"/login/password", strings.NewReader(passwordForm(id, state).Encode()))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://main.example.invalid")
	r.AddCookie(&http.Cookie{Name: BrowserCookieName, Value: randomSecret(t)})
	response, err := f.browser.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatal("duplicate/shadowed browser cookie accepted")
	}
	var sessions int64
	if err := f.db.Model(&federationdomain.BrowserSession{}).Count(&sessions).Error; err != nil || sessions != 0 {
		t.Fatal("rejected HTTP requests created authority sessions")
	}
}

func TestInteractionBridgeNeedsClientStateAndCannotCrossClients(t *testing.T) {
	f := newBrowserFixture(t)
	state, nonce, verifier := randomSecret(t), randomSecret(t), randomSecret(t)
	party := f.party(t, "main", nonce)
	id, _ := f.begin(t, party, state, nonce, verifier)
	query := func(client, secret, proof string) (InteractionResponse, int) {
		data, _ := json.Marshal(map[string]string{"id": id, "state": proof})
		r, err := http.NewRequest(http.MethodPost, f.issuer+"/interaction", bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		r.SetBasicAuth(client, secret)
		response, err := f.rpc.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var output InteractionResponse
		if response.StatusCode == http.StatusOK {
			if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
				t.Fatal(err)
			}
		}
		return output, response.StatusCode
	}
	if _, status := query("main", "wrong", state); status != http.StatusUnauthorized {
		t.Fatal("bridge accepted wrong client secret")
	}
	if _, status := query("shop", strings.Repeat("s", 32), state); status != http.StatusForbidden {
		t.Fatal("bridge crossed client boundary")
	}
	if _, status := query("main", strings.Repeat("m", 32), randomSecret(t)); status != http.StatusForbidden {
		t.Fatal("bridge accepted uncorrelated state")
	}
	output, status := query("main", strings.Repeat("m", 32), state)
	if status != http.StatusOK || output.ID != id || output.State != state || output.Nonce != nonce || output.CodeChallenge != oauth2.S256ChallengeFromVerifier(verifier) || output.CodeChallengeMethod != "S256" || output.FormAction != f.issuer+"/login/password" {
		t.Fatal("bridge lost RP correlation data")
	}
}

func TestHTTPSRealSecondFactorAndCancellation(t *testing.T) {
	f := newBrowserFixture(t)
	verifierService := usertotpapp.NewService(f.config, userstore.New(f.db), nil)
	setup, err := verifierService.Setup(1)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifierService.Enable(1, code); err != nil {
		t.Fatal(err)
	}
	state, nonce, verifier := randomSecret(t), randomSecret(t), randomSecret(t)
	party := f.party(t, "main", nonce)
	id, _ := f.begin(t, party, state, nonce, verifier)
	form := passwordForm(id, state)
	form.Set("remember_me", "true")
	location, status, cookies := f.post(t, "/login/password", "https://main.example.invalid", form)
	if status != http.StatusSeeOther || location.Query().Get("step") != "second_factor" {
		t.Fatal("password bypassed HTTP second-factor step")
	}
	for _, cookie := range cookies {
		if cookie.Name == SessionCookieName {
			t.Fatal("cookie issued before TOTP")
		}
	}
	code, err = totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	location, status, cookies = f.post(t, "/login/second-factor", "https://main.example.invalid", url.Values{"authRequestID": {id}, "csrf": {state}, "code": {code}})
	if status != http.StatusSeeOther || location.Path != "/api/identity/authorize/callback" {
		t.Fatalf("real HTTP TOTP failed: %d", status)
	}
	persistent := false
	for _, cookie := range cookies {
		if cookie.Name == SessionCookieName && cookie.MaxAge > 29*24*60*60 {
			persistent = true
		}
	}
	if !persistent {
		t.Fatal("second factor lost remembered-session selection")
	}
	// Reauthentication creates a distinct pending interaction to cancel.
	state2, nonce2, verifier2 := randomSecret(t), randomSecret(t), randomSecret(t)
	party2 := f.party(t, "main", nonce2)
	authorize := party2.OAuthConfig().AuthCodeURL(state2, oauth2.SetAuthURLParam("nonce", nonce2), oauth2.S256ChallengeOption(verifier2), oauth2.SetAuthURLParam("prompt", "login"))
	continueURL, status, _ := f.get(t, authorize)
	if status != http.StatusFound {
		t.Fatal("reauthentication did not start")
	}
	id2 := continueURL.Query().Get("authRequestID")
	location, status, _ = f.post(t, "/login/cancel", "https://main.example.invalid", url.Values{"authRequestID": {id2}, "csrf": {state2}})
	if status != http.StatusSeeOther || location.Query().Get("error") != "access_denied" || location.Query().Get("state") != state2 {
		t.Fatal("cancellation lost standard error/state")
	}
	if _, err := f.store.Request(t.Context(), id2); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("cancelled request remains usable")
	}
}
