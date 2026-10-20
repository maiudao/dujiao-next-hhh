package federationstore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testStore(t *testing.T) (*Store, *gorm.DB) {
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
	s := New(db)
	if err := db.AutoMigrate(&userdomain.User{}); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	user := userdomain.User{ID: 1, Email: "fixture@example.invalid", Status: constants.UserStatusActive}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return s, db
}

func testSecret(t *testing.T) string {
	t.Helper()
	value, err := Secret()
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func testInteraction(t *testing.T, s *Store) (*federationdomain.Interaction, string, string) {
	t.Helper()
	browser, csrf := testSecret(t), testSecret(t)
	value, err := s.Begin(context.Background(), "main", []byte(`{"scope":"openid profile"}`), browser, csrf)
	if err != nil {
		t.Fatal(err)
	}
	return value, browser, csrf
}

func testAuthenticated(t *testing.T, s *Store) (*federationdomain.Interaction, string, *federationdomain.BrowserSession) {
	t.Helper()
	value, browser, csrf := testInteraction(t, s)
	cookie, session, err := s.AuthenticatePassword(context.Background(), value, 1, 0, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	value, err = s.ReadBound(context.Background(), value.ID, "main", browser, csrf)
	if err != nil {
		t.Fatal(err)
	}
	return value, cookie, session
}

func TestSubjectIsStableAcrossIndependentLogins(t *testing.T) {
	s, db := testStore(t)
	first, _, _ := testAuthenticated(t, s)
	second, _, _ := testAuthenticated(t, s)
	if first.SubjectID == "" || first.SubjectID != second.SubjectID {
		t.Fatal("subject changed between logins")
	}
	var count int64
	if err := db.Model(&federationdomain.Subject{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("subject count %d: %v", count, err)
	}
}

func TestDatabaseReopenPreservesUnconsumedCodeAndHashedBrowserSecrets(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	value, browser, csrf := testInteraction(t, s)
	cookie, session, err := s.AuthenticatePassword(ctx, value, 1, 0, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCode(ctx, value.ID, "reopen-code-fixture"); err != nil {
		t.Fatal(err)
	}
	dsn := db.Dialector.(*sqlite.Dialector).DSN
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	reopenedPool, err := reopened.DB()
	if err != nil {
		t.Fatal(err)
	}
	reopenedPool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = reopenedPool.Close() })
	s = New(reopened)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := s.ReadBound(ctx, value.ID, "main", browser, csrf)
	if err != nil {
		t.Fatal(err)
	}
	if stored.BrowserHash == browser || stored.CSRFHash == csrf || stored.CodeHash == nil || *stored.CodeHash == "reopen-code-fixture" {
		t.Fatal("raw browser/code secret stored")
	}
	loaded, err := s.Session(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != session.ID || loaded.CookieHash == cookie {
		t.Fatal("session was lost or raw cookie persisted")
	}
	lookup, err := s.RequestByCode(ctx, "reopen-code-fixture")
	if err != nil || lookup.ID != value.ID {
		t.Fatalf("code lost on reopen: %v", err)
	}
	token, err := s.ConsumeCode(ctx, value.ID, "main", `["openid"]`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRequest(ctx, value.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishRequest(ctx, value.ID); err != nil {
		t.Fatal("finishing committed grant must be idempotent")
	}
	if _, err := s.RequestByCode(ctx, "reopen-code-fixture"); !errors.Is(err, ErrInteraction) {
		t.Fatal("consumed code readable after restart")
	}
	if _, err := s.LiveToken(ctx, token.ID, token.SubjectID); err != nil {
		t.Fatal(err)
	}
}

func TestBindingsExpiryAndAttemptBudget(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	value, browser, csrf := testInteraction(t, s)
	for _, args := range [][3]string{{"other", browser, csrf}, {"main", testSecret(t), csrf}, {"main", browser, testSecret(t)}, {"main", "", csrf}} {
		if _, err := s.ReadBound(ctx, value.ID, args[0], args[1], args[2]); !errors.Is(err, ErrInteraction) {
			t.Fatalf("invalid binding accepted: %v", err)
		}
	}
	for i := 0; i < MaxAttempts; i++ {
		if err := s.RejectAttempt(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ReadBound(ctx, value.ID, "main", browser, csrf); !errors.Is(err, ErrInteraction) {
		t.Fatal("attempt limit ignored")
	}
	if _, _, err := s.AuthenticatePassword(ctx, value, 1, 0, time.Hour); !errors.Is(err, ErrInteraction) {
		t.Fatal("stale read bypassed attempt limit")
	}
	value, _, _ = testInteraction(t, s)
	s.now = func() time.Time { return value.ExpiresAt }
	if _, _, err := s.AuthenticatePassword(ctx, value, 1, 0, time.Hour); !errors.Is(err, ErrInteraction) {
		t.Fatal("expired request authenticated")
	}
}

func TestCancellationPreventsStaleAuthentication(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	value, _, _ := testInteraction(t, s)
	if err := s.Cancel(ctx, value); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AuthenticatePassword(ctx, value, 1, 0, time.Hour); !errors.Is(err, ErrInteraction) {
		t.Fatal("cancelled request authenticated")
	}
	for _, model := range []any{&federationdomain.Subject{}, &federationdomain.BrowserSession{}} {
		var count int64
		if err := db.Model(model).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("failed auth left records: %d %v", count, err)
		}
	}
}

func TestSecondFactorCannotBeDowngradedOrSwitched(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	now := time.Now()
	if err := db.Model(&userdomain.User{}).Where("id = ?", 1).Update("totp_enabled_at", now).Error; err != nil {
		t.Fatal(err)
	}
	value, browser, csrf := testInteraction(t, s)
	if _, _, err := s.AuthenticatePassword(ctx, value, 1, 0, time.Hour); !errors.Is(err, ErrInteraction) {
		t.Fatal("2FA user authenticated without second factor")
	}
	if _, _, err := s.AuthenticateSecondFactor(ctx, value, time.Hour, func(*gorm.DB, uint) error { return nil }); !errors.Is(err, ErrInteraction) {
		t.Fatal("2FA completed without first factor")
	}
	if err := s.AwaitSecondFactor(ctx, value, 1, 0, false); err != nil {
		t.Fatal(err)
	}
	if err := s.AwaitSecondFactor(ctx, value, 2, 0, false); !errors.Is(err, ErrInteraction) {
		t.Fatal("pending user switched")
	}
	if _, _, err := s.AuthenticatePassword(ctx, value, 1, 0, time.Hour); !errors.Is(err, ErrInteraction) {
		t.Fatal("2FA state downgraded")
	}
	value, err := s.ReadBound(ctx, value.ID, "main", browser, csrf)
	if err != nil {
		t.Fatal(err)
	}
	if _, session, err := s.AuthenticateSecondFactor(ctx, value, time.Hour, func(*gorm.DB, uint) error { return nil }); err != nil || session.AMR != "pwd otp" {
		t.Fatalf("2FA login failed: %v", err)
	}
}

func TestSessionInvalidationAndRestart(t *testing.T) {
	for _, mutation := range []string{"revoke", "version", "disable", "delete", "expire", "cutoff", "enable_2fa"} {
		t.Run(mutation, func(t *testing.T) {
			s, db := testStore(t)
			_, cookie, session := testAuthenticated(t, s)
			// No process-local session map may be required after restarting the adapter.
			s = New(db)
			if _, err := s.Session(context.Background(), cookie); err != nil {
				t.Fatal(err)
			}
			var err error
			switch mutation {
			case "revoke":
				err = s.RevokeSession(context.Background(), session.ID)
			case "version":
				err = db.Model(&userdomain.User{}).Where("id = ?", 1).Update("token_version", 1).Error
			case "disable":
				err = db.Model(&userdomain.User{}).Where("id = ?", 1).Update("status", "disabled").Error
			case "delete":
				err = db.Model(&userdomain.User{}).Where("id = ?", 1).Update("deleted_at", time.Now()).Error
			case "expire":
				s.now = func() time.Time { return session.ExpiresAt }
			case "cutoff":
				err = db.Model(&userdomain.User{}).Where("id = ?", 1).Update("token_invalid_before", session.AuthenticatedAt).Error
			case "enable_2fa":
				err = db.Model(&userdomain.User{}).Where("id = ?", 1).Update("totp_enabled_at", time.Now()).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Session(context.Background(), cookie); !errors.Is(err, ErrInteraction) {
				t.Fatalf("invalid session accepted: %v", err)
			}
		})
	}
}

func TestExistingSessionCanAuthorizeAnotherClientRequestWithoutNewSession(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	first, cookie, session := testAuthenticated(t, s)
	second, browser, csrf := testInteraction(t, s)
	if err := s.ResumeSession(ctx, second, cookie); err != nil {
		t.Fatal(err)
	}
	second, err := s.ReadBound(ctx, second.ID, "main", browser, csrf)
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionID != session.ID || second.SubjectID != first.SubjectID {
		t.Fatal("SSO changed browser session or subject")
	}
	if err := s.ResumeSession(ctx, second, cookie); !errors.Is(err, ErrInteraction) {
		t.Fatal("authenticated request was replayed")
	}
	var count int64
	if err := db.Model(&federationdomain.BrowserSession{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("session count %d: %v", count, err)
	}
	if err := s.RevokeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	third, _, _ := testInteraction(t, s)
	if err := s.ResumeSession(ctx, third, cookie); !errors.Is(err, ErrInteraction) {
		t.Fatal("revoked cookie resumed SSO")
	}
}

func TestIssuedCodeExpiryCancellationAndSessionRevocation(t *testing.T) {
	for _, mutation := range []string{"code_expiry", "cancel", "revoke", "version"} {
		t.Run(mutation, func(t *testing.T) {
			s, db := testStore(t)
			ctx := context.Background()
			value, _, session := testAuthenticated(t, s)
			if err := s.SaveCode(ctx, value.ID, "code-fixture"); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "code_expiry":
				var request federationdomain.Interaction
				if err := db.First(&request, "id = ?", value.ID).Error; err != nil {
					t.Fatal(err)
				}
				s.now = func() time.Time { return *request.CodeExpiresAt }
			case "cancel":
				if err := s.Cancel(ctx, value); err != nil {
					t.Fatal(err)
				}
			case "revoke":
				if err := s.RevokeSession(ctx, session.ID); err != nil {
					t.Fatal(err)
				}
			case "version":
				if err := db.Model(&userdomain.User{}).Where("id = ?", 1).Update("token_version", 1).Error; err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.ConsumeCode(ctx, value.ID, "main", `["openid"]`); !errors.Is(err, ErrInteraction) {
				t.Fatalf("invalid code issued token: %v", err)
			}
		})
	}
}

func TestTokenRemainsBoundToLiveSessionClientAndSubject(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	value, _, session := testAuthenticated(t, s)
	if err := s.SaveCode(ctx, value.ID, "code-fixture"); err != nil {
		t.Fatal(err)
	}
	for _, scopes := range []string{`null`, `{}`, `[]`, `[123]`} {
		if _, err := s.ConsumeCode(ctx, value.ID, "main", scopes); !errors.Is(err, ErrInteraction) {
			t.Fatalf("invalid scope document accepted: %s", scopes)
		}
	}
	token, err := s.ConsumeCode(ctx, value.ID, "main", `["openid"]`)
	if err != nil {
		t.Fatal(err)
	}
	if token.ExpiresAt.Sub(token.CreatedAt) > 5*time.Minute {
		t.Fatal("access token exceeds short lifetime")
	}
	if _, err := s.Token(ctx, token.ID, "other", token.SubjectID); !errors.Is(err, ErrInteraction) {
		t.Fatal("wrong client accepted")
	}
	if _, err := s.Token(ctx, token.ID, "main", "other"); !errors.Is(err, ErrInteraction) {
		t.Fatal("wrong subject accepted")
	}
	if _, err := s.Token(ctx, token.ID, "main", token.SubjectID); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Token(ctx, token.ID, "main", token.SubjectID); !errors.Is(err, ErrInteraction) {
		t.Fatal("revoked session still authorized token")
	}
}

func TestCodeConsumptionIsSingleUseAndTransactional(t *testing.T) {
	s, db := testStore(t)
	ctx := context.Background()
	value, _, _ := testAuthenticated(t, s)
	if err := s.SaveCode(ctx, value.ID, "authorization-code-fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeCode(ctx, value.ID, "other", `["openid"]`); !errors.Is(err, ErrInteraction) {
		t.Fatal("wrong client consumed code")
	}
	// Force insertion to fail after the conditional code update, then retry.
	if err := db.Callback().Create().Before("gorm:create").Register("test:token_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_access_tokens" {
			tx.AddError(errors.New("injected token persistence failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeCode(ctx, value.ID, "main", `["openid"]`); err == nil {
		t.Fatal("injected insert failure ignored")
	}
	if err := db.Callback().Create().Remove("test:token_failure"); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	for i := 0; i < cap(results); i++ {
		go func() { _, err := s.ConsumeCode(ctx, value.ID, "main", `["openid"]`); results <- err }()
	}
	success := 0
	for i := 0; i < cap(results); i++ {
		if err := <-results; err == nil {
			success++
		} else if !errors.Is(err, ErrInteraction) {
			t.Fatal(err)
		}
	}
	var count int64
	if err := db.Model(&federationdomain.AccessToken{}).Count(&count).Error; err != nil || success != 1 || count != 1 {
		t.Fatalf("success=%d tokens=%d err=%v", success, count, err)
	}
}
