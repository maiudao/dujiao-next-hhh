package federationstore

import (
	"context"
	"time"

	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	"gorm.io/gorm"
)

// StartShopSession accepts only a completed Shop authentication from the HTTP
// adapter. It is never exposed as an endpoint accepting a browser-supplied ID.
func (s *Store) StartShopSession(ctx context.Context, userID uint, version uint64, deadline time.Time, previous string, secondFactor bool) (string, *federationdomain.BrowserSession, error) {
	now := s.now()
	if !deadline.After(now) || deadline.After(now.Add(30*24*time.Hour)) || (previous != "" && !validSecret(previous)) {
		return "", nil, ErrInteraction
	}
	cookie, err := Secret()
	if err != nil {
		return "", nil, err
	}
	id, err := Secret()
	if err != nil {
		return "", nil, err
	}
	session := &federationdomain.BrowserSession{ID: id, CookieHash: Hash(cookie), UserID: userID, TokenVersion: version, AuthenticatedAt: now, ExpiresAt: deadline, AMR: "shop"}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user, err := activeUser(tx, userID, version)
		if err != nil {
			return err
		}
		if user.TOTPEnabledAt != nil {
			if !secondFactor {
				return ErrInteraction
			}
			session.AMR = "pwd otp"
		}
		if _, err := subjectFor(tx, userID, now); err != nil {
			return err
		}
		if err := tx.Create(session).Error; err != nil {
			return err
		}
		if previous != "" {
			return tx.Model(&federationdomain.BrowserSession{}).Where("cookie_hash=? AND revoked_at IS NULL", Hash(previous)).UpdateColumn("revoked_at", now).Error
		}
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	return cookie, session, nil
}
