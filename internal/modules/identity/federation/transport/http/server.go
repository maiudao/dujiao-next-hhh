// Package federationhttp provides the browser boundary around the durable OIDC
// adapter. Bootstrap must explicitly supply trusted proxy/rate/captcha wiring.
package federationhttp

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dujiao-next/internal/constants"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	federationapp "github.com/dujiao-next/internal/modules/identity/federation/application"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	"github.com/dujiao-next/internal/modules/identity/federation/infrastructure/oidcprovider"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

const (
	BrowserCookieName = "__Host-huihua-identity-browser"
	SessionCookieName = "__Host-huihua-identity-session"
	maxBody           = 16 * 1024
)

type CaptchaVerifier interface {
	Verify(string, captchahttp.CaptchaPayloadRequest, string) error
}
type UIClient struct{ ID, FormURL string }
type Options struct {
	Issuer   string
	Provider *op.Provider
	Protocol *oidcprovider.Storage
	Store    *federationstore.Store
	Auth     *federationapp.Service
	Clients  []UIClient
	Captcha  CaptchaVerifier
	// These callbacks must use the application's trusted proxy and existing
	// shared limiter. There is no permissive fallback when they are absent.
	ClientIP  func(*http.Request) string
	RateLimit func(*http.Request, string, string) error
	Logger    *slog.Logger
	Audit     func(*http.Request, LoginEvent)
}

type LoginEvent struct {
	ClientID string
	UserID   uint
	Success  bool
}

type Server struct {
	options Options
	issuer  *url.URL
	forms   map[string]*url.URL
	handler http.Handler
}

func secureURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.ForceQuery || u.Opaque != "" || strings.ContainsAny(raw, "*\\\r\n\t ") {
		return nil, errors.New("invalid identity HTTP URL")
	}
	return u, nil
}

func New(options Options) (*Server, error) {
	issuer, err := secureURL(options.Issuer)
	if err != nil || options.Provider == nil || options.Protocol == nil || options.Store == nil || options.Auth == nil || options.Captcha == nil || options.ClientIP == nil || options.RateLimit == nil || options.Logger == nil || len(options.Clients) == 0 {
		return nil, errors.New("incomplete identity HTTP configuration")
	}
	if options.Provider.Storage() != options.Protocol {
		return nil, errors.New("identity provider storage mismatch")
	}
	issuer.Path = strings.TrimSuffix(issuer.Path, "/")
	s := &Server{options: options, issuer: issuer, forms: make(map[string]*url.URL)}
	for _, client := range options.Clients {
		form, err := secureURL(client.FormURL)
		if err != nil || form.Path == "" {
			return nil, errors.New("invalid identity form URL")
		}
		registered, err := options.Protocol.GetClientByClientID(context.Background(), client.ID)
		if err != nil {
			return nil, errors.New("unregistered identity UI client")
		}
		// All library login redirects first reach our bound continuation endpoint.
		login, err := url.Parse(registered.LoginURL("probe"))
		if err != nil || login.Scheme != issuer.Scheme || login.Host != issuer.Host || login.Path != issuer.Path+"/login/continue" {
			return nil, errors.New("identity continuation URL mismatch")
		}
		if _, exists := s.forms[client.ID]; exists {
			return nil, errors.New("duplicate identity UI client")
		}
		s.forms[client.ID] = form
	}
	s.handler = http.StripPrefix(issuer.Path, options.Provider)
	return s, nil
}

func secretValue(value string) bool {
	data, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(data) == 32 && base64.RawURLEncoding.EncodeToString(data) == value
}

func cookieValue(r *http.Request, name string) (string, error) {
	var value string
	count := 0
	for _, cookie := range r.Cookies() {
		if cookie.Name == name {
			count++
			value = cookie.Value
		}
	}
	if count > 1 || (count == 1 && !secretValue(value)) {
		return "", federationdomain.ErrInteraction
	}
	return value, nil
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, action string) {
	// Only fixed actions/status are logged. URL/query/body, credentials and raw
	// upstream errors are deliberately excluded even on protocol failures.
	s.options.Logger.WarnContext(r.Context(), "identity request rejected", "action", action, "status", status)
	http.Error(w, "登录暂时无法完成，请返回重试。", status)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !strings.EqualFold(r.Host, s.issuer.Host) || len(r.URL.RawQuery) > 8192 || (!strings.HasPrefix(r.URL.Path, s.issuer.Path+"/") && r.URL.Path != s.issuer.Path) {
		s.fail(w, r, http.StatusBadRequest, "host_or_path")
		return
	}
	if r.Body == nil {
		r.Body = http.NoBody
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	path := strings.TrimPrefix(r.URL.Path, s.issuer.Path)
	if err := s.options.RateLimit(r, "identity", ""); err != nil {
		s.fail(w, r, http.StatusTooManyRequests, "rate_limit")
		return
	}
	if path == "/interaction" {
		s.interaction(w, r)
		return
	}
	browser, err := cookieValue(r, BrowserCookieName)
	if err != nil {
		s.fail(w, r, http.StatusForbidden, "browser_cookie")
		return
	}
	session, err := cookieValue(r, SessionCookieName)
	if err != nil {
		s.fail(w, r, http.StatusForbidden, "session_cookie")
		return
	}
	csrf := ""
	switch path {
	case s.options.Provider.AuthorizationEndpoint().Relative():
		if r.Method != http.MethodGet {
			s.fail(w, r, http.StatusMethodNotAllowed, "authorize_method")
			return
		}
		if r.URL.Query().Get("id_token_hint") != "" {
			s.fail(w, r, http.StatusBadRequest, "authorize_hint")
			return
		}
		if browser == "" {
			browser, err = federationstore.Secret()
			if err != nil {
				s.fail(w, r, http.StatusInternalServerError, "browser_create")
				return
			}
			http.SetCookie(w, &http.Cookie{Name: BrowserCookieName, Value: browser, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60})
		}
		// First-party RP state is a 256-bit random value and doubles as the
		// form's synchronizer token. Authentication still requires this browser.
		csrf = r.URL.Query().Get("state")
	case "/login/password", "/login/second-factor", "/login/cancel":
		if r.Method != http.MethodPost || r.URL.RawQuery != "" {
			s.fail(w, r, http.StatusMethodNotAllowed, "form_method")
			return
		}
		media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || media != "application/x-www-form-urlencoded" {
			s.fail(w, r, http.StatusUnsupportedMediaType, "form_type")
			return
		}
		if err := r.ParseForm(); err != nil {
			s.fail(w, r, http.StatusBadRequest, "form_size")
			return
		}
		csrf = r.PostForm.Get("csrf")
	default:
		if path == s.options.Provider.EndSessionEndpoint().Relative() {
			if r.Method != http.MethodPost || r.URL.RawQuery != "" {
				s.fail(w, r, http.StatusMethodNotAllowed, "logout_method")
				return
			}
			media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || media != "application/x-www-form-urlencoded" || r.ParseForm() != nil {
				s.fail(w, r, http.StatusBadRequest, "logout_body")
				return
			}
			form, ok := s.forms[r.PostForm.Get("client_id")]
			if !ok || r.Header.Get("Origin") != form.Scheme+"://"+form.Host || !secretValue(r.PostForm.Get("state")) {
				s.fail(w, r, http.StatusForbidden, "logout_origin")
				return
			}
		}
		if path == s.options.Provider.TokenEndpoint().Relative() || path == s.options.Provider.IntrospectionEndpoint().Relative() || path == s.options.Provider.RevocationEndpoint().Relative() {
			if r.Method != http.MethodPost || r.URL.RawQuery != "" {
				s.fail(w, r, http.StatusMethodNotAllowed, "token_method")
				return
			}
		}
		if path == s.options.Provider.UserinfoEndpoint().Relative() && r.URL.RawQuery != "" {
			s.fail(w, r, http.StatusBadRequest, "userinfo_query")
			return
		}
		if path == s.options.Provider.AuthorizationEndpoint().Relative()+"/callback" || path == "/login/continue" {
			if r.Method != http.MethodGet {
				s.fail(w, r, http.StatusMethodNotAllowed, "continue_method")
				return
			}
			id := r.URL.Query().Get("id")
			if path == "/login/continue" {
				id = r.URL.Query().Get("authRequestID")
			}
			_, protocol, err := s.request(r, id)
			if err != nil {
				s.fail(w, r, http.StatusForbidden, "continue_request")
				return
			}
			csrf = protocol.State
		}
	}
	r = r.WithContext(oidcprovider.WithBrowser(r.Context(), oidcprovider.BrowserBinding{Browser: browser, CSRF: csrf, SessionCookie: session}))
	if path == "/login/continue" {
		s.continueLogin(w, r)
		return
	}
	if path == "/login/password" || path == "/login/second-factor" || path == "/login/cancel" {
		s.form(w, r, path, browser, csrf)
		return
	}
	if path == s.options.Provider.EndSessionEndpoint().Relative() {
		s.handler.ServeHTTP(logoutWriter{ResponseWriter: w}, r)
		return
	}
	s.handler.ServeHTTP(w, r)
}

type logoutWriter struct{ http.ResponseWriter }

func (w logoutWriter) WriteHeader(status int) {
	if status == http.StatusFound || status == http.StatusSeeOther {
		http.SetCookie(w.ResponseWriter, &http.Cookie{Name: SessionCookieName, Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
	w.ResponseWriter.WriteHeader(status)
}

func (s *Server) request(r *http.Request, id string) (*federationdomain.Interaction, *oidc.AuthRequest, error) {
	if !secretValue(id) {
		return nil, nil, federationdomain.ErrInteraction
	}
	value, err := s.options.Store.Request(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	var protocol oidc.AuthRequest
	if err := json.Unmarshal([]byte(value.RequestJSON), &protocol); err != nil {
		return nil, nil, err
	}
	if _, ok := s.forms[value.ClientID]; !ok || protocol.ClientID != value.ClientID {
		return nil, nil, federationdomain.ErrInteraction
	}
	return value, &protocol, nil
}

func (s *Server) returnForm(w http.ResponseWriter, r *http.Request, value *federationdomain.Interaction, protocol *oidc.AuthRequest, failed bool) {
	target := *s.forms[value.ClientID]
	query := target.Query()
	query.Set("authRequestID", value.ID)
	query.Set("state", protocol.State)
	if value.Stage == federationdomain.StageSecondFactor {
		query.Set("step", "second_factor")
	}
	if failed {
		query.Set("error", "authentication_failed")
	}
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

func (s *Server) callback(w http.ResponseWriter, r *http.Request, id string) {
	target := s.issuer.Path + s.options.Provider.AuthorizationEndpoint().Relative() + "/callback?id=" + url.QueryEscape(id)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) continueLogin(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("authRequestID")
	value, protocol, err := s.request(r, id)
	if err != nil {
		s.fail(w, r, http.StatusForbidden, "continue_request")
		return
	}
	request, err := s.options.Protocol.AuthRequestByID(r.Context(), id)
	if err != nil {
		s.fail(w, r, http.StatusForbidden, "continue_binding")
		return
	}
	if request.Done() {
		s.callback(w, r, id)
		return
	}
	s.returnForm(w, r, value, protocol, false)
}

func (s *Server) form(w http.ResponseWriter, r *http.Request, path, browser, csrf string) {
	id := r.PostForm.Get("authRequestID")
	value, protocol, err := s.request(r, id)
	if err != nil {
		s.fail(w, r, http.StatusForbidden, "form_request")
		return
	}
	expected := s.forms[value.ClientID]
	if r.Header.Get("Origin") != expected.Scheme+"://"+expected.Host {
		s.fail(w, r, http.StatusForbidden, "form_origin")
		return
	}
	if subtle.ConstantTimeCompare([]byte(csrf), []byte(protocol.State)) != 1 {
		s.fail(w, r, http.StatusForbidden, "form_csrf")
		return
	}
	value, err = s.options.Store.ReadBound(r.Context(), id, value.ClientID, browser, csrf)
	if err != nil {
		s.fail(w, r, http.StatusForbidden, "form_binding")
		return
	}
	if err := s.options.RateLimit(r, "identity_form", value.ClientID); err != nil {
		s.fail(w, r, http.StatusTooManyRequests, "form_rate")
		return
	}
	if path == "/login/cancel" {
		if err := s.options.Store.Cancel(r.Context(), value); err != nil {
			s.fail(w, r, http.StatusForbidden, "cancel")
			return
		}
		target, err := url.Parse(protocol.RedirectURI)
		if err != nil {
			s.fail(w, r, http.StatusInternalServerError, "cancel_redirect")
			return
		}
		query := target.Query()
		query.Set("error", "access_denied")
		query.Set("state", protocol.State)
		target.RawQuery = query.Encode()
		http.Redirect(w, r, target.String(), http.StatusSeeOther)
		return
	}
	previous, _ := cookieValue(r, SessionCookieName)
	binding := federationapp.Binding{RequestID: id, ClientID: value.ClientID, Browser: browser, CSRF: csrf, PreviousSessionCookie: previous}
	var result *federationapp.Result
	remember := value.RememberMe
	if path == "/login/password" {
		payload := captchahttp.CaptchaPayloadRequest{CaptchaID: r.PostForm.Get("captcha_id"), CaptchaCode: r.PostForm.Get("captcha_code"), TurnstileToken: r.PostForm.Get("turnstile_token")}
		if err := s.options.Captcha.Verify(constants.CaptchaSceneLogin, payload, s.options.ClientIP(r)); err != nil {
			s.fail(w, r, http.StatusForbidden, "captcha")
			return
		}
		email, password := r.PostForm.Get("email"), r.PostForm.Get("password")
		if len(email) > 254 || len(password) > 1024 {
			s.fail(w, r, http.StatusBadRequest, "credential_size")
			return
		}
		remember = r.PostForm.Get("remember_me") == "true"
		result, err = s.options.Auth.Password(r.Context(), binding, email, password, remember)
	} else {
		result, err = s.options.Auth.SecondFactor(r.Context(), binding, r.PostForm.Get("code"), r.PostForm.Get("recovery_code"))
	}
	if err != nil {
		if s.options.Audit != nil {
			s.options.Audit(r, LoginEvent{ClientID: value.ClientID})
		}
		fresh, _, lookupErr := s.request(r, id)
		if lookupErr != nil {
			s.fail(w, r, http.StatusUnauthorized, "authentication")
			return
		}
		s.options.Logger.WarnContext(r.Context(), "identity authentication failed", "client", value.ClientID)
		s.returnForm(w, r, fresh, protocol, true)
		return
	}
	if result.RequiresSecondFactor {
		fresh, _, err := s.request(r, id)
		if err != nil {
			s.fail(w, r, http.StatusForbidden, "second_factor_state")
			return
		}
		s.returnForm(w, r, fresh, protocol, false)
		return
	}
	if result.Session == nil || result.Cookie == "" {
		s.fail(w, r, http.StatusInternalServerError, "session_result")
		return
	}
	cookie := &http.Cookie{Name: SessionCookieName, Value: result.Cookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if remember {
		cookie.MaxAge = int(time.Until(result.Session.ExpiresAt).Seconds())
		cookie.Expires = result.Session.ExpiresAt
	}
	http.SetCookie(w, cookie)
	if s.options.Audit != nil {
		s.options.Audit(r, LoginEvent{ClientID: value.ClientID, UserID: result.Session.UserID, Success: true})
	}
	s.options.Logger.InfoContext(r.Context(), "identity authentication completed", "client", value.ClientID, "user_id", result.Session.UserID)
	s.callback(w, r, id)
}

// interaction is a client-authenticated, read-only bridge used by the main RP
// to match its own state/nonce/PKCE before showing the original form. It grants
// no identity and cannot replace the browser cookie binding on form submission.
func (s *Server) interaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Origin") != "" || r.URL.RawQuery != "" {
		s.fail(w, r, http.StatusMethodNotAllowed, "interaction_method")
		return
	}
	clientID, clientSecret, ok := r.BasicAuth()
	if !ok || s.options.Protocol.AuthorizeClientIDSecret(r.Context(), clientID, clientSecret) != nil {
		s.fail(w, r, http.StatusUnauthorized, "interaction_client")
		return
	}
	var input struct {
		ID    string `json:"id"`
		State string `json:"state"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		s.fail(w, r, http.StatusBadRequest, "interaction_body")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		s.fail(w, r, http.StatusBadRequest, "interaction_body")
		return
	}
	value, protocol, err := s.request(r, input.ID)
	if err != nil || value.ClientID != clientID || subtle.ConstantTimeCompare([]byte(input.State), []byte(protocol.State)) != 1 {
		s.fail(w, r, http.StatusForbidden, "interaction_binding")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	action := "/login/password"
	if value.Stage == federationdomain.StageSecondFactor {
		action = "/login/second-factor"
	}
	_ = json.NewEncoder(w).Encode(InteractionResponse{ID: value.ID, ClientID: value.ClientID, State: protocol.State, Nonce: protocol.Nonce, CodeChallenge: protocol.CodeChallenge, CodeChallengeMethod: string(protocol.CodeChallengeMethod), Stage: value.Stage, FormAction: strings.TrimSuffix(s.options.Issuer, "/") + action, CancelAction: strings.TrimSuffix(s.options.Issuer, "/") + "/login/cancel"})
}

type InteractionResponse struct {
	ID                  string `json:"auth_request_id"`
	ClientID            string `json:"client_id"`
	State               string `json:"state"`
	Nonce               string `json:"nonce"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Stage               string `json:"stage"`
	FormAction          string `json:"form_action"`
	CancelAction        string `json:"cancel_action"`
}
