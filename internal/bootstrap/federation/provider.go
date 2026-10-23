// Package federation assembles the real identity provider. It never generates
// production keys or silently creates identity tables during router setup.
package federation

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	federationapp "github.com/dujiao-next/internal/modules/identity/federation/application"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/mainchallenge"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/oidcprovider"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/shopauth"
	federationhttp "github.com/dujiao-next/internal/modules/identity/federation/transport/http"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/zitadel/oidc/v3/pkg/op"
	"gorm.io/gorm"
)

const Path = "/api/identity"

type Dependencies struct {
	DB                  *gorm.DB
	Auth                *userauthapp.Service
	Captcha             federationhttp.CaptchaVerifier
	ClientIP            func(*http.Request) string
	RateLimit           func(*http.Request, string, string) error
	Logger              *slog.Logger
	Audit               func(*http.Request, federationhttp.LoginEvent)
	RegistrationEnabled func() (bool, error)
}

func New(cfg *config.Config, deps Dependencies) (*federationhttp.Server, error) {
	if cfg == nil {
		return nil, errors.New("missing identity configuration")
	}
	settings := cfg.IdentityProvider
	if !settings.Enabled {
		return nil, nil
	}
	issuer, err := url.Parse(settings.Issuer)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.Path != Path || issuer.User != nil || issuer.RawQuery != "" || issuer.Fragment != "" || deps.DB == nil || deps.Auth == nil || deps.Captcha == nil || deps.ClientIP == nil || deps.RateLimit == nil || deps.Logger == nil || deps.Audit == nil {
		return nil, errors.New("incomplete identity provider wiring")
	}
	if settings.SessionHours < 1 || settings.SessionHours > 720 || settings.RememberHours < settings.SessionHours || settings.RememberHours > 720 {
		return nil, errors.New("invalid identity session lifetime")
	}
	cryptoBytes, err := base64.RawURLEncoding.DecodeString(settings.CryptoKey)
	if err != nil || len(cryptoBytes) != 32 || base64.RawURLEncoding.EncodeToString(cryptoBytes) != settings.CryptoKey {
		return nil, errors.New("invalid identity encryption key")
	}
	key, err := signingKey(settings.SigningKeyFile)
	if err != nil {
		return nil, err
	}
	store := federationstore.New(deps.DB)
	clients := make([]oidcprovider.ClientConfig, 0, len(settings.Clients))
	forms := make([]federationhttp.UIClient, 0, len(settings.Clients))
	challenges := make(map[string]federationhttp.ChallengeVerifier)
	for _, client := range settings.Clients {
		formURL, err := url.Parse(client.FormURL)
		if err == nil && (formURL.Hostname() == "www.huihuahui.xyz" || formURL.Hostname() == "huihuahui.xyz") && client.ChallengeURL == "" {
			return nil, errors.New("main-site identity client requires original challenge bridge")
		}
		clients = append(clients, oidcprovider.ClientConfig{ID: client.ID, Secret: client.Secret, RedirectURI: client.RedirectURI, PostLogoutRedirectURI: client.PostLogoutRedirectURI, LoginURL: settings.Issuer + "/login/continue"})
		forms = append(forms, federationhttp.UIClient{ID: client.ID, FormURL: client.FormURL})
		if client.ChallengeURL != "" {
			verifier, err := mainchallenge.New(client.ChallengeURL, client.FormURL, client.ID, client.Secret, nil)
			if err != nil {
				return nil, errors.New("invalid identity challenge bridge")
			}
			challenges[client.ID] = verifier
		}
	}
	protocol, err := oidcprovider.New(store, clients, key, settings.SigningKeyID)
	if err != nil {
		return nil, errors.New("invalid identity provider credentials or clients")
	}
	var cryptoKey [32]byte
	copy(cryptoKey[:], cryptoBytes)
	provider, err := op.NewProvider(&op.Config{CryptoKey: cryptoKey, CryptoKeyId: settings.SigningKeyID, CodeMethodS256: true}, protocol, op.StaticIssuer(settings.Issuer))
	if err != nil {
		return nil, errors.New("identity provider initialization failed")
	}
	auth, err := federationapp.New(store, deps.Auth, shopauth.SecondFactor{Config: cfg, Store: store}, time.Duration(settings.SessionHours)*time.Hour, time.Duration(settings.RememberHours)*time.Hour)
	if err != nil {
		return nil, err
	}
	server, err := federationhttp.New(federationhttp.Options{Issuer: settings.Issuer, Provider: provider, Protocol: protocol, Store: store, Auth: auth, Clients: forms, Captcha: deps.Captcha, Challenges: challenges, ClientIP: deps.ClientIP, RateLimit: deps.RateLimit, Logger: deps.Logger, Audit: deps.Audit, Registration: deps.Auth, RegistrationEnabled: deps.RegistrationEnabled})
	if err != nil {
		return nil, err
	}
	// Explicit deployment migration must have completed before enabling routes.
	// No data or schema writes occur here, including on configuration failures.
	if err := SchemaReady(context.Background(), deps.DB); err != nil {
		return nil, err
	}
	return server, nil
}

func signingKey(path string) (*rsa.PrivateKey, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("identity signing key file is required")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("identity signing key is unavailable")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 32769))
	if err != nil || len(data) > 32768 {
		return nil, errors.New("invalid identity signing key file")
	}
	block, rest := pem.Decode(data)
	if block == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("invalid identity signing key encoding")
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	default:
		err = errors.New("unsupported key encoding")
	}
	if err != nil || key == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, errors.New("invalid identity RSA signing key")
	}
	return key, nil
}

// SchemaReady is intentionally not a migration runner. Deployment invokes the
// store's explicit Migrate only after its paired backup/recovery gate.
func SchemaReady(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("missing identity database")
	}
	for _, model := range []any{&federationdomain.Subject{}, &federationdomain.BrowserSession{}, &federationdomain.Interaction{}, &federationdomain.AccessToken{}, &federationdomain.ClientSession{}} {
		if !db.WithContext(ctx).Migrator().HasTable(model) {
			return errors.New("identity schema is not ready")
		}
	}
	return nil
}
