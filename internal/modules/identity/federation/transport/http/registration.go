package federationhttp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

type RegistrationService interface {
	SendRegistrationCodeForSite(context.Context, string, string, mailbrand.Brand) error
	Register(string, string, string, bool, bool) (*userdomain.User, string, time.Time, error)
}

// registrationCode is a server-to-server adapter, never a public mail proxy.
// It consumes the main site's original captcha/risk approval and binds the
// destination email to the pending, browser-bound OIDC interaction.
func (s *Server) registrationCode(w http.ResponseWriter, r *http.Request) {
	if s.options.Registration == nil || s.options.RegistrationEnabled == nil {
		s.fail(w, r, 404, "registration_disabled")
		return
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		s.fail(w, r, 403, "registration_method")
		return
	}
	client, credential, ok := r.BasicAuth()
	if !ok || s.options.Protocol.AuthorizeClientIDSecret(r.Context(), client, credential) != nil {
		s.fail(w, r, 401, "registration_client")
		return
	}
	form, configured := s.forms[client]
	challenge, bridged := s.options.Challenges[client]
	if !configured || !bridged {
		s.fail(w, r, 403, "registration_client")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		s.fail(w, r, 415, "registration_type")
		return
	}
	var input struct {
		ID      string `json:"auth_request_id"`
		State   string `json:"state"`
		Email   string `json:"email"`
		Locale  string `json:"locale"`
		Receipt string `json:"receipt"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !secretValue(input.ID) || !secretValue(input.State) || !secretValue(input.Receipt) || len(input.Email) > 254 || len(input.Locale) > 16 {
		s.fail(w, r, 400, "registration_body")
		return
	}
	value, protocol, err := s.request(r, input.ID)
	if err != nil || value.ClientID != client || value.Stage != "pending" || subtle.ConstantTimeCompare([]byte(protocol.State), []byte(input.State)) != 1 {
		s.fail(w, r, 403, "registration_binding")
		return
	}
	enabled, err := s.options.RegistrationEnabled()
	if err != nil {
		s.fail(w, r, 503, "registration_settings")
		return
	}
	if !enabled {
		s.fail(w, r, 403, "registration_closed")
		return
	}
	retryAfter, expiresIn := 60, 600
	if policy, ok := s.options.Registration.(interface{ RegistrationCodePolicy() (int, int) }); ok {
		retryAfter, expiresIn = policy.RegistrationCodePolicy()
	}
	if challenge.Verify(r.Context(), input.ID, input.State, input.Receipt, input.Email) != nil {
		s.fail(w, r, 403, "registration_challenge")
		return
	}
	brand := mailbrand.Brand{SiteName: "绘画荟", SiteURL: form.Scheme + "://" + form.Host, FromName: "绘画荟"}
	err = s.options.Registration.SendRegistrationCodeForSite(r.Context(), input.Email, input.Locale, brand)
	if err != nil {
		status, code := http.StatusServiceUnavailable, "mail_unavailable"
		switch {
		case errors.Is(err, userauthapp.ErrVerifyCodeTooFrequent):
			status, code = 429, "too_frequent"
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		case errors.Is(err, userauthapp.ErrEmailExists):
			status, code = 409, "email_exists"
		case errors.Is(err, userauthapp.ErrInvalidEmail):
			status, code = 400, "invalid_email"
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	length := 6
	if policy, ok := s.options.Registration.(interface{ RegistrationCodeLength() int }); ok {
		length = policy.RegistrationCodeLength()
	}
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	if value.ExpiresAt.Before(deadline) {
		deadline = value.ExpiresAt
	}
	token := registrationCheckToken(credential, client, input.ID, input.State, input.Email, deadline)
	_ = json.NewEncoder(w).Encode(map[string]any{"sent": true, "retry_after": retryAfter, "expires_in": expiresIn, "code_length": length, "check_token": token})
}

func (s *Server) registerAccount(r *http.Request) error {
	if s.options.Registration == nil || s.options.RegistrationEnabled == nil {
		return userauthapp.ErrRegistrationDisabled
	}
	enabled, err := s.options.RegistrationEnabled()
	if err != nil || !enabled {
		if err != nil {
			return err
		}
		return userauthapp.ErrRegistrationDisabled
	}
	password := r.PostForm.Get("password")
	code := strings.TrimSpace(r.PostForm.Get("code"))
	if password != r.PostForm.Get("password_confirm") {
		return userauthapp.ErrInvalidPassword
	}
	if code == "" || len(code) > 32 {
		return userauthapp.ErrVerifyCodeInvalid
	}
	// Force verification regardless of the storefront's optional legacy setting.
	// Register atomically consumes the code and creates the Shop identity.
	_, _, _, err = s.options.Registration.Register(r.PostForm.Get("email"), password, code, true, true)
	return err
}
