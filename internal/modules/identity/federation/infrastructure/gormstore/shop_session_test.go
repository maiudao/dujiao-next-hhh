package federationstore

import (
	"errors"
	"testing"
	"time"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
)

func TestShopSessionRotationAndIndependentBrowserRevocation(t *testing.T) {
	s, _ := testStore(t)
	deadline := time.Now().Add(time.Hour)
	firstCookie, first, err := s.StartShopSession(t.Context(), 1, 0, deadline, "", false)
	if err != nil {
		t.Fatal(err)
	}
	otherCookie, other, err := s.StartShopSession(t.Context(), 1, 0, deadline, "", false)
	if err != nil {
		t.Fatal(err)
	}
	rotatedCookie, rotated, err := s.StartShopSession(t.Context(), 1, 0, deadline, firstCookie, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == rotated.ID || firstCookie == rotatedCookie {
		t.Fatal("login did not rotate session")
	}
	if _, err := s.SessionByID(t.Context(), first.ID); !errors.Is(err, ErrInteraction) {
		t.Fatal("old JWT session survived rotation")
	}
	if err := s.RevokeSession(t.Context(), rotated.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(t.Context(), rotatedCookie); !errors.Is(err, ErrInteraction) {
		t.Fatal("logout left browser cookie usable")
	}
	if _, err := s.SessionByID(t.Context(), rotated.ID); !errors.Is(err, ErrInteraction) {
		t.Fatal("logout left JWT session usable")
	}
	if got, err := s.Session(t.Context(), otherCookie); err != nil || got.ID != other.ID {
		t.Fatal("logout affected another browser")
	}
}

func TestShopSessionRequiresCompletedSecondFactorAndCurrentVersion(t *testing.T) {
	s, db := testStore(t)
	now := time.Now().Add(-time.Minute)
	if err := db.Model(&userdomain.User{}).Where("id = ?", 1).UpdateColumn("totp_enabled_at", now).Error; err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Hour)
	if _, _, err := s.StartShopSession(t.Context(), 1, 0, deadline, "", false); !errors.Is(err, ErrInteraction) {
		t.Fatal("2FA was skipped")
	}
	cookie, _, err := s.StartShopSession(t.Context(), 1, 0, deadline, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(t.Context(), cookie); err != nil {
		t.Fatal("completed 2FA could not restore session")
	}
	if err := db.Model(&userdomain.User{}).Where("id = ?", 1).UpdateColumn("token_version", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Session(t.Context(), cookie); !errors.Is(err, ErrInteraction) {
		t.Fatal("security version change left session usable")
	}
	if _, _, err := s.StartShopSession(t.Context(), 1, 0, deadline, "", true); !errors.Is(err, ErrInteraction) {
		t.Fatal("stale token created session")
	}
}
