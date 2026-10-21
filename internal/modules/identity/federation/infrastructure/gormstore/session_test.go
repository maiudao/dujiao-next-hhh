package federationstore

import (
	"errors"
	"testing"
	"time"

	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"gorm.io/gorm"
)

func TestClientSessionDelegationRequiresConsumedCodeAndLiveAuthority(t *testing.T) {
	s, db := testStore(t)
	request, _, session := testAuthenticated(t, s)
	if _, err := s.ClientSession(t.Context(), "main", session.ID, request.SubjectID); !errors.Is(err, ErrInteraction) {
		t.Fatal("unconsumed authentication created client delegation", err)
	}
	if err := s.SaveCode(t.Context(), request.ID, "delegation-test-code"); err != nil {
		t.Fatal(err)
	}
	token, err := s.ConsumeCode(t.Context(), request.ID, "main", `["openid"]`)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][2]string{{"other", request.SubjectID}, {"main", testSecret(t)}} {
		if _, err := s.ClientSession(t.Context(), input[0], session.ID, input[1]); !errors.Is(err, ErrInteraction) {
			t.Fatal("foreign client/subject used delegation", err)
		}
		if err := s.RevokeClientSession(t.Context(), input[0], session.ID, input[1]); !errors.Is(err, ErrInteraction) {
			t.Fatal("foreign client/subject revoked session", err)
		}
	}
	// Short API tokens remain expired. The independently delegated session check
	// can confirm the original browser session, but cannot extend its deadline.
	then := time.Now().Add(6 * time.Minute)
	s.now = func() time.Time { return then }
	if _, err := s.Token(t.Context(), token.ID, "main", request.SubjectID); !errors.Is(err, ErrInteraction) {
		t.Fatal("expired API token remained usable", err)
	}
	live, err := s.ClientSession(t.Context(), "main", session.ID, request.SubjectID)
	if err != nil || !live.ExpiresAt.Equal(session.ExpiresAt) {
		t.Fatal("delegated check changed original session deadline", err)
	}
	if err := db.Model(&userdomain.User{}).Where("id = ?", session.UserID).UpdateColumn("token_version", 1).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClientSession(t.Context(), "main", session.ID, request.SubjectID); !errors.Is(err, ErrInteraction) {
		t.Fatal("password/security version change left session active", err)
	}
	if err := s.RevokeClientSession(t.Context(), "main", session.ID, request.SubjectID); err != nil {
		t.Fatal("delegated logout could not revoke invalidated session", err)
	}
	if err := s.RevokeClientSession(t.Context(), "main", session.ID, request.SubjectID); err != nil {
		t.Fatal("delegated logout was not idempotent", err)
	}
}

func TestClientSessionGrantFailureRollsBackCodeAndToken(t *testing.T) {
	s, db := testStore(t)
	request, _, _ := testAuthenticated(t, s)
	if err := s.SaveCode(t.Context(), request.ID, "transactional-delegation-code"); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:grant_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_client_sessions" {
			tx.AddError(errors.New("injected grant persistence failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeCode(t.Context(), request.ID, "main", `["openid"]`); err == nil {
		t.Fatal("grant persistence failure ignored")
	}
	var count int64
	if err := db.Model(&federationdomain.AccessToken{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("failed grant left a usable token", err)
	}
	if err := db.Callback().Create().Remove("test:grant_failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ConsumeCode(t.Context(), request.ID, "main", `["openid"]`); err != nil {
		t.Fatal("grant failure consumed authorization code", err)
	}
	if err := db.Model(&federationdomain.ClientSession{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("successful retry did not create exactly one delegation", err)
	}
}

func TestDelegatedLogoutRevokesBothClientsButNotOtherBrowser(t *testing.T) {
	s, _ := testStore(t)
	main, cookie, session := testAuthenticated(t, s)
	consume := func(request *federationdomain.Interaction, client, code string) {
		t.Helper()
		if err := s.SaveCode(t.Context(), request.ID, code); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeCode(t.Context(), request.ID, client, `["openid"]`); err != nil {
			t.Fatal(err)
		}
	}
	consume(main, "main", "main-session-delegation")
	shop, err := s.Begin(t.Context(), "shop", []byte(`{"scope":"openid"}`), testSecret(t), testSecret(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ResumeSession(t.Context(), shop, cookie); err != nil {
		t.Fatal(err)
	}
	shop, err = s.Request(t.Context(), shop.ID)
	if err != nil || shop.SessionID != session.ID {
		t.Fatal("second client failed to share the browser session", err)
	}
	consume(shop, "shop", "shop-session-delegation")
	other, _, otherSession := testAuthenticated(t, s)
	consume(other, "main", "other-browser-delegation")
	if err := s.RevokeClientSession(t.Context(), "main", session.ID, main.SubjectID); err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"main", "shop"} {
		if _, err := s.ClientSession(t.Context(), client, session.ID, main.SubjectID); !errors.Is(err, ErrInteraction) {
			t.Fatal("delegated logout left another client active", err)
		}
	}
	if _, err := s.ClientSession(t.Context(), "main", otherSession.ID, other.SubjectID); err != nil {
		t.Fatal("delegated logout revoked a different browser", err)
	}
}
