package oidcprovider

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/zitadel/oidc/v3/pkg/oidc"
	"github.com/zitadel/oidc/v3/pkg/op"
)

// ClientConfig is an explicit first-party registration, not dynamic discovery.
// Secrets and signing keys must be loaded from the deployment's secret store.
type ClientConfig struct {
	ID                    string
	Secret                string
	RedirectURI           string
	PostLogoutRedirectURI string
	LoginURL              string
}

type client struct{ config ClientConfig }

func exactHTTPS(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Fragment == "" && u.RawQuery == "" && u.Opaque == "" && u.Path != "" && !u.ForceQuery && !strings.ContainsAny(value, "*\\\r\n\t ")
}

func (c client) GetID() string                     { return c.config.ID }
func (c client) RedirectURIs() []string            { return []string{c.config.RedirectURI} }
func (c client) PostLogoutRedirectURIs() []string  { return []string{c.config.PostLogoutRedirectURI} }
func (client) ApplicationType() op.ApplicationType { return op.ApplicationTypeWeb }
func (client) AuthMethod() oidc.AuthMethod         { return oidc.AuthMethodBasic }
func (client) ResponseTypes() []oidc.ResponseType  { return []oidc.ResponseType{oidc.ResponseTypeCode} }
func (client) GrantTypes() []oidc.GrantType        { return []oidc.GrantType{oidc.GrantTypeCode} }
func (c client) LoginURL(id string) string {
	u, _ := url.Parse(c.config.LoginURL)
	query := u.Query()
	query.Set("authRequestID", id)
	u.RawQuery = query.Encode()
	return u.String()
}
func (client) AccessTokenType() op.AccessTokenType                          { return op.AccessTokenTypeBearer }
func (client) IDTokenLifetime() time.Duration                               { return 5 * time.Minute }
func (client) DevMode() bool                                                { return false }
func (client) RestrictAdditionalIdTokenScopes() func([]string) []string     { return restrictScopes }
func (client) RestrictAdditionalAccessTokenScopes() func([]string) []string { return restrictScopes }
func (client) IsScopeAllowed(scope string) bool                             { return allowedScope(scope) }
func (client) IDTokenUserinfoClaimsAssertion() bool                         { return true }
func (client) ClockSkew() time.Duration                                     { return 5 * time.Second }
func allowedScope(scope string) bool {
	return slices.Contains([]string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail}, scope)
}
func restrictScopes(scopes []string) []string {
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if allowedScope(scope) {
			result = append(result, scope)
		}
	}
	return result
}
