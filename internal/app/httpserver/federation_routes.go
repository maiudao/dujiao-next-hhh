package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/bootstrap/federation"
	"github.com/dujiao-next/internal/cache"
	"github.com/dujiao-next/internal/config"
	"github.com/dujiao-next/internal/constants"
	"github.com/dujiao-next/internal/logger"
	auditlogapp "github.com/dujiao-next/internal/modules/auditlog/application"
	captchahttp "github.com/dujiao-next/internal/modules/captcha/transport/http"
	federationhttp "github.com/dujiao-next/internal/modules/identity/federation/transport/http"
	"github.com/dujiao-next/internal/platform/database/gormdb"
	"github.com/gin-gonic/gin"
)

type identityBoundaryKey struct{}
type identityBoundary struct {
	ip, requestID string
	form          bool
}

func registerFederationRoutes(r *gin.Engine, cfg *config.Config, c *container.Container) error {
	if !cfg.IdentityProvider.Enabled {
		return nil
	}
	if c == nil || c.UserAuthService == nil || c.CaptchaService == nil || c.UserLoginLogService == nil {
		return errors.New("identity authentication, captcha and audit services are required")
	}
	protocolLog := federation.Logger(logger.Z(), func(ctx context.Context) string {
		boundary, _ := ctx.Value(identityBoundaryKey{}).(identityBoundary)
		return boundary.requestID
	})
	server, err := federation.New(cfg, federation.Dependencies{
		DB: gormdb.DB, Auth: c.UserAuthService, Captcha: captchahttp.NewVerifier(c.CaptchaService),
		RegistrationEnabled: func() (bool, error) { return c.SettingService.GetRegistrationEnabled(true) },
		ClientIP: func(r *http.Request) string {
			boundary, _ := r.Context().Value(identityBoundaryKey{}).(identityBoundary)
			return boundary.ip
		},
		RateLimit: func(r *http.Request, bucket, _ string) error {
			boundary, ok := r.Context().Value(identityBoundaryKey{}).(identityBoundary)
			if !ok || boundary.ip == "" || (bucket != "identity" && bucket != "identity_form") || (bucket == "identity_form" && !boundary.form) {
				return errors.New("identity middleware boundary missing")
			}
			return nil
		},
		Logger: protocolLog,
		Audit: func(r *http.Request, event federationhttp.LoginEvent) {
			boundary, _ := r.Context().Value(identityBoundaryKey{}).(identityBoundary)
			status, reason := constants.LoginLogStatusFailed, "identity_authentication_failed"
			if event.Success {
				status, reason = constants.LoginLogStatusSuccess, ""
			}
			// This is the existing restricted login audit, with no request body,
			// email, protocol state, credentials or raw upstream error text.
			if err := c.UserLoginLogService.Record(auditlogapp.UserLoginRecord{UserID: event.UserID, Status: status, FailReason: reason, LoginSource: "identity_" + event.ClientID, ClientIP: boundary.ip, RequestID: boundary.requestID}); err != nil {
				logger.Warnw("identity login audit unavailable", "request_id", boundary.requestID)
			}
		},
	})
	if err != nil {
		return err
	}
	prefix := strings.TrimSpace(cfg.Redis.Prefix)
	if prefix == "" {
		prefix = constants.RedisPrefixDefault
	}
	general := middleware.RateLimitMiddleware(cache.Client(), middleware.RateLimitRule{Prefix: prefix + ":rate:identity", WindowSeconds: 60, MaxRequests: 120, BlockSeconds: 60}, middleware.KeyByIP)
	form := middleware.RateLimitMiddleware(cache.Client(), middleware.RateLimitRule{Prefix: prefix + ":rate:identity_form", WindowSeconds: cfg.Security.LoginRateLimit.WindowSeconds, MaxRequests: cfg.Security.LoginRateLimit.MaxAttempts, BlockSeconds: cfg.Security.LoginRateLimit.BlockSeconds}, middleware.KeyByIP)
	if cfg.Security.LoginRateLimit.WindowSeconds < 1 || cfg.Security.LoginRateLimit.MaxAttempts < 1 {
		return errors.New("identity login rate limit is required")
	}
	// The pinned OIDC library ignores WithLogger and uses global slog calls.
	// Install its restricted sink once during application setup, never swap a
	// process-wide logger around concurrent requests. The application's zap
	// request/business loggers keep their existing destinations and behavior.
	slog.SetDefault(protocolLog)
	browser := federationBrowser(cfg, c)
	r.Any(federation.Path+"/*identityPath", identityRecovery, general, func(ctx *gin.Context) {
		path := ctx.Param("identityPath")
		if path == "/login/password" || path == "/login/second-factor" || path == "/login/cancel" || path == "/login/register" || path == "/registration/code" {
			form(ctx)
			return
		}
		ctx.Next()
	}, func(ctx *gin.Context) {
		path := ctx.Param("identityPath")
		if path == "/browser" || path == "/browser/logout" {
			browser(ctx)
			return
		}
		boundary := identityBoundary{ip: ctx.ClientIP(), requestID: ctx.GetString("request_id"), form: path == "/login/password" || path == "/login/second-factor" || path == "/login/cancel" || path == "/login/register" || path == "/registration/code"}
		request := ctx.Request.WithContext(context.WithValue(ctx.Request.Context(), identityBoundaryKey{}, boundary))
		server.ServeHTTP(ctx.Writer, request)
	})
	return nil
}

// Recover inside the generic application recovery so neither panic text nor
// third-party errors containing credentials reach the generic error logger.
func identityRecovery(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Header("Referrer-Policy", "no-referrer")
	defer func() {
		if recover() != nil {
			logger.Errorw("identity request failed", "request_id", ctx.GetString("request_id"))
			ctx.Header("Cache-Control", "no-store")
			ctx.AbortWithStatus(http.StatusInternalServerError)
		}
	}()
	ctx.Next()
}
