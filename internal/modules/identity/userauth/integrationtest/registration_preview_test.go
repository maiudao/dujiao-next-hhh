package integrationtest

import (
	"errors"
	"testing"
	"time"

	emailverificationdomain "github.com/dujiao-next/internal/modules/identity/emailverification/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
)

func TestRegistrationPreviewPreservesCodeUntilAtomicRegistration(t *testing.T) {
	svc, _, db := newRegistrationDomainPolicyAuthService(t)
	now := time.Now()
	old := emailverificationdomain.Code{Email: "preview@example.com", Purpose: "register", Code: "123456", SentAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Minute)}
	latest := emailverificationdomain.Code{Email: old.Email, Purpose: "register", Code: "654321", SentAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&latest).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckRegistrationCode(old.Email, old.Code); !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) {
		t.Fatal("old code accepted")
	}
	for i := 0; i < 2; i++ {
		if err := svc.CheckRegistrationCode(latest.Email, latest.Code); err != nil {
			t.Fatal("valid preview failed", err)
		}
	}
	var stored emailverificationdomain.Code
	if err := db.First(&stored, latest.ID).Error; err != nil || stored.VerifiedAt != nil || stored.AttemptCount != 1 {
		t.Fatal("preview consumed proof or lost failed-attempt counter")
	}
	user, _, _, err := svc.Register(latest.Email, "secret123", latest.Code, true, true)
	if err != nil || user.EmailVerifiedAt == nil {
		t.Fatal("preview prevented verified registration", err)
	}
	if err := svc.CheckRegistrationCode(latest.Email, latest.Code); !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) {
		t.Fatal("consumed code reused")
	}
}

func TestRegistrationPreviewSharesAttemptLimitAndExpiry(t *testing.T) {
	svc, _, db := newRegistrationDomainPolicyAuthService(t)
	now := time.Now()
	proof := emailverificationdomain.Code{Email: "locked@example.com", Purpose: "register", Code: "123456", SentAt: now, ExpiresAt: now.Add(time.Minute)}
	if err := db.Create(&proof).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := svc.CheckRegistrationCode(proof.Email, "000000"); !errors.Is(err, userauthapp.ErrVerifyCodeInvalid) {
			t.Fatal("guess not counted", err)
		}
	}
	if err := svc.CheckRegistrationCode(proof.Email, proof.Code); !errors.Is(err, userauthapp.ErrVerifyCodeAttemptsExceeded) {
		t.Fatal("preview bypassed attempt limit")
	}
	if _, _, _, err := svc.Register(proof.Email, "secret123", proof.Code, true, true); !errors.Is(err, userauthapp.ErrVerifyCodeAttemptsExceeded) {
		t.Fatal("final registration bypassed preview limit")
	}
	if err := db.Model(&proof).Updates(map[string]any{"attempt_count": 0, "expires_at": now.Add(-time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckRegistrationCode(proof.Email, proof.Code); !errors.Is(err, userauthapp.ErrVerifyCodeExpired) {
		t.Fatal("expired code accepted")
	}
}
