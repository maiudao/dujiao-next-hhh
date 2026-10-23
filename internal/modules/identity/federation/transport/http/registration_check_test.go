package federationhttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
	"github.com/dujiao-next/internal/shared/mailbrand"
)

type previewRegistrationStub struct {
	calls      int
	rejection  error
	registered bool
}

func (*previewRegistrationStub) SendRegistrationCodeForSite(context.Context, string, string, mailbrand.Brand) error {
	return nil
}
func (*previewRegistrationStub) RegistrationCodePolicy() (int, int) { return 90, 900 }
func (*previewRegistrationStub) RegistrationCodeLength() int        { return 8 }
func (p *previewRegistrationStub) CheckRegistrationCode(string, string) error {
	p.calls++
	return p.rejection
}
func (p *previewRegistrationStub) Register(_, _, _ string, intent, forceEmail bool) (*userdomain.User, string, time.Time, error) {
	if !intent || !forceEmail {
		return nil, "", time.Time{}, errors.New("registration lost intent or email proof")
	}
	p.registered = true
	return nil, "", time.Time{}, nil
}

func TestRegistrationCheckCapabilityBindsEveryFieldAndExpiry(t *testing.T) {
	now := time.Now()
	token := registrationCheckToken("key", "main", "id", "state", " Artist@Example.com ", now.Add(time.Minute))
	if !validRegistrationCheckToken(token, "key", "main", "id", "state", "artist@example.com", now) {
		t.Fatal("canonical email rejected")
	}
	for _, fields := range [][5]string{
		{"wrong", "main", "id", "state", "artist@example.com"},
		{"key", "shop", "id", "state", "artist@example.com"},
		{"key", "main", "other", "state", "artist@example.com"},
		{"key", "main", "id", "other", "artist@example.com"},
		{"key", "main", "id", "state", "other@example.com"},
	} {
		if validRegistrationCheckToken(token, fields[0], fields[1], fields[2], fields[3], fields[4], now) {
			t.Fatal("capability crossed registration binding")
		}
	}
	if validRegistrationCheckToken(token, "key", "main", "id", "state", "artist@example.com", now.Add(time.Minute)) {
		t.Fatal("expired capability accepted")
	}
	for _, malformed := range []string{"", token + "x", "-1.invalid", strings.Repeat("x", 129)} {
		if validRegistrationCheckToken(malformed, "key", "main", "id", "state", "artist@example.com", now) {
			t.Fatal("malformed capability accepted")
		}
	}
}

func TestRegistrationCodeHTTPPreviewAndRemovedCheckbox(t *testing.T) {
	f := newBrowserFixture(t)
	nonce, verifier, state := randomSecret(t), randomSecret(t), randomSecret(t)
	id, _ := f.begin(t, f.party(t, "main", nonce), state, nonce, verifier)
	challenge := &fixtureClientChallenge{id: id, state: state, receipt: randomSecret(t)}
	service := &previewRegistrationStub{}
	f.handler.options.Challenges = map[string]ChallengeVerifier{"main": challenge}
	f.handler.options.Registration = service
	f.handler.options.RegistrationEnabled = func() (bool, error) { return true, nil }
	call := func(path string, values map[string]string, origin string) (int, []byte) {
		t.Helper()
		body, _ := json.Marshal(values)
		r, _ := http.NewRequest("POST", f.issuer+path, bytes.NewReader(body))
		r.SetBasicAuth("main", strings.Repeat("m", 32))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		response, err := f.rpc.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		result, _ := io.ReadAll(response.Body)
		return response.StatusCode, result
	}
	status, body := call("/registration/code", map[string]string{"auth_request_id": id, "state": state, "email": "fixture@example.invalid", "locale": "zh-CN", "receipt": challenge.receipt}, "")
	var delivery struct {
		Sent   bool   `json:"sent"`
		Length int    `json:"code_length"`
		Token  string `json:"check_token"`
	}
	if status != 200 || json.Unmarshal(body, &delivery) != nil || !delivery.Sent || delivery.Length != 8 || delivery.Token == "" || challenge.calls != 1 {
		t.Fatal("send lost preview capability or actual code length")
	}
	input := map[string]string{"auth_request_id": id, "state": state, "email": "fixture@example.invalid", "code": "12345678", "check_token": delivery.Token}
	for _, mutation := range []string{"email", "state", "check_token", "origin"} {
		bad := make(map[string]string)
		for key, value := range input {
			bad[key] = value
		}
		origin := ""
		switch mutation {
		case "email":
			bad[mutation] = "other@example.invalid"
		case "state":
			bad[mutation] = randomSecret(t)
		case "check_token":
			bad[mutation] = ""
		case "origin":
			origin = "https://main.example.invalid"
		}
		if status, _ := call("/registration/code/check", bad, origin); status != 403 || service.calls != 0 {
			t.Fatal("unbound preview reached code checker")
		}
	}
	for _, test := range []struct {
		rejection error
		status    int
		reason    string
	}{
		{nil, 200, ""}, {userauthapp.ErrVerifyCodeInvalid, 422, "invalid_code"}, {userauthapp.ErrVerifyCodeExpired, 422, "expired_code"}, {userauthapp.ErrVerifyCodeAttemptsExceeded, 429, "attempts_exceeded"},
	} {
		service.rejection = test.rejection
		status, body := call("/registration/code/check", input, "")
		var result struct {
			Valid bool   `json:"valid"`
			Error string `json:"error"`
		}
		if status != test.status || json.Unmarshal(body, &result) != nil || result.Error != test.reason || result.Valid != (test.rejection == nil) {
			t.Fatal("preview status lost")
		}
	}
	if challenge.calls != 1 {
		t.Fatal("preview consumed another puzzle receipt")
	}
	f.denyRate = true
	before := service.calls
	if status, _ := call("/registration/code/check", input, ""); status != 429 || service.calls != before {
		t.Fatal("preview bypassed rate limit")
	}
	r := httptest.NewRequest("POST", f.issuer+"/login/register", nil)
	r.PostForm = url.Values{"email": {"fixture@example.invalid"}, "password": {"synthetic-password"}, "password_confirm": {"synthetic-password"}, "code": {"12345678"}}
	if err := f.handler.registerAccount(r); err != nil || !service.registered {
		t.Fatal("removed checkbox still required", err)
	}
}
