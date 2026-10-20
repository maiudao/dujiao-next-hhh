package integrationtest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	userauthgormstore "github.com/dujiao-next/internal/modules/identity/userauth/infrastructure/gormstore"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func addEmailChallenge(t *testing.T, db *gorm.DB, email, purpose, value string) *emailverificationdomain.Code {
	t.Helper()
	now := time.Now()
	code := &emailverificationdomain.Code{
		Email: email, Purpose: purpose, Code: value,
		SentAt: now, CreatedAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := db.Create(code).Error; err != nil {
		t.Fatal(err)
	}
	return code
}

func emailChallenge(t *testing.T, db *gorm.DB, id uint) emailverificationdomain.Code {
	t.Helper()
	var code emailverificationdomain.Code
	if err := db.First(&code, id).Error; err != nil {
		t.Fatal(err)
	}
	return code
}

func TestRegistrationFailurePreservesProofAndRetrySucceeds(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	code := addEmailChallenge(t, db, "atomic@example.com", constants.VerifyPurposeRegister, "123456")
	failure := errors.New("injected account insert failure")
	const callback = "test:reject-registration-insert"
	if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, _, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
	if !errors.Is(err, failure) {
		t.Fatalf("want injected failure, got %v", err)
	}
	if emailChallenge(t, db, code.ID).VerifiedAt != nil {
		t.Fatal("failed account insert consumed proof")
	}
	if err := db.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	user, token, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
	if err != nil || user == nil || token == "" {
		t.Fatalf("retry failed: %v", err)
	}
	if emailChallenge(t, db, code.ID).VerifiedAt == nil {
		t.Fatal("successful retry did not consume proof")
	}
}

func TestWrongVerificationAttemptsPersistAndStopCorrectCode(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	code := addEmailChallenge(t, db, "attempts@example.com", constants.VerifyPurposeRegister, "123456")
	for attempt := 1; attempt <= 5; attempt++ {
		_, _, _, err := svc.Register(code.Email, "secret123", "999999", true, true)
		if !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if got := emailChallenge(t, db, code.ID).AttemptCount; got != attempt {
			t.Fatalf("counter=%d, want %d", got, attempt)
		}
	}
	_, _, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
	if !errors.Is(err, userauthapp.ErrVerifyCodeAttemptsExceeded) {
		t.Fatalf("correct code bypassed attempt budget: %v", err)
	}
	if emailChallenge(t, db, code.ID).VerifiedAt != nil {
		t.Fatal("locked challenge was consumed")
	}
}

func TestConcurrentRegistrationConsumesOneProofOnce(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	code := addEmailChallenge(t, db, "parallel@example.com", constants.VerifyPurposeRegister, "123456")
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Go(func() {
			<-start
			_, _, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
			results <- err
		})
	}
	close(start)
	workers.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) && !errors.Is(err, userauthapp.ErrEmailExists) {
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	var count int64
	if err := db.Model(&userdomain.User{}).Where("email = ?", code.Email).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if success != 1 || count != 1 || emailChallenge(t, db, code.ID).VerifiedAt == nil {
		t.Fatalf("successes=%d accounts=%d; want one committed registration", success, count)
	}
}

func TestPasswordResetFailureRollsBackProofAndAuthVersion(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	user, _, _, err := svc.Register("reset@example.com", "secret123", "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	code := addEmailChallenge(t, db, user.Email, constants.VerifyPurposeReset, "123456")
	failure := errors.New("injected reset update failure")
	const callback = "test:reject-password-update"
	if err := db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(user.Email, code.Code, "newsecret123"); !errors.Is(err, failure) {
		t.Fatalf("want injected failure, got %v", err)
	}
	if emailChallenge(t, db, code.ID).VerifiedAt != nil {
		t.Fatal("failed reset consumed proof")
	}
	var before userdomain.User
	if err := db.First(&before, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if before.TokenVersion != user.TokenVersion || before.PasswordHash != user.PasswordHash {
		t.Fatal("failed reset changed credentials")
	}
	if err := db.Callback().Update().Remove(callback); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(user.Email, code.Code, "newsecret123"); err != nil {
		t.Fatal(err)
	}
	var after userdomain.User
	if err := db.First(&after, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.TokenVersion != user.TokenVersion+1 || after.TokenInvalidBefore == nil ||
		bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("newsecret123")) != nil {
		t.Fatal("successful reset did not update password and auth version together")
	}
	if err := svc.ResetPassword(user.Email, code.Code, "thirdsecret123"); !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) {
		t.Fatalf("reset proof replay accepted: %v", err)
	}
}

func TestChangeEmailWrongNewProofPreservesOldProof(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	user, _, _, err := svc.Register("a-old@example.com", "secret123", "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	oldCode := addEmailChallenge(t, db, user.Email, constants.VerifyPurposeChangeEmailOld, "123456")
	newCode := addEmailChallenge(t, db, "z-new@example.com", constants.VerifyPurposeChangeEmailNew, "654321")
	_, err = svc.ChangeEmail(user.ID, newCode.Email, oldCode.Code, "000000")
	if !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) {
		t.Fatalf("wrong new proof: %v", err)
	}
	if emailChallenge(t, db, oldCode.ID).VerifiedAt != nil || emailChallenge(t, db, newCode.ID).AttemptCount != 1 {
		t.Fatal("wrong second proof consumed first proof or lost attempt count")
	}
	changed, err := svc.ChangeEmail(user.ID, newCode.Email, oldCode.Code, newCode.Code)
	if err != nil {
		t.Fatal(err)
	}
	if changed.ID != user.ID || changed.Email != newCode.Email ||
		emailChallenge(t, db, oldCode.ID).VerifiedAt == nil || emailChallenge(t, db, newCode.ID).VerifiedAt == nil {
		t.Fatal("email change did not retain user ID and consume both proofs")
	}
}

func TestTelegramEmailBindingFailureDoesNotConsumeProof(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	user, _, _, err := svc.Register("channel@example.com", "secret123", "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	code := addEmailChallenge(t, db, user.Email, constants.VerifyPurposeTelegramBind, "123456")
	failure := errors.New("injected identity insert failure")
	const callback = "test:reject-channel-identity"
	if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_oauth_identities" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	input := userauthapp.BindTelegramChannelByEmailCodeInput{
		Email: user.Email, Code: code.Code,
		Identity: userauthapp.TelegramChannelIdentityInput{ChannelUserID: "atomic-channel-test"},
	}
	if _, _, _, err := svc.BindTelegramChannelByEmailCode(input); !errors.Is(err, failure) {
		t.Fatalf("want injected binding failure, got %v", err)
	}
	if emailChallenge(t, db, code.ID).VerifiedAt != nil {
		t.Fatal("failed identity binding consumed proof")
	}
	if err := db.Callback().Create().Remove(callback); err != nil {
		t.Fatal(err)
	}
	bound, identity, _, err := svc.BindTelegramChannelByEmailCode(input)
	if err != nil || bound == nil || identity == nil || identity.UserID != user.ID {
		t.Fatalf("binding retry failed: %v", err)
	}
}

func TestRegistrationRejectsUnusableEmailProofs(t *testing.T) {
	for _, fault := range []string{"expired", "wrong-purpose", "consumed", "replaced"} {
		t.Run(fault, func(t *testing.T) {
			svc, _, db := setupTelegramOAuthTestService(t)
			purpose := constants.VerifyPurposeRegister
			if fault == "wrong-purpose" {
				purpose = constants.VerifyPurposeReset
			}
			code := addEmailChallenge(t, db, "invalid@example.com", purpose, "123456")
			expected := userauthapp.ErrVerifyCodeInvalid
			switch fault {
			case "expired":
				expected = userauthapp.ErrVerifyCodeExpired
				if err := db.Model(code).UpdateColumn("expires_at", time.Now().Add(-time.Second)).Error; err != nil {
					t.Fatal(err)
				}
			case "consumed":
				if err := db.Model(code).UpdateColumn("verified_at", time.Now()).Error; err != nil {
					t.Fatal(err)
				}
			case "replaced":
				addEmailChallenge(t, db, code.Email, purpose, "654321")
			}
			_, _, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
			if !errors.Is(err, expected) {
				t.Fatalf("want %v, got %v", expected, err)
			}
			var count int64
			if err := db.Model(&userdomain.User{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("invalid proof created an account")
			}
		})
	}
}

type afterPreviewUnit struct {
	inner userauthapp.AuthUnitOfWork
	after func()
	calls int
}

func (u *afterPreviewUnit) WithinTransaction(ctx context.Context, fn func(userauthapp.AuthTransaction) error) error {
	err := u.inner.WithinTransaction(ctx, fn)
	u.calls++
	if err == nil && u.calls == 1 {
		u.after()
	}
	return err
}

func TestRegistrationRechecksProofAfterPasswordHashPreparation(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	code := addEmailChallenge(t, db, "recheck@example.com", constants.VerifyPurposeRegister, "123456")
	svc.SetAuthUnitOfWork(&afterPreviewUnit{inner: userauthgormstore.New(db), after: func() {
		if err := db.Model(code).UpdateColumn("attempt_count", 5).Error; err != nil {
			t.Fatal(err)
		}
	}})
	_, _, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
	if !errors.Is(err, userauthapp.ErrVerifyCodeAttemptsExceeded) {
		t.Fatalf("preflight bypassed changed verification budget: %v", err)
	}
	if emailChallenge(t, db, code.ID).VerifiedAt != nil {
		t.Fatal("exhausted proof was consumed after preflight")
	}
}

func TestChangeEmailWriteFailureRollsBackBothProofs(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	user, _, _, err := svc.Register("rollback-old@example.com", "secret123", "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	oldCode := addEmailChallenge(t, db, user.Email, constants.VerifyPurposeChangeEmailOld, "123456")
	newCode := addEmailChallenge(t, db, "rollback-new@example.com", constants.VerifyPurposeChangeEmailNew, "654321")
	failure := errors.New("injected email update failure")
	if err := db.Callback().Update().Before("gorm:update").Register("test:reject-email-update", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ChangeEmail(user.ID, newCode.Email, oldCode.Code, newCode.Code); !errors.Is(err, failure) {
		t.Fatalf("want injected email update failure, got %v", err)
	}
	if emailChallenge(t, db, oldCode.ID).VerifiedAt != nil || emailChallenge(t, db, newCode.ID).VerifiedAt != nil {
		t.Fatal("failed email update consumed one or both proofs")
	}
}
