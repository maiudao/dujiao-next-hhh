// Package oidcprovider adapts the mature OIDC provider to Shop's durable identity
// records. It is deliberately not wired into bootstrap until UI, RP and legacy
// account ownership have passed the cross-site acceptance checks.
package oidcprovider

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	"github.com/go-jose/go-jose/v4"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

type browserKey struct{}

// BrowserBinding is populated only by the host-only-cookie/CSRF HTTP adapter.
// It must never be built from arbitrary client IDs, subjects or URL credentials.
type BrowserBinding struct{ Browser, CSRF, SessionCookie string }

func WithBrowser(ctx context.Context, binding BrowserBinding) context.Context {
	return context.WithValue(ctx, browserKey{}, binding)
}
func browser(ctx context.Context) BrowserBinding {
	value, _ := ctx.Value(browserKey{}).(BrowserBinding)
	return value
}

type Storage struct {
	store   *federationstore.Store
	clients map[string]client
	key     *rsa.PrivateKey
	keyID   string
}

var _ op.Storage = (*Storage)(nil)
var _ op.CanSetUserinfoFromRequest = (*Storage)(nil)
var _ op.CanTerminateSessionFromRequest = (*Storage)(nil)

func New(store *federationstore.Store, configs []ClientConfig, key *rsa.PrivateKey, keyID string) (*Storage, error) {
	if store == nil || key == nil || key.N == nil || key.N.BitLen() < 2048 || keyID == "" || len(configs) == 0 {
		return nil, errors.New("invalid identity provider configuration")
	}
	if err := key.Validate(); err != nil {
		return nil, errors.New("invalid identity signing key")
	}
	s := &Storage{store: store, clients: make(map[string]client), key: key, keyID: keyID}
	for _, config := range configs {
		if strings.TrimSpace(config.ID) == "" || len(config.Secret) < 32 || !exactHTTPS(config.RedirectURI) || !exactHTTPS(config.PostLogoutRedirectURI) || !exactHTTPS(config.LoginURL) {
			return nil, errors.New("invalid identity client registration")
		}
		if _, exists := s.clients[config.ID]; exists {
			return nil, errors.New("duplicate identity client")
		}
		s.clients[config.ID] = client{config: config}
	}
	return s, nil
}

type request struct {
	protocol  oidc.AuthRequest
	record    *federationdomain.Interaction
	session   *federationdomain.BrowserSession
	codeGrant bool
}

func (r *request) GetID() string  { return r.record.ID }
func (r *request) GetACR() string { return "" }
func (r *request) GetAMR() []string {
	if r.session == nil {
		return nil
	}
	return strings.Fields(r.session.AMR)
}
func (r *request) GetAudience() []string { return []string{r.protocol.ClientID} }
func (r *request) GetAuthTime() time.Time {
	if r.session == nil {
		return time.Time{}
	}
	return r.session.AuthenticatedAt
}
func (r *request) GetClientID() string { return r.protocol.ClientID }
func (r *request) GetCodeChallenge() *oidc.CodeChallenge {
	return &oidc.CodeChallenge{Challenge: r.protocol.CodeChallenge, Method: r.protocol.CodeChallengeMethod}
}
func (r *request) GetNonce() string                   { return r.protocol.Nonce }
func (r *request) GetRedirectURI() string             { return r.protocol.RedirectURI }
func (r *request) GetResponseType() oidc.ResponseType { return r.protocol.ResponseType }
func (r *request) GetResponseMode() oidc.ResponseMode { return r.protocol.ResponseMode }
func (r *request) GetScopes() []string                { return slices.Clone([]string(r.protocol.Scopes)) }
func (r *request) GetState() string                   { return r.protocol.State }
func (r *request) GetSubject() string                 { return r.record.SubjectID }
func (r *request) Done() bool {
	return r.session != nil && (r.record.Stage == federationstore.StageAuthenticated || r.record.Stage == federationstore.StageCodeIssued)
}

func (s *Storage) hydrate(ctx context.Context, value *federationdomain.Interaction, codeGrant bool) (*request, error) {
	r := &request{record: value, codeGrant: codeGrant}
	if err := json.Unmarshal([]byte(value.RequestJSON), &r.protocol); err != nil {
		return nil, federationstore.ErrInteraction
	}
	if value.ClientID != r.protocol.ClientID {
		return nil, federationstore.ErrInteraction
	}
	if value.SessionID != "" {
		var err error
		r.session, err = s.store.SessionByID(ctx, value.SessionID)
		if err != nil {
			return nil, err
		}
		user, err := s.store.UserForSubject(ctx, value.SubjectID)
		if err != nil || user.ID != r.session.UserID {
			return nil, federationstore.ErrInteraction
		}
	}
	return r, nil
}

func (s *Storage) CreateAuthRequest(ctx context.Context, protocol *oidc.AuthRequest, _ string) (op.AuthRequest, error) {
	if protocol == nil {
		return nil, oidc.ErrInvalidRequest()
	}
	registered, ok := s.clients[protocol.ClientID]
	challenge, err := base64.RawURLEncoding.DecodeString(protocol.CodeChallenge)
	if !ok || protocol.RedirectURI != registered.config.RedirectURI || protocol.ResponseType != oidc.ResponseTypeCode || (protocol.ResponseMode != "" && protocol.ResponseMode != oidc.ResponseModeQuery) || protocol.CodeChallengeMethod != oidc.CodeChallengeMethodS256 || err != nil || len(challenge) != sha256.Size || base64.RawURLEncoding.EncodeToString(challenge) != protocol.CodeChallenge || len(protocol.State) < 32 || len(protocol.State) > 512 || len(protocol.Nonce) < 32 || len(protocol.Nonce) > 512 || !slices.Contains(protocol.Scopes, oidc.ScopeOpenID) || len(protocol.Scopes) > 3 || len(protocol.ACRValues) != 0 || protocol.RequestParam != "" {
		return nil, oidc.ErrInvalidRequest().WithDescription("invalid first-party authorization request")
	}
	for _, scope := range protocol.Scopes {
		if !allowedScope(scope) {
			return nil, oidc.ErrInvalidScope()
		}
	}
	for _, prompt := range protocol.Prompt {
		if prompt != oidc.PromptNone && prompt != oidc.PromptLogin {
			return nil, oidc.ErrInvalidRequest().WithDescription("unsupported prompt")
		}
	}
	if slices.Contains(protocol.Prompt, oidc.PromptNone) && len(protocol.Prompt) != 1 {
		return nil, oidc.ErrInvalidRequest()
	}
	binding := browser(ctx)
	// ID token hints, login hints and browser credentials are not persisted.
	copyProtocol := *protocol
	copyProtocol.IDTokenHint = ""
	copyProtocol.LoginHint = ""
	data, err := json.Marshal(copyProtocol)
	if err != nil {
		return nil, err
	}
	value, err := s.store.Begin(ctx, protocol.ClientID, data, binding.Browser, binding.CSRF)
	if err != nil {
		return nil, err
	}
	if binding.SessionCookie != "" && !slices.Contains(protocol.Prompt, oidc.PromptLogin) {
		session, sessionErr := s.store.Session(ctx, binding.SessionCookie)
		if sessionErr != nil && !errors.Is(sessionErr, federationstore.ErrInteraction) {
			return nil, sessionErr
		}
		fresh := sessionErr == nil && (protocol.MaxAge == nil || (time.Since(session.AuthenticatedAt) >= 0 && time.Since(session.AuthenticatedAt).Seconds() < float64(*protocol.MaxAge)))
		if fresh {
			if err := s.store.ResumeSession(ctx, value, binding.SessionCookie); err != nil {
				return nil, err
			}
		}
	}
	value, err = s.store.Request(ctx, value.ID)
	if err != nil {
		return nil, err
	}
	result, err := s.hydrate(ctx, value, false)
	if err != nil {
		return nil, err
	}
	if slices.Contains(protocol.Prompt, oidc.PromptNone) && !result.Done() {
		_ = s.store.Cancel(ctx, value)
		return nil, oidc.ErrLoginRequired()
	}
	return result, nil
}

func (s *Storage) AuthRequestByID(ctx context.Context, id string) (op.AuthRequest, error) {
	value, err := s.store.Request(ctx, id)
	if err != nil {
		return nil, err
	}
	binding := browser(ctx)
	value, err = s.store.ReadBound(ctx, id, value.ClientID, binding.Browser, binding.CSRF)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, value, false)
}
func (s *Storage) AuthRequestByCode(ctx context.Context, code string) (op.AuthRequest, error) {
	value, err := s.store.RequestByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, value, true)
}
func (s *Storage) SaveAuthCode(ctx context.Context, id, code string) error {
	value, err := s.AuthRequestByID(ctx, id)
	if err != nil {
		return err
	}
	if !value.Done() {
		return federationstore.ErrInteraction
	}
	return s.store.SaveCode(ctx, id, code)
}
func (s *Storage) DeleteAuthRequest(ctx context.Context, id string) error {
	return s.store.FinishRequest(ctx, id)
}
func (s *Storage) CreateAccessToken(ctx context.Context, value op.TokenRequest) (string, time.Time, error) {
	r, ok := value.(*request)
	if !ok || !r.codeGrant || !r.Done() {
		return "", time.Time{}, oidc.ErrInvalidGrant()
	}
	scopes, err := json.Marshal(r.GetScopes())
	if err != nil {
		return "", time.Time{}, err
	}
	token, err := s.store.ConsumeCode(ctx, r.GetID(), r.GetClientID(), string(scopes))
	if err != nil {
		return "", time.Time{}, err
	}
	return token.ID, token.ExpiresAt, nil
}

func (s *Storage) GetClientByClientID(_ context.Context, id string) (op.Client, error) {
	value, ok := s.clients[id]
	if !ok {
		return nil, oidc.ErrInvalidClient()
	}
	return value, nil
}
func (s *Storage) AuthorizeClientIDSecret(_ context.Context, id, secret string) error {
	value, ok := s.clients[id]
	expected := sha256.Sum256([]byte(value.config.Secret))
	actual := sha256.Sum256([]byte(secret))
	if subtle.ConstantTimeCompare(expected[:], actual[:]) != 1 || !ok {
		return oidc.ErrInvalidClient()
	}
	return nil
}

func fillUserinfo(info *oidc.UserInfo, user *userdomain.User, subject, sid string, scopes []string) {
	info.Subject = subject
	info.AppendClaims("sid", sid)
	if slices.Contains(scopes, oidc.ScopeEmail) {
		info.Email = user.Email
		info.EmailVerified = oidc.Bool(user.HasTrustedEmailProof())
	}
	if slices.Contains(scopes, oidc.ScopeProfile) {
		info.Name = user.DisplayName
		info.Nickname = user.DisplayName
	}
}
func (s *Storage) SetUserinfoFromRequest(ctx context.Context, info *oidc.UserInfo, value op.IDTokenRequest, scopes []string) error {
	r, ok := value.(*request)
	if !ok || !r.Done() {
		return oidc.ErrInvalidGrant()
	}
	session, err := s.store.SessionByID(ctx, r.record.SessionID)
	if err != nil {
		return err
	}
	user, err := s.store.UserForSubject(ctx, r.GetSubject())
	if err != nil {
		return err
	}
	if user.ID != session.UserID {
		return federationstore.ErrInteraction
	}
	fillUserinfo(info, user, r.GetSubject(), session.ID, restrictScopes(scopes))
	return nil
}
func (*Storage) SetUserinfoFromScopes(context.Context, *oidc.UserInfo, string, string, []string) error {
	return nil
}
func (s *Storage) SetUserinfoFromToken(ctx context.Context, info *oidc.UserInfo, tokenID, subject, origin string) error {
	// Browser cross-origin userinfo is not part of this server-side RP contract.
	if origin != "" {
		return oidc.ErrAccessDenied()
	}
	token, err := s.store.LiveToken(ctx, tokenID, subject)
	if err != nil {
		return err
	}
	user, err := s.store.UserForSubject(ctx, subject)
	if err != nil {
		return err
	}
	var scopes []string
	if err := json.Unmarshal([]byte(token.ScopesJSON), &scopes); err != nil {
		return err
	}
	fillUserinfo(info, user, subject, token.SessionID, scopes)
	return nil
}
func (s *Storage) SetIntrospectionFromToken(ctx context.Context, info *oidc.IntrospectionResponse, tokenID, subject, clientID string) error {
	token, err := s.store.Token(ctx, tokenID, clientID, subject)
	if err != nil {
		return err
	}
	info.Active = true
	info.Subject = subject
	info.ClientID = clientID
	info.Audience = oidc.Audience{clientID}
	info.Expiration = oidc.FromTime(token.ExpiresAt)
	info.IssuedAt = oidc.FromTime(token.CreatedAt)
	info.TokenType = oidc.BearerToken
	var scopes []string
	if err := json.Unmarshal([]byte(token.ScopesJSON), &scopes); err != nil {
		return err
	}
	info.Scope = oidc.SpaceDelimitedArray(scopes)
	info.Claims = map[string]any{"sid": token.SessionID}
	return nil
}
func (*Storage) GetPrivateClaimsFromScopes(context.Context, string, string, []string) (map[string]any, error) {
	return nil, nil
}
func (*Storage) GetKeyByIDAndClientID(context.Context, string, string) (*jose.JSONWebKey, error) {
	return nil, oidc.ErrUnsupportedGrantType()
}
func (*Storage) ValidateJWTProfileScopes(context.Context, string, []string) ([]string, error) {
	return nil, oidc.ErrUnsupportedGrantType()
}
func (*Storage) CreateAccessAndRefreshTokens(context.Context, op.TokenRequest, string) (string, string, time.Time, error) {
	return "", "", time.Time{}, oidc.ErrUnsupportedGrantType()
}
func (*Storage) TokenRequestByRefreshToken(context.Context, string) (op.RefreshTokenRequest, error) {
	return nil, op.ErrInvalidRefreshToken
}
func (*Storage) GetRefreshTokenInfo(context.Context, string, string) (string, string, error) {
	return "", "", op.ErrInvalidRefreshToken
}
func (*Storage) TerminateSession(context.Context, string, string) error {
	return oidc.ErrAccessDenied()
}
func (s *Storage) TerminateSessionFromRequest(ctx context.Context, end *op.EndSessionRequest) (string, error) {
	// The verified hint and the actual current browser must agree. A subject or
	// client ID alone must never log out every device belonging to that account.
	if end == nil || end.IDTokenHintClaims == nil || end.IDTokenHintClaims.SessionID == "" {
		return "", oidc.ErrAccessDenied()
	}
	session, err := s.store.Session(ctx, browser(ctx).SessionCookie)
	if err != nil {
		return "", err
	}
	if session.ID != end.IDTokenHintClaims.SessionID {
		return "", oidc.ErrAccessDenied()
	}
	user, err := s.store.UserForSubject(ctx, end.UserID)
	if err != nil || user.ID != session.UserID {
		return "", oidc.ErrAccessDenied()
	}
	if err := s.store.RevokeSession(ctx, session.ID); err != nil {
		return "", err
	}
	return end.RedirectURI, nil
}
func (s *Storage) RevokeToken(ctx context.Context, id, subject, clientID string) *oidc.Error {
	if _, ok := s.clients[clientID]; !ok {
		return oidc.ErrInvalidClient()
	}
	if err := s.store.RevokeToken(ctx, id, clientID, subject); err != nil {
		return oidc.ErrServerError()
	}
	return nil
}
func (s *Storage) Health(ctx context.Context) error { return s.store.Health(ctx) }

type signingKey struct {
	id  string
	key *rsa.PrivateKey
}

func (k signingKey) ID() string                                { return k.id }
func (signingKey) SignatureAlgorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (k signingKey) Key() any                                  { return k.key }

type publicKey struct {
	id  string
	key *rsa.PublicKey
}

func (k publicKey) ID() string                       { return k.id }
func (publicKey) Algorithm() jose.SignatureAlgorithm { return jose.RS256 }
func (publicKey) Use() string                        { return "sig" }
func (k publicKey) Key() any                         { return k.key }
func (s *Storage) SigningKey(context.Context) (op.SigningKey, error) {
	return signingKey{s.keyID, s.key}, nil
}
func (*Storage) SignatureAlgorithms(context.Context) ([]jose.SignatureAlgorithm, error) {
	return []jose.SignatureAlgorithm{jose.RS256}, nil
}
func (s *Storage) KeySet(context.Context) ([]op.Key, error) {
	return []op.Key{publicKey{s.keyID, &s.key.PublicKey}}, nil
}
