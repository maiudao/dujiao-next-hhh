package application

import (
	"context"

	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/shared/mailbrand"
	"github.com/dujiao-next/internal/shared/passwordpolicy"
)

type registrationBrandKey struct{}

func (s *Service) RegistrationPasswordPolicy() passwordpolicy.Policy {
	return s.cfg.Security.PasswordPolicy.ValidationPolicy()
}

func (s *Service) RequiresRegistrationEmailProof() bool {
	return s != nil && s.cfg != nil && s.cfg.IdentityProvider.Enabled
}

func (s *Service) RegistrationCodePolicy() (int, int) {
	return resolveSendIntervalSeconds(s.cfg.Email.VerifyCode), resolveExpireMinutes(s.cfg.Email.VerifyCode) * 60
}

func (s *Service) RegistrationCodeLength() int {
	return resolveCodeLength(s.cfg.Email.VerifyCode)
}

// Preview and final registration share one locked attempt counter. A successful
// preview does not consume the proof; account creation rechecks it atomically.
func (s *Service) CheckRegistrationCode(email, code string) error {
	normalized, err := normalizeUserSuppliedEmail(email)
	if err != nil {
		return err
	}
	return s.checkEmailProofs([]emailProof{{normalized, constants.VerifyPurposeRegister, code}})
}

// SendRegistrationCodeForSite is called only by the authenticated federation
// adapter. SMTP settings and verification-code storage remain owned by Shop.
func (s *Service) SendRegistrationCodeForSite(ctx context.Context, email, locale string, brand mailbrand.Brand) error {
	return s.SendVerifyCode(context.WithValue(ctx, registrationBrandKey{}, brand), email, constants.VerifyPurposeRegister, locale)
}
