package federationhttp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
	"gorm.io/gorm"
)

func TestHTTPSConfidentialSessionDelegationAndRevocation(t *testing.T) {
	f := newBrowserFixture(t)
	state, nonce, verifier := randomSecret(t), randomSecret(t), randomSecret(t)
	party := f.party(t, "main", nonce)
	id, _ := f.begin(t, party, state, nonce, verifier)
	location, status, _ := f.post(t, "/login/password", "https://main.example.invalid", passwordForm(id, state))
	if status != http.StatusSeeOther {
		t.Fatal("real password login failed")
	}
	callback, status, _ := f.get(t, location.String())
	if status != http.StatusFound {
		t.Fatal("real authorization callback failed")
	}
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](t.Context(), callback.Query().Get("code"), party, rp.WithCodeVerifier(verifier))
	if err != nil {
		t.Fatal(err)
	}
	claims := tokens.IDTokenClaims
	input, _ := json.Marshal(map[string]string{"session_id": claims.SessionID, "subject": claims.Subject})
	call := func(path, method, origin, client, secret, body string) (int, []byte) {
		t.Helper()
		r, err := http.NewRequest(method, f.issuer+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		if client != "" {
			r.SetBasicAuth(client, secret)
		}
		response, err := f.rpc.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		if response.Header.Get("Cache-Control") != "no-store" {
			t.Error("session response was cacheable")
		}
		return response.StatusCode, data
	}
	for _, scenario := range []struct {
		path, method, origin, client, secret, body string
	}{
		{"/session/check", "POST", "", "", "", string(input)},
		{"/session/check", "POST", "https://main.example.invalid", "main", strings.Repeat("m", 32), string(input)},
		{"/session/check", "GET", "", "main", strings.Repeat("m", 32), string(input)},
		{"/session/check?unexpected=1", "POST", "", "main", strings.Repeat("m", 32), string(input)},
		{"/session/check", "POST", "", "main", strings.Repeat("x", 32), string(input)},
		{"/session/check", "POST", "", "shop", strings.Repeat("s", 32), string(input)},
		{"/session/revoke", "POST", "", "shop", strings.Repeat("s", 32), string(input)},
		{"/session/check", "POST", "", "main", strings.Repeat("m", 32), string(input) + `{}`},
		{"/session/check", "POST", "", "main", strings.Repeat("m", 32), strings.Repeat(" ", 5000) + string(input)},
	} {
		if code, _ := call(scenario.path, scenario.method, scenario.origin, scenario.client, scenario.secret, scenario.body); code < 400 {
			t.Fatal("invalid confidential session request accepted")
		}
	}
	code, body := call("/session/check", "POST", "", "main", strings.Repeat("m", 32), string(input))
	if code != http.StatusOK || strings.Contains(string(body), "email") || strings.Contains(string(body), "cookie") {
		t.Fatal("delegated session response invalid or over-disclosed", code)
	}
	var result struct {
		ExpiresAt int64 `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.ExpiresAt <= time.Now().Unix() {
		t.Fatal("missing live authority deadline", err)
	}
	if err := f.db.Callback().Query().Before("gorm:query").Register("test:session_db_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "identity_client_sessions" {
			tx.AddError(errors.New("synthetic unavailable database"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/session/check", "/session/revoke"} {
		if code, _ := call(path, "POST", "", "main", strings.Repeat("m", 32), string(input)); code != http.StatusServiceUnavailable {
			t.Fatal("database outage masqueraded as a permanent session revocation", code)
		}
	}
	if err := f.db.Callback().Query().Remove("test:session_db_failure"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if code, _ := call("/session/revoke", "POST", "", "main", strings.Repeat("m", 32), string(input)); code != http.StatusNoContent {
			t.Fatal("idempotent confidential logout failed", code)
		}
	}
	if code, _ := call("/session/check", "POST", "", "main", strings.Repeat("m", 32), string(input)); code != http.StatusUnauthorized {
		t.Fatal("revoked authority session stayed active", code)
	}
	if _, err := f.store.SessionByID(t.Context(), claims.SessionID); err == nil {
		t.Fatal("confidential logout did not revoke authority browser session")
	}
	for _, sensitive := range []string{claims.Subject, claims.SessionID, tokens.AccessToken, strings.Repeat("m", 32)} {
		if strings.Contains(f.logs.String(), sensitive) {
			t.Fatal("session logs leaked sensitive values")
		}
	}
}
