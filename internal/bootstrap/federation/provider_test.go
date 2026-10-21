package federation

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/config"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	federationhttp "github.com/dujiao-next/internal/modules/identity/federation/transport/http"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixtureCaptcha struct{}

func (fixtureCaptcha) Verify(string, captchahttp.CaptchaPayloadRequest, string) error { return nil }

func providerFixture(t *testing.T) (*config.Config, Dependencies) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "identity.db")), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "key.pem")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0600); err != nil {
		t.Fatal(err)
	}
	cryptoKey, err := federationstore.Secret()
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{IdentityProvider: config.IdentityProviderConfig{Enabled: true, Issuer: "https://identity.example.invalid/api/identity", SigningKeyFile: keyFile, SigningKeyID: "fixture-key", CryptoKey: cryptoKey, SessionHours: 1, RememberHours: 168, Clients: []config.IdentityClientConfig{{ID: "main", Secret: strings.Repeat("m", 32), RedirectURI: "https://main.example.invalid/member/callback", PostLogoutRedirectURI: "https://main.example.invalid/logout/complete", FormURL: "https://main.example.invalid/login"}}}}
	deps := Dependencies{DB: db, Auth: userauthapp.NewService(cfg, nil, nil, nil, nil, nil, nil), Captcha: fixtureCaptcha{}, ClientIP: func(*http.Request) string { return "127.0.0.1" }, RateLimit: func(*http.Request, string, string) error { return nil }, Logger: slog.New(slog.NewJSONHandler(io.Discard, nil)), Audit: func(*http.Request, federationhttp.LoginEvent) {}}
	return cfg, deps
}

func TestDisabledProviderNeedsNoDatabaseOrSecrets(t *testing.T) {
	server, err := New(&config.Config{}, Dependencies{})
	if err != nil || server != nil {
		t.Fatal("disabled provider initialized dependencies")
	}
}

func TestProviderRequiresExplicitSchemaAndFailClosedConfiguration(t *testing.T) {
	cfg, deps := providerFixture(t)
	if _, err := New(cfg, deps); err == nil {
		t.Fatal("missing schema accepted")
	}
	if deps.DB.Migrator().HasTable(&federationdomain.Subject{}) {
		t.Fatal("initialization created schema")
	}
	if err := federationstore.New(deps.DB).Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	server, err := New(cfg, deps)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, cfg.IdentityProvider.Issuer+"/.well-known/openid-configuration", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), cfg.IdentityProvider.Issuer) {
		t.Fatal("configured provider discovery unavailable")
	}
	for _, mutation := range []string{"lifetime", "crypto", "client", "key", "captcha", "audit", "limiter"} {
		t.Run(mutation, func(t *testing.T) {
			copyConfig, copyDeps := *cfg, deps
			switch mutation {
			case "lifetime":
				copyConfig.IdentityProvider.RememberHours = 721
			case "crypto":
				copyConfig.IdentityProvider.CryptoKey = "invalid"
			case "client":
				copyConfig.IdentityProvider.Clients = nil
			case "key":
				copyConfig.IdentityProvider.SigningKeyFile = filepath.Join(t.TempDir(), "absent.pem")
			case "captcha":
				copyDeps.Captcha = nil
			case "audit":
				copyDeps.Audit = nil
			case "limiter":
				copyDeps.RateLimit = nil
			}
			if _, err := New(&copyConfig, copyDeps); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
		})
	}
}

func TestProtocolLoggerDiscardsArbitraryMessagesAndSecretAttributes(t *testing.T) {
	core, records := observer.New(zap.DebugLevel)
	log := Logger(zap.New(core), func(context.Context) string { return "fixture-request" })
	log.With("password", "fixture-secret").WithGroup("headers").Error("URL with fixture-secret", "error", "Bearer fixture-secret", "cookie", "fixture-secret")
	log.Info("identity authentication completed", "client", "main", "user_id", uint(1), "token", "fixture-secret")
	for _, record := range records.All() {
		if strings.Contains(record.Message, "fixture-secret") {
			t.Fatal("secret leaked in message")
		}
		for _, field := range record.Context {
			if strings.Contains(field.String, "fixture-secret") {
				t.Fatal("secret leaked in metadata")
			}
		}
	}
	if records.Len() != 2 || records.All()[0].Message != "identity protocol event" || records.All()[1].ContextMap()["client"] != "main" {
		t.Fatal("safe diagnostic metadata lost")
	}
}
