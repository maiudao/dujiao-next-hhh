package integrationtest

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dujiao-next/internal/config"
	federationapp "github.com/dujiao-next/internal/modules/identity/federation/application"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/shopauth"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	usertotpapp "github.com/dujiao-next/internal/modules/identity/userauth/totp/application"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fixture struct {
	db       *gorm.DB
	store    *federationstore.Store
	auth     *federationapp.Service
	password *userauthapp.Service
	totp     *usertotpapp.Service
	user     *userdomain.User
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "auth.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("fixture-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	user := &userdomain.User{Email: "fixture@example.invalid", PasswordHash: string(passwordHash), Status: "active", EmailVerifiedAt: &now}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{App: config.AppConfig{SecretKey: "synthetic-federation-totp-encryption-key"}}
	repo := userstore.New(db)
	password := userauthapp.NewService(cfg, repo, nil, nil, nil, nil, nil)
	auth, err := federationapp.New(store, password, shopauth.SecondFactor{Config: cfg, Store: store}, time.Hour, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{db, store, auth, password, usertotpapp.NewService(cfg, repo, nil), user}
}

func (f *fixture) begin(t *testing.T) federationapp.Binding {
	t.Helper()
	browser, err := federationstore.Secret()
	if err != nil {
		t.Fatal(err)
	}
	csrf, err := federationstore.Secret()
	if err != nil {
		t.Fatal(err)
	}
	value, err := f.store.Begin(t.Context(), "main", []byte(`{"client_id":"main"}`), browser, csrf)
	if err != nil {
		t.Fatal(err)
	}
	return federationapp.Binding{RequestID: value.ID, ClientID: "main", Browser: browser, CSRF: csrf}
}

func (f *fixture) enableTOTP(t *testing.T) (string, []string) {
	t.Helper()
	setup, err := f.totp.Setup(f.user.ID)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.totp.Enable(f.user.ID, code)
	if err != nil {
		t.Fatal(err)
	}
	return setup.Secret, result.RecoveryCodes
}

func TestRealPasswordVerificationDoesNotIssueSessionBeforeBoundCommit(t *testing.T) {
	f := newFixture(t)
	identity, err := f.password.CheckPassword(f.user.Email, "fixture-password")
	if err != nil || identity.UserID != f.user.ID || identity.RequiresTOTP {
		t.Fatalf("password proof failed: %v", err)
	}
	var user userdomain.User
	if err := f.db.First(&user, f.user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.LastLoginAt != nil {
		t.Fatal("first-factor verification mutated login state")
	}
	binding := f.begin(t)
	wrong := binding
	wrong.Browser = "not-a-browser-secret"
	if _, err := f.auth.Password(t.Context(), wrong, f.user.Email, "fixture-password", false); err == nil {
		t.Fatal("password bypassed browser binding")
	}
	if _, err := f.auth.Password(t.Context(), binding, f.user.Email, "wrong", false); !errors.Is(err, userauthapp.ErrInvalidCredentials) {
		t.Fatal("wrong password accepted")
	}
	result, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", false)
	if err != nil || result.Cookie == "" || result.Session == nil || result.RequiresSecondFactor {
		t.Fatalf("password login failed: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), result.Cookie) || strings.Contains(string(encoded), result.Session.CookieHash) {
		t.Fatal("frontend result serialized session secret")
	}
	if _, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", false); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("password replay created another session")
	}
}

func TestRealTOTPRequiresFirstFactorAndAllowsSuccessfulFinalAttempt(t *testing.T) {
	f := newFixture(t)
	secret, _ := f.enableTOTP(t)
	binding := f.begin(t)
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, code, ""); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("TOTP skipped password")
	}
	result, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", true)
	if err != nil || !result.RequiresSecondFactor || result.Cookie != "" || result.Session != nil {
		t.Fatalf("first factor bypassed TOTP: %v", err)
	}
	for i := 0; i < federationstore.MaxAttempts-1; i++ {
		if _, err := f.auth.SecondFactor(t.Context(), binding, "invalid", ""); err == nil {
			t.Fatal("invalid TOTP accepted")
		}
	}
	code, err = totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	result, err = f.auth.SecondFactor(t.Context(), binding, code, "")
	if err != nil || result.Session == nil || result.Session.AMR != "pwd otp" {
		t.Fatalf("last valid TOTP attempt failed: %v", err)
	}
	if result.Session.ExpiresAt.Sub(result.Session.AuthenticatedAt) != 30*24*time.Hour {
		t.Fatal("second factor changed remembered lifetime")
	}
	if err := f.store.SaveCode(t.Context(), binding.RequestID, "last-attempt-code"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ConsumeCode(t.Context(), binding.RequestID, "main", `["openid"]`); err != nil {
		t.Fatal("last successful attempt blocked protocol exchange")
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, code, ""); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("TOTP interaction replay accepted")
	}
}

func TestRecoveryCodeRollsBackWhenSessionCannotBeSaved(t *testing.T) {
	f := newFixture(t)
	_, recovery := f.enableTOTP(t)
	binding := f.begin(t)
	if _, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", false); err != nil {
		t.Fatal(err)
	}
	var before userdomain.User
	if err := f.db.First(&before, f.user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Callback().Create().Before("gorm:create").Register("test:session-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_browser_sessions" {
			tx.AddError(errors.New("injected session insert failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, "", recovery[0]); err == nil {
		t.Fatal("injected save failure ignored")
	}
	if err := f.db.Callback().Create().Remove("test:session-failure"); err != nil {
		t.Fatal(err)
	}
	var after userdomain.User
	if err := f.db.First(&after, f.user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if before.RecoveryCodes != after.RecoveryCodes {
		t.Fatal("recovery code lost without committed session")
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, "", recovery[0]); err != nil {
		t.Fatalf("recovery retry failed: %v", err)
	}
	other := f.begin(t)
	if _, err := f.auth.Password(t.Context(), other, f.user.Email, "fixture-password", false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.SecondFactor(t.Context(), other, "", recovery[0]); err == nil {
		t.Fatal("recovery code reused in another interaction")
	}
}

func TestPendingSecondFactorCannotSurviveAuthVersionChange(t *testing.T) {
	f := newFixture(t)
	_, recovery := f.enableTOTP(t)
	binding := f.begin(t)
	if _, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", false); err != nil {
		t.Fatal(err)
	}
	if err := f.db.Model(&userdomain.User{}).Where("id = ?", f.user.ID).UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, "", recovery[0]); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("pending authentication survived security change")
	}
	var count int64
	if err := f.db.Model(&federationdomain.BrowserSession{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed second factor persisted a session")
	}
}

func TestSecondFactorAttemptLimitBlocksLaterValidRecoveryCode(t *testing.T) {
	f := newFixture(t)
	_, recovery := f.enableTOTP(t)
	binding := f.begin(t)
	if _, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", false); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < federationstore.MaxAttempts; i++ {
		if _, err := f.auth.SecondFactor(t.Context(), binding, "invalid", ""); err == nil {
			t.Fatal("invalid TOTP accepted")
		}
	}
	if _, err := f.auth.SecondFactor(t.Context(), binding, "", recovery[0]); !errors.Is(err, federationdomain.ErrInteraction) {
		t.Fatal("attempt limit bypassed with recovery code")
	}
	var stored federationdomain.Interaction
	if err := f.db.First(&stored, "id = ?", binding.RequestID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Attempts != federationstore.MaxAttempts || stored.SessionID != "" {
		t.Fatal("failed proof did not persist only bounded attempts")
	}
}

func TestConcurrentInteractionsCannotConsumeOneRecoveryCodeTwice(t *testing.T) {
	f := newFixture(t)
	_, recovery := f.enableTOTP(t)
	bindings := []federationapp.Binding{f.begin(t), f.begin(t)}
	for _, binding := range bindings {
		if _, err := f.auth.Password(t.Context(), binding, f.user.Email, "fixture-password", false); err != nil {
			t.Fatal(err)
		}
	}
	results := make(chan error, len(bindings))
	for _, binding := range bindings {
		go func(binding federationapp.Binding) {
			_, err := f.auth.SecondFactor(t.Context(), binding, "", recovery[0])
			results <- err
		}(binding)
	}
	success := 0
	for range bindings {
		if err := <-results; err == nil {
			success++
		}
	}
	var sessions int64
	if err := f.db.Model(&federationdomain.BrowserSession{}).Count(&sessions).Error; err != nil || success != 1 || sessions != 1 {
		t.Fatalf("recovery concurrency produced %d successes and %d sessions: %v", success, sessions, err)
	}
}
