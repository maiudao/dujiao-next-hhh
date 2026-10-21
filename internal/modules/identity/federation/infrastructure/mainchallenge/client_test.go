package mainchallenge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func proof(value byte) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string(value), 32)))
}

func TestExactAuthenticatedBackendRedemptionAndNoRedirects(t *testing.T) {
	id, state, receipt := proof('i'), proof('s'), proof('r')
	calls, redirected := 0, 0
	status := http.StatusNoContent
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected++; w.WriteHeader(http.StatusNoContent) }))
	defer other.Close()
	backend := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		client, secret, ok := r.BasicAuth()
		if !ok || client != "main" || secret != strings.Repeat("m", 32) || r.Method != http.MethodPost || r.URL.Path != "/auth/member/challenge/consume" || r.URL.RawQuery != "" || r.Header.Get("Origin") != "" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("unsafe backend request")
		}
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["id"] != id || body["state"] != state || body["receipt"] != receipt {
			t.Error("incorrect challenge binding")
		}
		w.Header().Set("Location", other.URL)
		w.WriteHeader(status)
		_, _ = w.Write([]byte("backend-secret-error"))
	}))
	defer backend.Close()
	c, err := New(backend.URL+"/auth/member/challenge/consume", backend.URL+"/login", "main", strings.Repeat("m", 32), backend.Client().Transport)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Verify(t.Context(), id, state, receipt, "fixture@example.invalid"); err != nil {
		t.Fatal(err)
	}
	for _, denied := range []int{http.StatusFound, http.StatusTemporaryRedirect, http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError, http.StatusOK} {
		status = denied
		err := c.Verify(t.Context(), id, state, receipt, "fixture@example.invalid")
		if err == nil || strings.Contains(err.Error(), "backend-secret") {
			t.Fatal("failed backend accepted or exposed")
		}
	}
	if redirected != 0 {
		t.Fatal("secret was redirected to another host")
	}
	before := calls
	if c.Verify(t.Context(), "bad", state, receipt, "fixture@example.invalid") == nil || calls != before {
		t.Fatal("malformed secret sent")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if c.Verify(ctx, id, state, receipt, "fixture@example.invalid") == nil {
		t.Fatal("canceled request accepted")
	}
}

func TestRejectUntrustedBackendTargets(t *testing.T) {
	for _, target := range []string{
		"http://main.example.invalid/auth/member/challenge/consume",
		"https://elsewhere.invalid/auth/member/challenge/consume",
		"https://main.example.invalid/other",
		"https://main.example.invalid/auth/member/challenge/consume?x=1",
		"https://main.example.invalid/auth/member/challenge/consume#x",
		"https://user:pass@main.example.invalid/auth/member/challenge/consume",
		"https://main.example.invalid/auth/member/challenge/%63onsume",
	} {
		if _, err := New(target, "https://main.example.invalid/login", "main", strings.Repeat("m", 32), nil); err == nil {
			t.Fatal("untrusted challenge URL accepted")
		}
	}
}
