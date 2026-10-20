package shopauth

import (
	"context"
	"strings"
	"time"

	"github.com/dujiao-next/internal/config"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	usertotpapp "github.com/dujiao-next/internal/modules/identity/userauth/totp/application"
	"gorm.io/gorm"
)

type SecondFactor struct {
	Config *config.Config
	Store  *federationstore.Store
}

// Complete uses the existing encrypted TOTP and recovery-code service, with its
// repository bound to the exact transaction which creates the authority session.
func (a SecondFactor) Complete(ctx context.Context, value *federationdomain.Interaction, code, recovery string, lifetime time.Duration) (string, *federationdomain.BrowserSession, error) {
	if a.Config == nil || a.Store == nil {
		return "", nil, federationdomain.ErrInteraction
	}
	code, recovery = strings.TrimSpace(code), strings.TrimSpace(recovery)
	return a.Store.AuthenticateSecondFactor(ctx, value, lifetime, func(tx *gorm.DB, userID uint) error {
		if (code == "") == (recovery == "") || len(code) > 16 || len(recovery) > 64 {
			return federationdomain.ErrInteraction
		}
		verifier := usertotpapp.NewService(a.Config, userstore.New(tx), nil)
		if recovery != "" {
			return verifier.VerifyChallengeRecoveryCode(userID, recovery)
		}
		return verifier.VerifyChallengeCode(userID, code)
	})
}
