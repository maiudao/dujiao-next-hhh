package federationapp

import (
	"context"
	"errors"
	"time"

	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
)

type Store interface {
	ReadBound(context.Context, string, string, string, string) (*federationdomain.Interaction, error)
	RejectAttempt(context.Context, *federationdomain.Interaction) error
	AwaitSecondFactor(context.Context, *federationdomain.Interaction, uint, uint64, bool) error
	AuthenticatePassword(context.Context, *federationdomain.Interaction, uint, uint64, time.Duration, string) (string, *federationdomain.BrowserSession, error)
}
type PasswordVerifier interface {
	CheckPassword(string, string) (*userauthapp.PasswordIdentity, error)
}
type SecondFactorCompleter interface {
	Complete(context.Context, *federationdomain.Interaction, string, string, time.Duration, string) (string, *federationdomain.BrowserSession, error)
}

type Service struct {
	store                      Store
	password                   PasswordVerifier
	secondFactor               SecondFactorCompleter
	lifetime, rememberLifetime time.Duration
}

func New(store Store, password PasswordVerifier, secondFactor SecondFactorCompleter, lifetime, rememberLifetime time.Duration) (*Service, error) {
	if store == nil || password == nil || secondFactor == nil || lifetime <= 0 || rememberLifetime < lifetime || rememberLifetime > 30*24*time.Hour {
		return nil, errors.New("invalid federation authentication configuration")
	}
	return &Service{store: store, password: password, secondFactor: secondFactor, lifetime: lifetime, rememberLifetime: rememberLifetime}, nil
}

type Binding struct {
	RequestID, ClientID, Browser, CSRF string
	PreviousSessionCookie              string
}
type Result struct {
	RequiresSecondFactor bool
	// Only the host-only HTTP cookie adapter may read these fields. No protocol
	// token or session cookie is serialized into a frontend JSON response.
	Cookie  string                           `json:"-"`
	Session *federationdomain.BrowserSession `json:"-"`
}

func (s *Service) duration(remember bool) time.Duration {
	if remember {
		return s.rememberLifetime
	}
	return s.lifetime
}

// Password runs after the transport has checked its captcha, origin, request
// size and per-client/IP rate limits. Binding is still verified on every step.
func (s *Service) Password(ctx context.Context, binding Binding, email, password string, remember bool) (*Result, error) {
	value, err := s.store.ReadBound(ctx, binding.RequestID, binding.ClientID, binding.Browser, binding.CSRF)
	if err != nil {
		return nil, err
	}
	if value.Stage != federationdomain.StagePending {
		return nil, federationdomain.ErrInteraction
	}
	identity, err := s.password.CheckPassword(email, password)
	if err != nil {
		if attemptErr := s.store.RejectAttempt(ctx, value); attemptErr != nil {
			return nil, attemptErr
		}
		return nil, err
	}
	if identity == nil || identity.UserID == 0 {
		return nil, federationdomain.ErrInteraction
	}
	if identity.RequiresTOTP {
		if err := s.store.AwaitSecondFactor(ctx, value, identity.UserID, identity.TokenVersion, remember); err != nil {
			return nil, err
		}
		return &Result{RequiresSecondFactor: true}, nil
	}
	cookie, session, err := s.store.AuthenticatePassword(ctx, value, identity.UserID, identity.TokenVersion, s.duration(remember), binding.PreviousSessionCookie)
	if err != nil {
		return nil, err
	}
	return &Result{Cookie: cookie, Session: session}, nil
}

func (s *Service) SecondFactor(ctx context.Context, binding Binding, code, recovery string) (*Result, error) {
	value, err := s.store.ReadBound(ctx, binding.RequestID, binding.ClientID, binding.Browser, binding.CSRF)
	if err != nil {
		return nil, err
	}
	if value.Stage != federationdomain.StageSecondFactor {
		return nil, federationdomain.ErrInteraction
	}
	cookie, session, err := s.secondFactor.Complete(ctx, value, code, recovery, s.duration(value.RememberMe), binding.PreviousSessionCookie)
	if err != nil {
		return nil, err
	}
	return &Result{Cookie: cookie, Session: session}, nil
}
