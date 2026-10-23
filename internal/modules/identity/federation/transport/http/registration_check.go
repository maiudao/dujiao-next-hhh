package federationhttp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
)

// A send capability authorizes previews only for the email and pending flow
// that passed the original puzzle/risk checks. It is never an account proof.
func registrationCheckToken(key, client, id, state, email string, deadline time.Time) string {
	expires := strconv.FormatInt(deadline.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(key))
	for _, value := range []string{"registration-code-preview", client, id, state, strings.ToLower(strings.TrimSpace(email)), expires} {
		_, _ = mac.Write([]byte(value))
		_, _ = mac.Write([]byte{0})
	}
	return expires + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func validRegistrationCheckToken(token, key, client, id, state, email string, now time.Time) bool {
	if len(token) > 128 {
		return false
	}
	expires, _, ok := strings.Cut(token, ".")
	seconds, err := strconv.ParseInt(expires, 10, 64)
	if !ok || err != nil || seconds <= now.Unix() || seconds > now.Add(24*time.Hour).Unix() {
		return false
	}
	expected := registrationCheckToken(key, client, id, state, email, time.Unix(seconds, 0))
	return hmac.Equal([]byte(token), []byte(expected))
}

func (s *Server) registrationCodeCheck(w http.ResponseWriter, r *http.Request) {
	checker, ok := s.options.Registration.(interface{ CheckRegistrationCode(string, string) error })
	if !ok || s.options.RegistrationEnabled == nil {
		s.fail(w, r, http.StatusNotFound, "registration_disabled")
		return
	}
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		s.fail(w, r, http.StatusForbidden, "registration_check_method")
		return
	}
	client, credential, authorized := r.BasicAuth()
	if !authorized || s.options.Protocol.AuthorizeClientIDSecret(r.Context(), client, credential) != nil {
		s.fail(w, r, http.StatusUnauthorized, "registration_check_client")
		return
	}
	if _, configured := s.forms[client]; !configured || s.options.Challenges[client] == nil {
		s.fail(w, r, http.StatusForbidden, "registration_check_client")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		s.fail(w, r, http.StatusUnsupportedMediaType, "registration_check_type")
		return
	}
	var input struct {
		ID         string `json:"auth_request_id"`
		State      string `json:"state"`
		Email      string `json:"email"`
		Code       string `json:"code"`
		CheckToken string `json:"check_token"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !secretValue(input.ID) || !secretValue(input.State) || len(input.Email) > 254 || len(input.Code) < 4 || len(input.Code) > 10 {
		s.fail(w, r, http.StatusBadRequest, "registration_check_body")
		return
	}
	for _, digit := range input.Code {
		if digit < '0' || digit > '9' {
			s.fail(w, r, http.StatusBadRequest, "registration_check_body")
			return
		}
	}
	value, protocol, err := s.request(r, input.ID)
	if err != nil || value.ClientID != client || value.Stage != "pending" || !hmac.Equal([]byte(protocol.State), []byte(input.State)) || !validRegistrationCheckToken(input.CheckToken, credential, client, input.ID, input.State, input.Email, time.Now()) {
		s.fail(w, r, http.StatusForbidden, "registration_check_binding")
		return
	}
	enabled, err := s.options.RegistrationEnabled()
	if err != nil || !enabled {
		s.fail(w, r, http.StatusForbidden, "registration_closed")
		return
	}
	err = checker.CheckRegistrationCode(input.Email, input.Code)
	status, reason := http.StatusOK, ""
	if err != nil {
		status, reason = http.StatusServiceUnavailable, "check_unavailable"
		switch {
		case errors.Is(err, userauthapp.ErrVerifyCodeInvalid):
			status, reason = http.StatusUnprocessableEntity, "invalid_code"
		case errors.Is(err, userauthapp.ErrVerifyCodeExpired):
			status, reason = http.StatusUnprocessableEntity, "expired_code"
		case errors.Is(err, userauthapp.ErrVerifyCodeAttemptsExceeded):
			status, reason = http.StatusTooManyRequests, "attempts_exceeded"
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"valid": err == nil, "error": reason})
}
