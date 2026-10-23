package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/app/httpserver/middleware"
	"github.com/dujiao-next/internal/config"
	federationdomain "github.com/dujiao-next/internal/modules/identity/federation/domain"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	federationhttp "github.com/dujiao-next/internal/modules/identity/federation/transport/http"
	"github.com/dujiao-next/internal/modules/identity/jwttoken"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	userpresenter "github.com/dujiao-next/internal/modules/identity/userauth/transport/presenter"
	"github.com/dujiao-next/internal/platform/database/gormdb"
	"github.com/gin-gonic/gin"
)

func federationSessionValidator(cfg *config.Config) middleware.UserSessionValidator {
	if !cfg.IdentityProvider.Enabled {
		return nil
	}
	store := federationstore.New(gormdb.DB)
	return func(ctx context.Context, claims *userauthapp.UserJWTClaims) error {
		if claims.SessionID == "" {
			return federationdomain.ErrInteraction
		}
		session, err := store.SessionByID(ctx, claims.SessionID)
		if err != nil {
			if errors.Is(err, federationdomain.ErrInteraction) {
				return err
			}
			return middleware.ErrUserSessionUnavailable
		}
		if session.UserID != claims.UserID || session.TokenVersion != claims.TokenVersion {
			return federationdomain.ErrInteraction
		}
		return nil
	}
}

func identitySignal(w http.ResponseWriter, host string) {
	if host != "shop.huihuahui.xyz" {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "huihua-member-signal", Value: strconv.FormatInt(time.Now().UnixNano(), 36), Path: "/", Domain: "huihuahui.xyz", Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 60 * 60})
}

// Buffer only the small authentication responses, so the original login,
// registration, social and 2FA handlers remain the sole proof verifiers. Their
// successfully issued access token is bound before any response reaches JS.
type identityAuthWriter struct {
	gin.ResponseWriter
	body   bytes.Buffer
	status int
}

func (w *identityAuthWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *identityAuthWriter) WriteHeaderNow() {
	if w.status == 0 {
		w.status = 200
	}
}
func (w *identityAuthWriter) Write(data []byte) (int, error) {
	w.WriteHeaderNow()
	return w.body.Write(data)
}
func (w *identityAuthWriter) WriteString(data string) (int, error) { return w.Write([]byte(data)) }
func (w *identityAuthWriter) Status() int {
	if w.status == 0 {
		return 200
	}
	return w.status
}
func (w *identityAuthWriter) Size() int     { return w.body.Len() }
func (w *identityAuthWriter) Written() bool { return w.status != 0 }

func federationAuthResponses(cfg *config.Config, c *container.Container) gin.HandlerFunc {
	store := federationstore.New(gormdb.DB)
	return func(ctx *gin.Context) {
		if ctx.Request.Method != http.MethodPost || !strings.HasPrefix(ctx.Request.URL.Path, "/api/v1/auth/") {
			ctx.Next()
			return
		}
		original := ctx.Writer
		defer func() { ctx.Writer = original }()
		buffer := &identityAuthWriter{ResponseWriter: original}
		ctx.Writer = buffer
		ctx.Next()
		ctx.Writer = original
		data := buffer.body.Bytes()
		original.Header().Del("Content-Length")
		original.Header().Set("Cache-Control", "no-store")
		if len(data) >= 64*1024 {
			original.WriteHeader(503)
			return
		}
		if buffer.Status() >= 200 && buffer.Status() < 300 && len(data) < 64*1024 {
			var envelope map[string]json.RawMessage
			if json.Unmarshal(data, &envelope) == nil {
				var payload map[string]json.RawMessage
				if json.Unmarshal(envelope["data"], &payload) == nil {
					var raw string
					if json.Unmarshal(payload["token"], &raw) == nil && raw != "" {
						claims, err := c.UserAuthService.ParseUserJWT(raw)
						if err != nil || !jwttoken.IsAccessType(claims.Typ) || claims.UserID == 0 || claims.ExpiresAt == nil {
							original.WriteHeader(503)
							_, _ = original.Write([]byte(`{"code":503,"message":"统一登录未完成，请重试"}`))
							return
						}
						deadline := claims.ExpiresAt.Time
						if cap := time.Now().Add(time.Duration(cfg.IdentityProvider.RememberHours) * time.Hour); cap.Before(deadline) {
							deadline = cap
						}
						previous := ""
						if cookie, e := ctx.Request.Cookie(federationhttp.SessionCookieName); e == nil && len(cookie.Value) == 43 {
							previous = cookie.Value
						}
						cookie, session, err := store.StartShopSession(ctx.Request.Context(), claims.UserID, claims.TokenVersion, deadline, previous, ctx.Request.URL.Path == "/api/v1/auth/login/verify-2fa")
						if err == nil {
							raw, err = c.UserAuthService.BindBrowserToken(raw, session.ID, session.ExpiresAt)
						}
						if err != nil {
							original.WriteHeader(503)
							_, _ = original.Write([]byte(`{"code":503,"message":"统一登录未完成，请重试"}`))
							return
						}
						http.SetCookie(original, &http.Cookie{Name: federationhttp.SessionCookieName, Value: cookie, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: session.ExpiresAt, MaxAge: int(time.Until(session.ExpiresAt).Seconds())})
						identitySignal(original, ctx.Request.Host)
						payload["token"], _ = json.Marshal(raw)
						payload["expires_at"], _ = json.Marshal(session.ExpiresAt.Format(time.RFC3339))
						envelope["data"], _ = json.Marshal(payload)
						data, _ = json.Marshal(envelope)
					}
				}
			}
		}
		original.Header().Del("Content-Length")
		original.WriteHeader(buffer.Status())
		_, _ = original.Write(data)
	}
}

func federationBrowser(cfg *config.Config, c *container.Container) gin.HandlerFunc {
	store := federationstore.New(gormdb.DB)
	guard := func(ctx *gin.Context) bool {
		ctx.Header("Cache-Control", "no-store")
		ctx.Header("Pragma", "no-cache")
		origin := strings.TrimSuffix(cfg.IdentityProvider.Issuer, "/api/identity")
		if "https://"+ctx.Request.Host != origin || ctx.GetHeader("X-Huihua-Session") != "1" || (ctx.GetHeader("Origin") != "" && ctx.GetHeader("Origin") != origin) || (ctx.Request.Method == http.MethodPost && ctx.GetHeader("Origin") != origin) {
			ctx.AbortWithStatus(403)
			return false
		}
		return true
	}
	return func(ctx *gin.Context) {
		if ctx.Request.URL.Path == "/api/identity/browser" && ctx.Request.Method == http.MethodGet {
			if !guard(ctx) {
				return
			}
			cookie, err := ctx.Request.Cookie(federationhttp.SessionCookieName)
			if err != nil || cookie.Value == "" {
				ctx.JSON(200, gin.H{"enabled": true, "logged_in": false})
				return
			}
			session, err := store.Session(ctx.Request.Context(), cookie.Value)
			if err != nil {
				if errors.Is(err, federationdomain.ErrInteraction) {
					ctx.JSON(200, gin.H{"enabled": true, "logged_in": false})
				} else {
					ctx.AbortWithStatus(503)
				}
				return
			}
			user, err := c.UserStore.GetByID(session.UserID)
			if err != nil || user == nil {
				ctx.AbortWithStatus(503)
				return
			}
			raw, _, err := c.UserAuthService.GenerateUserJWT(user, 0)
			if err == nil {
				raw, err = c.UserAuthService.BindBrowserToken(raw, session.ID, session.ExpiresAt)
			}
			if err != nil {
				ctx.AbortWithStatus(503)
				return
			}
			ctx.JSON(200, gin.H{"enabled": true, "logged_in": true, "token": raw, "user": userpresenter.NewUserAuthBriefResp(user)})
			return
		}
		if ctx.Request.URL.Path == "/api/identity/browser/logout" && ctx.Request.Method == http.MethodPost {
			if !guard(ctx) {
				return
			}
			cookie, err := ctx.Request.Cookie(federationhttp.SessionCookieName)
			if err == nil && cookie.Value != "" {
				session, err := store.Session(ctx.Request.Context(), cookie.Value)
				if err != nil && !errors.Is(err, federationdomain.ErrInteraction) {
					ctx.AbortWithStatus(503)
					return
				}
				if err == nil && store.RevokeSession(ctx.Request.Context(), session.ID) != nil {
					ctx.AbortWithStatus(503)
					return
				}
			}
			http.SetCookie(ctx.Writer, &http.Cookie{Name: federationhttp.SessionCookieName, Path: "/", Value: "", MaxAge: -1, Expires: time.Unix(1, 0), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			identitySignal(ctx.Writer, ctx.Request.Host)
			ctx.Status(204)
			return
		}
		ctx.AbortWithStatus(http.StatusMethodNotAllowed)
	}
}
