package integrationtest

import (
	"errors"
	"testing"
	"time"

	"github.com/dujiao-next/internal/constants"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

func TestRegistrationWithoutCodePreservesLoginButDoesNotClaimEmailProof(t *testing.T) {
	svc, _, db := newRegistrationDomainPolicyAuthService(t)
	user, token, _, err := svc.Register("exempt@example.com", "secret123", "", true, false)
	if err != nil || user == nil || token == "" {
		t.Fatalf("registration failed: %v", err)
	}
	var stored userdomain.User
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.EmailVerifiedAt != nil || stored.HasTrustedEmailProof() || !stored.EmailVerificationExempt {
		t.Fatal("policy exemption was confused with mailbox proof")
	}
	result, err := svc.LoginStep1(user.Email, "secret123", false)
	if err != nil || result.Token == "" || result.RequiresTOTP {
		t.Fatalf("exempt password login was locked out: %v", err)
	}
	if _, err := svc.LoginStep1(user.Email, "wrong-password", false); !errors.Is(err, userauthapp.ErrInvalidCredentials) {
		t.Fatalf("exemption bypassed password check: %v", err)
	}
	// A real reset-email proof upgrades evidence atomically with the password.
	code := addEmailChallenge(t, db, user.Email, constants.VerifyPurposeReset, "123456")
	if err := svc.ResetPassword(user.Email, code.Code, "newsecret123"); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&stored, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.HasTrustedEmailProof() || stored.EmailVerificationExempt {
		t.Fatal("consumed proof was not recorded")
	}
}

func TestRegistrationCodeCreatesTrustedEvidenceOnlyAfterTransaction(t *testing.T) {
	svc, _, db := setupTelegramOAuthTestService(t)
	code := addEmailChallenge(t, db, "proved@example.com", constants.VerifyPurposeRegister, "123456")
	user, _, _, err := svc.Register(code.Email, "secret123", code.Code, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !user.HasTrustedEmailProof() || user.EmailVerificationExempt || user.EmailVerificationSource != userdomain.EmailProofCode {
		t.Fatal("registration proof was not recorded")
	}
	if emailChallenge(t, db, code.ID).VerifiedAt == nil {
		t.Fatal("proof not consumed")
	}
}

func TestLegacyEmailTimestampDoesNotBecomeTrustedEvidence(t *testing.T) {
	svc, _, repo, _ := newUser2FATestServices(t)
	user := createActiveUser(t, repo, "legacy@example.com", "secret123")
	if user.HasTrustedEmailProof() {
		t.Fatal("historical timestamp became linking evidence")
	}
	if _, err := svc.LoginStep1(user.Email, "secret123", false); err != nil {
		t.Fatalf("legacy login changed: %v", err)
	}
	user.EmailVerifiedAt = nil
	user.EmailVerificationExempt = false
	if err := repo.Update(user); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LoginStep1(user.Email, "secret123", false); !errors.Is(err, userauthapp.ErrEmailNotVerified) {
		t.Fatalf("unverified non-exempt account logged in: %v", err)
	}
}

func TestTrustedEmailProofRequiresKnownSourceAndTimestamp(t *testing.T) {
	now := time.Now()
	for _, source := range []string{"", "legacy", "admin", "unknown"} {
		user := &userdomain.User{Email: "owner@example.com", EmailVerifiedAt: &now, EmailVerificationSource: source}
		if user.HasTrustedEmailProof() {
			t.Fatalf("untrusted source accepted: %q", source)
		}
	}
	user := &userdomain.User{Email: "owner@example.com", EmailVerificationSource: userdomain.EmailProofCode}
	if user.HasTrustedEmailProof() {
		t.Fatal("source without proof time accepted")
	}
}

func TestEmailExemptionStillRequiresTwoFactorAuthentication(t *testing.T) {
	svc, totpSvc, repo, _ := newUser2FATestServices(t)
	user := createActiveUser(t, repo, "exempt-2fa@example.com", "secret123")
	user.EmailVerifiedAt = nil
	user.EmailVerificationExempt = true
	if err := repo.Update(user); err != nil {
		t.Fatal(err)
	}
	setup, err := totpSvc.Setup(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := totpSvc.Enable(user.ID, code); err != nil {
		t.Fatal(err)
	}
	result, err := svc.LoginStep1(user.Email, "secret123", false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RequiresTOTP || result.Token != "" || result.ChallengeToken == "" {
		t.Fatal("email-policy exemption bypassed the second factor")
	}
}

func TestEmailEvidenceMigrationDoesNotPromoteLegacyTimestamps(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	if err := db.Exec(`CREATE TABLE users (id integer PRIMARY KEY, email text NOT NULL UNIQUE, password_hash text NOT NULL, email_verified_at datetime)`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.Exec("INSERT INTO users (id, email, password_hash, email_verified_at) VALUES (?, ?, ?, ?)", 1, "legacy@example.com", "test-only", now).Error; err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := db.AutoMigrate(&userdomain.User{}); err != nil {
			t.Fatal(err)
		}
	}
	var user userdomain.User
	if err := db.First(&user, 1).Error; err != nil {
		t.Fatal(err)
	}
	if user.EmailVerifiedAt == nil || !user.EmailVerifiedAt.Equal(now) || user.HasTrustedEmailProof() || user.EmailVerificationExempt {
		t.Fatal("schema upgrade changed historical login policy or invented proof")
	}
}
