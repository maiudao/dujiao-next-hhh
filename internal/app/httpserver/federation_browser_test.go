package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/app/container"
	"github.com/dujiao-next/internal/config"
	federationstore "github.com/dujiao-next/internal/modules/identity/federation/infrastructure/gormstore"
	federationhttp "github.com/dujiao-next/internal/modules/identity/federation/transport/http"
	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userstore "github.com/dujiao-next/internal/modules/identity/user/infrastructure/gormstore"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/platform/database/gormdb"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFederationShopHTTPRestoresAndRevokesBoundTokens(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "browser.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = pool.Close() })
	previous := gormdb.DB
	gormdb.DB = db
	t.Cleanup(func() { gormdb.DB = previous })
	if err := db.AutoMigrate(&userdomain.User{}); err != nil {
		t.Fatal(err)
	}
	store := federationstore.New(db)
	if err := store.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	user := &userdomain.User{ID: 1, Email: "fixture@example.invalid", Status: "active"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{UserJWT: config.JWTConfig{SecretKey: strings.Repeat("j", 32), ExpireHours: 24}, IdentityProvider: config.IdentityProviderConfig{Enabled: true, Issuer: "https://shop.example.invalid/api/identity", RememberHours: 24}}
	auth := userauthapp.NewService(cfg, userstore.New(db), nil, nil, nil, nil, nil)
	c := &container.Container{Config: cfg, UserAuthService: auth, UserStore: userstore.New(db)}
	r := gin.New()
	r.Any("/api/identity/*identityPath", federationBrowser(cfg, c))
	r.Use(federationAuthResponses(cfg, c))
	r.POST("/api/v1/auth/login", func(ctx *gin.Context) {
		raw, _, err := auth.GenerateUserJWT(user, 0)
		if err != nil {
			t.Fatal(err)
		}
		ctx.JSON(200, gin.H{"data": gin.H{"token": raw}})
	})
	request := func(method, path string, cookie *http.Cookie, origin string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "https://shop.example.invalid"+path, nil)
		req.Header.Set("X-Huihua-Session", "1")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	login := request("POST", "/api/v1/auth/login", nil, "https://shop.example.invalid")
	if login.Code != 200 {
		t.Fatal("login was not adapted")
	}
	var envelope struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	claims, err := auth.ParseUserJWT(envelope.Data.Token)
	if err != nil || len(claims.SessionID) != 43 {
		t.Fatal("JWT left HTTP boundary without authoritative session")
	}
	var cookie *http.Cookie
	for _, value := range login.Result().Cookies() {
		if value.Name == federationhttp.SessionCookieName {
			cookie = value
		}
	}
	if cookie == nil || cookie.Domain != "" || !cookie.Secure || !cookie.HttpOnly {
		t.Fatal("authority cookie protections missing")
	}
	restored := request("GET", "/api/identity/browser", cookie, "")
	var state struct {
		Enabled bool   `json:"enabled"`
		Logged  bool   `json:"logged_in"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(restored.Body.Bytes(), &state); err != nil || restored.Code != 200 || !state.Logged {
		t.Fatal("cookie did not restore Shop login")
	}
	bound, err := auth.ParseUserJWT(state.Token)
	if err != nil || bound.SessionID != claims.SessionID {
		t.Fatal("restore issued token for different session")
	}
	if got := request("POST", "/api/identity/browser/logout", cookie, "https://foreign.invalid"); got.Code != 403 {
		t.Fatal("foreign origin revoked session")
	}
	if got := request("POST", "/api/identity/browser/logout", cookie, ""); got.Code != 403 {
		t.Fatal("missing origin revoked session")
	}
	if got := request("POST", "/api/identity/browser/logout", cookie, "https://shop.example.invalid"); got.Code != 204 {
		t.Fatal("logout failed")
	}
	if err := federationSessionValidator(cfg)(t.Context(), claims); err == nil {
		t.Fatal("old JWT survived logout")
	}
	if got := request("GET", "/api/identity/browser", cookie, ""); got.Code != 200 || strings.Contains(got.Body.String(), `"logged_in":true`) {
		t.Fatal("revoked cookie restored login")
	}
	if _, err := store.SessionByID(t.Context(), claims.SessionID); err == nil {
		t.Fatal("server revocation was not durable")
	}
	if got := request("POST", "/api/v1/auth/login", nil, "https://shop.example.invalid"); got.Code != 200 {
		t.Fatal("fresh login failed")
	}
	if err := db.Migrator().DropTable("identity_browser_sessions"); err != nil {
		t.Fatal(err)
	}
	failed := request("POST", "/api/v1/auth/login", nil, "https://shop.example.invalid")
	if failed.Code != 503 || strings.Contains(failed.Body.String(), `"token"`) {
		t.Fatal("storage failure returned an unbound token")
	}
}
