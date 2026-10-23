package federationhttp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	userdomain "github.com/dujiao-next/internal/modules/identity/user/domain"
	userauthapp "github.com/dujiao-next/internal/modules/identity/userauth/application"
)

func TestRegistrationErrorsRemainBoundAndReadable(t *testing.T) {
	f := newBrowserFixture(t)
	nonce, verifier, state := randomSecret(t), randomSecret(t), randomSecret(t)
	id, _ := f.begin(t, f.party(t, "main", nonce), state, nonce, verifier)
	challenge := &fixtureClientChallenge{id: id, state: state, receipt: randomSecret(t)}
	f.handler.options.Challenges = map[string]ChallengeVerifier{"main": challenge}
	service := &rejectingRegistration{previewRegistrationStub: previewRegistrationStub{}}
	f.handler.options.Registration = service
	f.handler.options.RegistrationEnabled = func() (bool, error) { return true, nil }
	for _, test := range []struct {
		err         error
		status      int
		code, field string
	}{
		{userauthapp.ErrVerifyCodeInvalid, 422, "invalid_code", "code"},
		{userauthapp.ErrVerifyCodeExpired, 422, "expired_code", "code"},
		{userauthapp.ErrEmailExists, 409, "email_exists", "email"},
		{errors.New("private upstream detail"), 503, "registration_unavailable", ""},
	} {
		service.err = test.err
		challenge.receipt = randomSecret(t) // Each final submission spends its own pass.
		values := url.Values{"authRequestID": {id}, "csrf": {state}, "email": {"fixture@example.invalid"}, "password": {"Synthetic-QA9"}, "password_confirm": {"Synthetic-QA9"}, "code": {"123456"}, "challenge_receipt": {challenge.receipt}}
		r, _ := http.NewRequest("POST", f.issuer+"/login/register", strings.NewReader(values.Encode()))
		r.Header.Set("Origin", "https://main.example.invalid")
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Accept", "application/json")
		response, err := f.browser.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]string
		err = json.NewDecoder(response.Body).Decode(&body)
		response.Body.Close()
		if err != nil || response.StatusCode != test.status || body["code"] != test.code || body["field"] != test.field || body["error"] == "" || strings.Contains(body["error"], "private") || response.Header.Get("Location") != "" {
			t.Fatalf("lost safe inline result: status=%d body=%v", response.StatusCode, body)
		}
		if response.Header.Get("Access-Control-Allow-Origin") != "https://main.example.invalid" || response.Header.Get("Access-Control-Allow-Credentials") != "true" {
			t.Fatal("registered form cannot read result")
		}
	}
	r := httptest.NewRequest("POST", f.issuer+"/login/register", nil)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("Origin", "https://unknown.example.invalid")
	w := httptest.NewRecorder()
	if f.handler.registrationJSON(w, r) || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("untrusted origin can read credentialed result")
	}
}

type rejectingRegistration struct {
	previewRegistrationStub
	err error
}

func (p *rejectingRegistration) Register(string, string, string, bool, bool) (*userdomain.User, string, time.Time, error) {
	return nil, "", time.Time{}, p.err
}
