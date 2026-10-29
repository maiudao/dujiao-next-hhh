package httpserver

import (
	"crypto/subtle"
	"net/http"

	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Only authenticated server-to-server session checks use this bounded bucket.
// They must not consume the browser/login quota shared by the main server IP.
func identitySessionRateKey(r *http.Request, clients []config.IdentityClientConfig) string {
	if r.Method != http.MethodPost || r.URL.Path != "/api/identity/session/check" || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		return ""
	}
	id, secret, ok := r.BasicAuth()
	if !ok || id == "" || secret == "" {
		return ""
	}
	for _, client := range clients {
		if client.ID == id && len(client.Secret) >= 32 && subtle.ConstantTimeCompare([]byte(secret), []byte(client.Secret)) == 1 {
			return client.ID
		}
	}
	return ""
}

func identityGeneralRateLimit(client *redis.Client, prefix string, clients []config.IdentityClientConfig) gin.HandlerFunc {
	general := middleware.RateLimitMiddleware(client, middleware.RateLimitRule{
		Prefix: prefix + ":rate:identity", WindowSeconds: 60, MaxRequests: 120, BlockSeconds: 60,
	}, middleware.KeyByIP)
	// Twenty checks/second per configured confidential client remains bounded;
	// the live session/owner/revocation checks in the handler stay mandatory.
	sessions := middleware.RateLimitMiddleware(client, middleware.RateLimitRule{
		Prefix: prefix + ":rate:identity_session", WindowSeconds: 60, MaxRequests: 1200, BlockSeconds: 60,
	}, func(ctx *gin.Context) string { return ctx.GetString("identity_session_rate_client") })
	return func(ctx *gin.Context) {
		if id := identitySessionRateKey(ctx.Request, clients); id != "" {
			ctx.Set("identity_session_rate_client", id)
			sessions(ctx)
			return
		}
		// Browser, password, token, revoke and invalid-credential requests keep
		// their existing IP limit. Password attempts also keep the form limit.
		general(ctx)
	}
}
