package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/bootstrap/federation"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/logger"
	auditlogapp "github.com/dujiao-next/internal/modules/auditlog/application"
	auditlogdomain "github.com/dujiao-next/internal/modules/auditlog/domain"
	auditstore "github.com/dujiao-next/internal/modules/auditlog/infrastructure/gormstore"
	captchaapp "github.com/dujiao-next/internal/modules/captcha/application"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/platform/database/gormdb"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestIdentityRoutesDisabledAndMissingDependencies(t *testing.T) {
	engine := gin.New()
	if err := registerFederationRoutes(engine, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	if len(engine.Routes()) != 0 {
		t.Fatal("disabled provider registered endpoints")
	}
	if err := registerFederationRoutes(engine, &config.Config{IdentityProvider: config.IdentityProviderConfig{Enabled: true}}, nil); err == nil {
		t.Fatal("missing real services accepted")
	}
}

func TestActualGinIdentityLoginUsesTrustedIPAuditAndSharedLimiter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "gin-identity.db")), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	previousDB, previousLog, previousSlog := gormdb.DB, logger.L, slog.Default()
	t.Cleanup(func() { gormdb.DB, logger.L = previousDB, previousLog; slog.SetDefault(previousSlog) })
	gormdb.DB = db
	core, logs := observer.New(zap.DebugLevel)
	logger.L = zap.New(core)
	if err := db.AutoMigrate(&userdomain.User{}, &auditlogdomain.UserLoginLog{}); err != nil {
		t.Fatal(err)
	}
	if err := federationstore.New(db).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	password := "synthetic-gin-password"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&userdomain.User{ID: 1, Email: "fixture@example.invalid", PasswordHash: string(hash), Status: "active", EmailVerifiedAt: &now}).Error; err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(nil)
	t.Cleanup(server.Close)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	secret := func() string {
		t.Helper()
		value, err := federationstore.Secret()
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	issuer := "https://" + server.Listener.Addr().String() + federation.Path
	cfg := &config.Config{IdentityProvider: config.IdentityProviderConfig{Enabled: true, Issuer: issuer, SigningKeyFile: keyFile, SigningKeyID: "gin-fixture", CryptoKey: secret(), SessionHours: 1, RememberHours: 168, Clients: []config.IdentityClientConfig{{ID: "main", Secret: strings.Repeat("m", 32), RedirectURI: "https://main.example.invalid/member/callback", PostLogoutRedirectURI: "https://main.example.invalid/logout/complete", FormURL: "https://main.example.invalid/login"}}}, Security: config.SecurityConfig{LoginRateLimit: config.LoginRateLimitConfig{WindowSeconds: 60, MaxAttempts: 2, BlockSeconds: 60}}}
	c := &container.Container{UserAuthService: userauthapp.NewService(cfg, userstore.New(db), nil, nil, nil, nil, nil), CaptchaService: captchaapp.NewService(nil, config.CaptchaConfig{}, nil), UserLoginLogService: auditlogapp.NewUserLoginService(auditstore.NewUserLoginStore(db))}
	engine := gin.New()
	if err := configureTrustedProxies(engine, nil); err != nil {
		t.Fatal(err)
	}
	engine.Use(middleware.RequestIDMiddleware(), middleware.RecoveryMiddleware(), middleware.LoggerMiddleware(logger.L))
	if err := registerFederationRoutes(engine, cfg, c); err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = engine
	server.StartTLS()
	rpc := server.Client()
	party, err := rp.NewRelyingPartyOIDC(context.Background(), issuer, "main", strings.Repeat("m", 32), "https://main.example.invalid/member/callback", []string{"openid"}, rp.WithHTTPClient(rpc))
	if err != nil {
		t.Fatal(err)
	}
	browser := server.Client()
	browser.Jar, _ = cookiejar.New(nil)
	browser.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	getLocation := func(target string) string {
		t.Helper()
		response, err := browser.Get(target)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		if response.StatusCode != http.StatusFound && response.StatusCode != http.StatusSeeOther {
			t.Fatalf("redirect status %d", response.StatusCode)
		}
		location, err := response.Location()
		if err != nil {
			t.Fatal(err)
		}
		return location.String()
	}
	state, nonce, verifier := secret(), secret(), secret()
	// A real library rejection includes protocol input in its native slog
	// attributes. Exercise that path under the actual startup-installed sink.
	_ = getLocation(party.OAuthConfig().AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.SetAuthURLParam("code_challenge", verifier), oauth2.SetAuthURLParam("code_challenge_method", "plain")))
	continued := getLocation(party.OAuthConfig().AuthCodeURL(state, oauth2.SetAuthURLParam("nonce", nonce), oauth2.S256ChallengeOption(verifier)))
	formURL, err := url.Parse(getLocation(continued))
	if err != nil {
		t.Fatal(err)
	}
	id := formURL.Query().Get("authRequestID")
	post := func(password string) int {
		t.Helper()
		form := url.Values{"authRequestID": {id}, "csrf": {state}, "email": {"fixture@example.invalid"}, "password": {password}}
		request, err := http.NewRequest(http.MethodPost, issuer+"/login/password", strings.NewReader(form.Encode()))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("Origin", "https://main.example.invalid")
		request.Header.Set("X-Forwarded-For", "203.0.113.99")
		response, err := browser.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}
	if post("wrong-password") != http.StatusSeeOther || post(password) != http.StatusSeeOther {
		t.Fatal("actual login handler did not complete")
	}
	if post(password) != http.StatusTooManyRequests {
		t.Fatal("existing application limiter bypassed")
	}
	var audits []auditlogdomain.UserLoginLog
	if err := db.Order("id").Find(&audits).Error; err != nil || len(audits) != 2 {
		t.Fatal("login audit missing or throttled request reached auth")
	}
	if audits[0].Status != "failed" || audits[1].Status != "success" || audits[1].UserID != 1 || audits[1].LoginSource != "identity_main" || audits[1].ClientIP == "203.0.113.99" || audits[1].ClientIP == "" {
		t.Fatal("identity audit or trusted proxy contract failed")
	}
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, password) || strings.Contains(entry.Message, state) {
			t.Fatal("credential leaked in log message")
		}
		for _, field := range entry.Context {
			if strings.Contains(field.String, password) || strings.Contains(field.String, state) {
				t.Fatal("credential leaked in request/protocol logs")
			}
		}
	}
}

func TestIdentityRecoveryDoesNotLogPanicCredentials(t *testing.T) {
	previous := logger.L
	t.Cleanup(func() { logger.L = previous })
	core, logs := observer.New(zap.DebugLevel)
	logger.L = zap.New(core)
	engine := gin.New()
	engine.Use(middleware.RequestIDMiddleware(), middleware.RecoveryMiddleware(), middleware.LoggerMiddleware(logger.L))
	engine.GET("/api/identity/panic", identityRecovery, func(*gin.Context) { panic("password=synthetic-panic-secret") })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://shop.example.invalid/api/identity/panic", nil))
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "synthetic-panic-secret") {
		t.Fatal("panic response leaked credentials")
	}
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "synthetic-panic-secret") {
			t.Fatal("panic message leaked")
		}
		for _, field := range entry.Context {
			if strings.Contains(field.String, "synthetic-panic-secret") {
				t.Fatal("panic metadata leaked")
			}
		}
	}
}
