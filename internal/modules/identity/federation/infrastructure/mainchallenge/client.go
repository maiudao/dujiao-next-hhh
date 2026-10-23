// Package mainchallenge redeems the main site's original captcha/risk approval.
// This is not an identity assertion; password, 2FA, state, CSRF and the authority
// browser binding are still checked by the existing federation service.
package mainchallenge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var errRejected = errors.New("main-site challenge unavailable")

type Client struct {
	endpoint, clientID, secret string
	http                       *http.Client
}

func New(endpoint, formURL, clientID, secret string, transport http.RoundTripper) (*Client, error) {
	target, err := url.Parse(endpoint)
	form, formErr := url.Parse(formURL)
	if err != nil || formErr != nil || target.Scheme != "https" || form.Scheme != "https" || target.Host == "" || target.Host != form.Host || target.Path != "/auth/member/challenge/consume" || target.User != nil || target.RawQuery != "" || target.ForceQuery || target.Fragment != "" || target.Opaque != "" || target.RawPath != "" || strings.ContainsAny(endpoint, "*\\\r\n\t ") || clientID == "" || len(secret) < 32 {
		return nil, errRejected
	}
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &Client{endpoint: endpoint, clientID: clientID, secret: secret, http: &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func valid(value string) bool {
	data, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(data) == 32 && base64.RawURLEncoding.EncodeToString(data) == value
}

func (c *Client) Verify(ctx context.Context, id, state, receipt, email string) error {
	_, err := c.verify(ctx, id, state, receipt, email, false)
	return err
}

func (c *Client) VerifyIdentifier(ctx context.Context, id, state, receipt, identifier string) (string, error) {
	return c.verify(ctx, id, state, receipt, identifier, true)
}

func (c *Client) verify(ctx context.Context, id, state, receipt, email string, resolve bool) (string, error) {
	if !valid(id) || !valid(state) || !valid(receipt) {
		return "", errRejected
	}
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 254 || (!resolve && !strings.Contains(email, "@")) {
		return "", errRejected
	}
	digest := sha256.Sum256([]byte(email))
	input := map[string]string{"id": id, "state": state, "receipt": receipt, "identity": hex.EncodeToString(digest[:])}
	if resolve {
		input["identifier"] = email
	}
	data, _ := json.Marshal(input)
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(data))
	if err != nil {
		return "", errRejected
	}
	r.Header.Set("Content-Type", "application/json")
	r.SetBasicAuth(c.clientID, c.secret)
	response, err := c.http.Do(r)
	if err != nil {
		return "", errRejected
	}
	defer response.Body.Close()
	// Only the explicitly requested canonical identifier response may be read.
	// Error bodies and underlying network errors are never exposed or logged.
	if resolve && response.StatusCode == http.StatusOK {
		var value struct {
			Email string `json:"email"`
		}
		decoder := json.NewDecoder(io.LimitReader(response.Body, 1025))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&value) != nil || decoder.Decode(&struct{}{}) != io.EOF || len(value.Email) > 254 || !strings.Contains(value.Email, "@") {
			return "", errRejected
		}
		return value.Email, nil
	}
	if response.StatusCode != http.StatusNoContent {
		return "", errRejected
	}
	if !strings.Contains(email, "@") {
		return "", errRejected
	}
	return email, nil
}
