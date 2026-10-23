package application

import (
	"time"

	"github.com/dujiao-next/internal/modules/identity/jwttoken"
	"github.com/golang-jwt/jwt/v5"
)

// BindBrowserToken preserves a successfully authenticated token's identity and
// caps its lifetime to the authority session. Challenge tokens cannot be bound.
func (s *Service) BindBrowserToken(raw, sid string, deadline time.Time) (string, error) {
	claims, err := s.ParseUserJWT(raw)
	if err != nil || !jwttoken.IsAccessType(claims.Typ) || claims.UserID == 0 || len(sid) != 43 || !deadline.After(time.Now()) || claims.ExpiresAt == nil {
		return "", ErrInvalidCredentials
	}
	claims.SessionID = sid
	if deadline.Before(claims.ExpiresAt.Time) {
		claims.ExpiresAt = jwt.NewNumericDate(deadline)
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.UserJWT.SecretKey))
}
