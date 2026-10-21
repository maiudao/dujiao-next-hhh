package federationhttp

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
)

func (s *Server) clientSession(w http.ResponseWriter, r *http.Request, revoke bool) {
	// These endpoints never accept browser Cookie/Origin as authentication.
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		s.fail(w, r, http.StatusForbidden, "session_method")
		return
	}
	client, credential, ok := r.BasicAuth()
	if !ok || s.options.Protocol.AuthorizeClientIDSecret(r.Context(), client, credential) != nil {
		s.fail(w, r, http.StatusUnauthorized, "session_client")
		return
	}
	if _, ok := s.forms[client]; !ok {
		s.fail(w, r, http.StatusUnauthorized, "session_client")
		return
	}
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		s.fail(w, r, http.StatusUnsupportedMediaType, "session_type")
		return
	}
	var input struct {
		SessionID string `json:"session_id"`
		Subject   string `json:"subject"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4097))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF || !secretValue(input.SessionID) || !secretValue(input.Subject) {
		s.fail(w, r, http.StatusBadRequest, "session_body")
		return
	}
	if revoke {
		if err := s.options.Store.RevokeClientSession(r.Context(), client, input.SessionID, input.Subject); err != nil {
			status := http.StatusServiceUnavailable
			if errors.Is(err, federationdomain.ErrInteraction) {
				status = http.StatusForbidden
			}
			s.fail(w, r, status, "session_revoke")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	session, err := s.options.Store.ClientSession(r.Context(), client, input.SessionID, input.Subject)
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, federationdomain.ErrInteraction) {
			status = http.StatusUnauthorized
		}
		s.fail(w, r, status, "session_check")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		SessionID string `json:"session_id"`
		Subject   string `json:"subject"`
		ExpiresAt int64  `json:"expires_at"`
	}{session.ID, input.Subject, session.ExpiresAt.Unix()})
}
