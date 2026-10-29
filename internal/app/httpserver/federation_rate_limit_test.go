package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/config"
	"github.com/gin-gonic/gin"
)

func TestIdentitySessionRateKeyRequiresExactConfidentialBoundary(t *testing.T) {
	clients := []config.IdentityClientConfig{{ID: "main", Secret: strings.Repeat("m", 32)}}
	for _, scenario := range []struct {
		name, method, path, id, secret, origin string
		allowed                                bool
	}{
		{"valid", "POST", "/api/identity/session/check", "main", strings.Repeat("m", 32), "", true},
		{"wrong-secret", "POST", "/api/identity/session/check", "main", strings.Repeat("x", 32), "", false},
		{"unknown-client", "POST", "/api/identity/session/check", "foreign", strings.Repeat("m", 32), "", false},
		{"no-auth", "POST", "/api/identity/session/check", "", "", "", false},
		{"browser-origin", "POST", "/api/identity/session/check", "main", strings.Repeat("m", 32), "https://main.example.invalid", false},
		{"query", "POST", "/api/identity/session/check?x=1", "main", strings.Repeat("m", 32), "", false},
		{"wrong-method", "GET", "/api/identity/session/check", "main", strings.Repeat("m", 32), "", false},
		{"password", "POST", "/api/identity/login/password", "main", strings.Repeat("m", 32), "", false},
		{"token", "POST", "/api/identity/oauth/token", "main", strings.Repeat("m", 32), "", false},
		{"revoke", "POST", "/api/identity/session/revoke", "main", strings.Repeat("m", 32), "", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			r := httptest.NewRequest(scenario.method, "https://shop.example.invalid"+scenario.path, nil)
			if scenario.id != "" {
				r.SetBasicAuth(scenario.id, scenario.secret)
			}
			r.Header.Set("Origin", scenario.origin)
			if got := identitySessionRateKey(r, clients); (got == "main") != scenario.allowed {
				t.Fatal("confidential session quota boundary mismatch")
			}
		})
	}
}

func TestIdentitySessionChecksDoNotExhaustLoginQuotaAndStayBounded(t *testing.T) {
	clients := []config.IdentityClientConfig{{ID: "main", Secret: strings.Repeat("m", 32)}, {ID: "other", Secret: strings.Repeat("o", 32)}}
	engine := gin.New()
	engine.Any("/api/identity/*identityPath", identityGeneralRateLimit(nil, "fixture", clients), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	call := func(path, client, secret string) int {
		r := httptest.NewRequest(http.MethodPost, "https://shop.example.invalid/api/identity"+path, nil)
		r.RemoteAddr = "192.0.2.1:1234"
		if client != "" {
			r.SetBasicAuth(client, secret)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, r)
		return w.Code
	}
	for i := 0; i < 1200; i++ {
		if call("/session/check", "main", strings.Repeat("m", 32)) != http.StatusNoContent {
			t.Fatalf("session check rejected at %d", i)
		}
	}
	if call("/session/check", "main", strings.Repeat("m", 32)) != http.StatusTooManyRequests {
		t.Fatal("session quota became unbounded")
	}
	if call("/session/check", "other", strings.Repeat("o", 32)) != http.StatusNoContent {
		t.Fatal("independent client quota was consumed")
	}
	for _, path := range []string{"/interaction", "/oauth/token", "/session/revoke", "/login/password"} {
		if call(path, "main", strings.Repeat("m", 32)) != http.StatusNoContent {
			t.Fatal("session polling blocked login or logout")
		}
	}
	for i := 4; i < 120; i++ {
		if call("/interaction", "", "") != http.StatusNoContent {
			t.Fatal("browser quota changed")
		}
	}
	if call("/interaction", "", "") != http.StatusTooManyRequests {
		t.Fatal("browser quota bypassed")
	}
	if call("/session/check", "main", strings.Repeat("x", 32)) != http.StatusTooManyRequests {
		t.Fatal("invalid credential bypassed browser quota")
	}
}
