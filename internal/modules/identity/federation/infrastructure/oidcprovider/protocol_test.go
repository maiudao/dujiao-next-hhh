package oidcprovider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	federationapp "github.com/dujiao-next/internal/modules/identity/federation/application"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
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

type protocolFixture struct {
	store    *federationstore.Store
	storage  *Storage
	provider *op.Provider
	server   *httptest.Server
	client   *http.Client
	binding  BrowserBinding
	db       *gorm.DB
	auth     *federationapp.Service
	config   *config.Config
}

func secret(t *testing.T) string {
	t.Helper()
	value, err := federationstore.Secret()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func newProtocolFixture(t *testing.T) *protocolFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "identity.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	// Legacy login evidence deliberately must not become email_verified=true.
	now := time.Now()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("fixture-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := userdomain.User{ID: 1, Email: "fixture@example.invalid", PasswordHash: string(passwordHash), DisplayName: "Fixture", Status: constants.UserStatusActive, EmailVerifiedAt: &now}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := New(store, []ClientConfig{
		{ID: "main", Secret: strings.Repeat("m", 32), RedirectURI: "https://main.example.invalid/member/callback", PostLogoutRedirectURI: "https://main.example.invalid/logout/complete", LoginURL: "https://main.example.invalid/login"},
		{ID: "shop", Secret: strings.Repeat("s", 32), RedirectURI: "https://shop.example.invalid/member/callback", PostLogoutRedirectURI: "https://shop.example.invalid/logout/complete", LoginURL: "https://shop.example.invalid/login"},
	}, key, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	t.Cleanup(server.Close)
	issuer := "http://" + server.Listener.Addr().String()
	var cryptoKey [32]byte
	if _, err := rand.Read(cryptoKey[:]); err != nil {
		t.Fatal(err)
	}
	provider, err := op.NewProvider(&op.Config{CryptoKey: cryptoKey, CryptoKeyId: "test-only", CodeMethodS256: true}, storage, op.StaticIssuer(issuer), op.WithAllowInsecure())
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		binding := BrowserBinding{}
		if cookie, err := r.Cookie("browser"); err == nil {
			binding.Browser = cookie.Value
		}
		if cookie, err := r.Cookie("csrf"); err == nil {
			binding.CSRF = cookie.Value
		}
		if cookie, err := r.Cookie("session"); err == nil {
			binding.SessionCookie = cookie.Value
		}
		provider.ServeHTTP(w, r.WithContext(WithBrowser(r.Context(), binding)))
	})
	server.Start()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cfg := &config.Config{App: config.AppConfig{SecretKey: "synthetic-protocol-totp-key"}}
	password := userauthapp.NewService(cfg, userstore.New(db), nil, nil, nil, nil, nil)
	auth, err := federationapp.New(store, password, shopauth.SecondFactor{Config: cfg, Store: store}, time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &protocolFixture{store: store, storage: storage, provider: provider, server: server, client: client, binding: BrowserBinding{Browser: secret(t), CSRF: secret(t)}, db: db, auth: auth, config: cfg}
}

func (f *protocolFixture) party(t *testing.T, clientID, nonce string) rp.RelyingParty {
	t.Helper()
	registered := f.storage.clients[clientID].config
	party, err := rp.NewRelyingPartyOIDC(t.Context(), f.server.URL, clientID, registered.Secret, registered.RedirectURI, []string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}, rp.WithHTTPClient(f.client), rp.WithVerifierOpts(rp.WithNonce(func(context.Context) string { return nonce }), rp.WithSupportedSigningAlgorithms("RS256")))
	if err != nil {
		t.Fatal(err)
	}
	return party
}

func (f *protocolFixture) get(t *testing.T, target string, binding BrowserBinding) (*url.URL, int) {
	t.Helper()
	if strings.HasPrefix(target, "/") {
		target = f.server.URL + target
	}
	r, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"browser": binding.Browser, "csrf": binding.CSRF, "session": binding.SessionCookie} {
		if value != "" {
			r.AddCookie(&http.Cookie{Name: name, Value: value})
		}
	}
	response, err := f.client.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return location, response.StatusCode
}

func (f *protocolFixture) begin(t *testing.T, party rp.RelyingParty, state, nonce, verifier string) string {
	t.Helper()
	target := party.OAuthConfig().AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier))
	location, status := f.get(t, target, f.binding)
	if status != http.StatusFound || location.Query().Get("authRequestID") == "" {
		t.Fatalf("authorization did not reach bound login: status=%d", status)
	}
	return location.Query().Get("authRequestID")
}

func (f *protocolFixture) authenticateFixture(t *testing.T, id string) {
	t.Helper()
	result, err := f.auth.Password(t.Context(), federationapp.Binding{RequestID: id, ClientID: "main", Browser: f.binding.Browser, CSRF: f.binding.CSRF}, "fixture@example.invalid", "fixture-password", false)
	if err != nil || result.RequiresSecondFactor || result.Cookie == "" {
		t.Fatalf("real password login failed: %v", err)
	}
	f.binding.SessionCookie = result.Cookie
}

func TestRealTwoFactorAuthenticationReachesVerifiedOIDCToken(t *testing.T) {
	f := newProtocolFixture(t)
	totpService := usertotpapp.NewService(f.config, userstore.New(f.db), nil)
	setup, err := totpService.Setup(1)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := totpService.Enable(1, code); err != nil {
		t.Fatal(err)
	}
	nonce, verifier := secret(t), secret(t)
	party := f.party(t, "main", nonce)
	id := f.begin(t, party, secret(t), nonce, verifier)
	binding := federationapp.Binding{RequestID: id, ClientID: "main", Browser: f.binding.Browser, CSRF: f.binding.CSRF}
	result, err := f.auth.Password(t.Context(), binding, "fixture@example.invalid", "fixture-password", false)
	if err != nil || !result.RequiresSecondFactor || result.Cookie != "" {
		t.Fatalf("password did not require TOTP: %v", err)
	}
	location, _ := f.get(t, op.AuthCallbackURL(f.provider)(t.Context(), id), f.binding)
	if location.Query().Get("code") != "" {
		t.Fatal("authorization code issued before second factor")
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, "invalid", ""); err == nil {
		t.Fatal("invalid TOTP passed")
	}
	code, err = totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err = f.auth.SecondFactor(t.Context(), binding, code, "")
	if err != nil {
		t.Fatal(err)
	}
	f.binding.SessionCookie = result.Cookie
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), f.code(t, id), party, rp.WithCodeVerifier(verifier))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(tokens.IDTokenClaims)
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		AMR []string `json:"amr"`
	}
	if err := json.Unmarshal(data, &claims); err != nil {
		t.Fatal(err)
	}
	if strings.Join(claims.AMR, " ") != "pwd otp" || tokens.IDTokenClaims.SessionID == "" {
		t.Fatal("verified ID token lost real second-factor authentication")
	}
}

func (f *protocolFixture) code(t *testing.T, id string) string {
	t.Helper()
	location, status := f.get(t, op.AuthCallbackURL(f.provider)(t.Context(), id), f.binding)
	if status != http.StatusFound || location.Query().Get("code") == "" {
		t.Fatalf("authenticated callback did not issue code: status=%d", status)
	}
	return location.Query().Get("code")
}

func TestDurableProviderExchangesBothClientsAndRevokesCurrentBrowser(t *testing.T) {
	f := newProtocolFixture(t)
	state, nonce, verifier := secret(t), secret(t), secret(t)
	main := f.party(t, "main", nonce)
	id := f.begin(t, main, state, nonce, verifier)
	if _, err := f.storage.AuthRequestByID(context.Background(), id); err == nil {
		t.Fatal("request ID alone bypassed browser binding")
	}
	f.authenticateFixture(t, id)
	code := f.code(t, id)
	if _, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), code, main, rp.WithCodeVerifier(secret(t))); err == nil {
		t.Fatal("wrong PKCE verifier accepted")
	}
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), code, main, rp.WithCodeVerifier(verifier))
	if err != nil {
		t.Fatal(err)
	}
	if tokens.IDTokenClaims.GetSubject() == "1" || tokens.IDTokenClaims.GetSubject() == "fixture@example.invalid" || tokens.IDTokenClaims.SessionID == "" {
		t.Fatal("identity is not an opaque subject with sid")
	}
	if bool(tokens.IDTokenClaims.EmailVerified) {
		t.Fatal("legacy email timestamp became authoritative email proof")
	}
	if _, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), code, main, rp.WithCodeVerifier(verifier)); err == nil {
		t.Fatal("authorization code replay accepted")
	}
	// A separate relying party reuses the same authority browser session.
	nonce2, verifier2 := secret(t), secret(t)
	shop := f.party(t, "shop", nonce2)
	id2 := f.begin(t, shop, secret(t), nonce2, verifier2)
	request, err := f.storage.AuthRequestByID(WithBrowser(t.Context(), f.binding), id2)
	if err != nil || !request.Done() {
		t.Fatalf("existing browser session did not resume: %v", err)
	}
	shopTokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), f.code(t, id2), shop, rp.WithCodeVerifier(verifier2))
	if err != nil {
		t.Fatal(err)
	}
	if shopTokens.IDTokenClaims.GetSubject() != tokens.IDTokenClaims.GetSubject() || shopTokens.IDTokenClaims.SessionID != tokens.IDTokenClaims.SessionID {
		t.Fatal("relying parties diverged on identity/session")
	}
	userinfo := func(token string) int {
		r, err := http.NewRequest(http.MethodGet, f.server.URL+"/userinfo", nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		response, err := f.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode == http.StatusOK {
			var info oidc.UserInfo
			if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
				t.Fatal(err)
			}
			if info.Subject != tokens.IDTokenClaims.GetSubject() {
				t.Fatal("userinfo identity mismatch")
			}
		}
		return response.StatusCode
	}
	if status := userinfo(tokens.AccessToken); status != http.StatusOK {
		t.Fatalf("userinfo failed: %d", status)
	}
	var metadata struct {
		EndSession    string `json:"end_session_endpoint"`
		Introspection string `json:"introspection_endpoint"`
	}
	response, err := f.client.Get(f.server.URL + "/.well-known/openid-configuration")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	introspect := func(clientID, token string) bool {
		body := url.Values{"token": {token}}
		r, err := http.NewRequest(http.MethodPost, metadata.Introspection, strings.NewReader(body.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.SetBasicAuth(clientID, f.storage.clients[clientID].config.Secret)
		response, err := f.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result oidc.IntrospectionResponse
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result.Active
	}
	if !introspect("main", tokens.AccessToken) || !introspect("shop", shopTokens.AccessToken) {
		t.Fatal("live token introspection failed")
	}
	if introspect("shop", tokens.AccessToken) {
		t.Fatal("client inspected another client's token")
	}
	// Another browser belonging to the same account is outside current-browser
	// logout. It must keep its independent session when this browser signs out.
	originalBinding := f.binding
	f.binding = BrowserBinding{Browser: secret(t), CSRF: secret(t)}
	otherNonce, otherVerifier := secret(t), secret(t)
	otherParty := f.party(t, "main", otherNonce)
	otherID := f.begin(t, otherParty, secret(t), otherNonce, otherVerifier)
	f.authenticateFixture(t, otherID)
	otherCookie := f.binding.SessionCookie
	f.binding = originalBinding
	// The library verifies signature, issuer and logout redirect first; the
	// adapter additionally binds that hint's sid to the real current browser.
	logout := &op.EndSessionRequest{UserID: tokens.IDTokenClaims.GetSubject(), ClientID: "main", IDTokenHintClaims: tokens.IDTokenClaims, RedirectURI: "https://main.example.invalid/logout/complete"}
	wrong := f.binding
	wrong.SessionCookie = secret(t)
	if _, err := f.storage.TerminateSessionFromRequest(WithBrowser(t.Context(), wrong), logout); err == nil {
		t.Fatal("foreign browser used logout hint")
	}
	logoutQuery := url.Values{"id_token_hint": {tokens.IDToken}, "client_id": {"main"}, "post_logout_redirect_uri": {f.storage.clients["main"].config.PostLogoutRedirectURI}, "state": {secret(t)}}
	location, status := f.get(t, metadata.EndSession+"?"+logoutQuery.Encode(), f.binding)
	if status != http.StatusFound || location.Scheme != "https" || location.Host != "main.example.invalid" || location.Path != "/logout/complete" || location.Query().Get("state") != logoutQuery.Get("state") {
		t.Fatalf("standard logout failed: status=%d", status)
	}
	if status := userinfo(tokens.AccessToken); status == http.StatusOK {
		t.Fatal("main token survived browser logout")
	}
	if status := userinfo(shopTokens.AccessToken); status == http.StatusOK {
		t.Fatal("shop token survived browser logout")
	}
	if introspect("main", tokens.AccessToken) || introspect("shop", shopTokens.AccessToken) {
		t.Fatal("introspection accepted token after logout")
	}
	if _, err := f.store.Session(t.Context(), otherCookie); err != nil {
		t.Fatal("current-browser logout revoked another device")
	}
}

func TestProviderRespectsSilentLoginAndReauthentication(t *testing.T) {
	for _, scenario := range []struct {
		name, prompt                   string
		loggedIn, maxAgeZero, wantDone bool
	}{
		{name: "anonymous silent", prompt: oidc.PromptNone},
		{name: "existing silent", prompt: oidc.PromptNone, loggedIn: true, wantDone: true},
		{name: "explicit login", prompt: oidc.PromptLogin, loggedIn: true},
		{name: "zero maximum age", loggedIn: true, maxAgeZero: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			f := newProtocolFixture(t)
			if scenario.loggedIn {
				nonce, verifier := secret(t), secret(t)
				party := f.party(t, "main", nonce)
				id := f.begin(t, party, secret(t), nonce, verifier)
				f.authenticateFixture(t, id)
			}
			state, nonce, verifier := secret(t), secret(t), secret(t)
			party := f.party(t, "main", nonce)
			options := []oauth2.AuthCodeOption{oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier)}
			if scenario.prompt != "" {
				options = append(options, oauth2.SetAuthURLParam("prompt", scenario.prompt))
			}
			if scenario.maxAgeZero {
				options = append(options, oauth2.SetAuthURLParam("max_age", "0"))
			}
			location, status := f.get(t, party.OAuthConfig().AuthCodeURL(state, options...), f.binding)
			if status != http.StatusFound {
				t.Fatalf("authorization returned status=%d", status)
			}
			if !scenario.loggedIn && scenario.prompt == oidc.PromptNone {
				if location.Query().Get("error") != "login_required" || location.Query().Get("state") != state {
					t.Fatal("anonymous silent request did not return correlated login_required")
				}
				return
			}
			id := location.Query().Get("authRequestID")
			request, err := f.storage.AuthRequestByID(WithBrowser(t.Context(), f.binding), id)
			if err != nil {
				t.Fatal(err)
			}
			if request.Done() != scenario.wantDone {
				t.Fatal("existing session violated prompt/max_age policy")
			}
		})
	}
}

func TestProviderRejectsUnsafeClientAndAuthorizationPolicies(t *testing.T) {
	f := newProtocolFixture(t)
	if _, err := New(f.store, []ClientConfig{{ID: "unsafe", Secret: strings.Repeat("x", 32), RedirectURI: "https://main.example.invalid/*", PostLogoutRedirectURI: "https://main.example.invalid/logout", LoginURL: "http://main.example.invalid/login"}}, f.storage.key, "key"); err == nil {
		t.Fatal("unsafe registration accepted")
	}
	if err := f.storage.AuthorizeClientIDSecret(t.Context(), "main", f.storage.clients["shop"].config.Secret); err == nil {
		t.Fatal("cross-client secret accepted")
	}
	for _, method := range []oidc.CodeChallengeMethod{"", oidc.CodeChallengeMethodPlain, "unsupported"} {
		protocol := &oidc.AuthRequest{ClientID: "main", RedirectURI: f.storage.clients["main"].config.RedirectURI, ResponseType: oidc.ResponseTypeCode, State: secret(t), Nonce: secret(t), Scopes: oidc.SpaceDelimitedArray{oidc.ScopeOpenID}, CodeChallenge: secret(t), CodeChallengeMethod: method}
		if _, err := f.storage.CreateAuthRequest(WithBrowser(t.Context(), f.binding), protocol, ""); err == nil {
			t.Fatal("non-S256 request accepted")
		}
	}
}
