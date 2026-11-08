package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"smart-home/config"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"google.golang.org/api/idtoken"
)

const testClientID = "client-123.apps.googleusercontent.com"

// fakeValidator mimics idtoken.Validate: it rejects a wrong audience and
// otherwise returns the given payload.
func fakeValidator(p *idtoken.Payload, err error) idTokenValidator {
	return func(_ context.Context, _ string, audience string) (*idtoken.Payload, error) {
		if err != nil {
			return nil, err
		}
		if p.Audience != audience {
			return nil, errors.New("idtoken: audience provided does not match aud claim in the JWT")
		}
		return p, nil
	}
}

func googlePayload() *idtoken.Payload {
	return &idtoken.Payload{
		Issuer:   "https://accounts.google.com",
		Audience: testClientID,
		Subject:  "sub-ok",
		Claims:   map[string]interface{}{"email": "me@example.com", "email_verified": true},
	}
}

func newTestAuth(v idTokenValidator) *AuthController {
	c := NewAuthController(&config.GoogleOauth{
		ClientID:      testClientID,
		AllowedSubs:   []string{"sub-ok"},
		AllowedEmails: []string{"me@example.com"},
	}, NewTokenAuth(testSecret), time.Hour)
	c.validate = v
	return c
}

func doLogin(c *AuthController, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	c.Login(rec, req)
	return rec
}

func TestLoginSuccess(t *testing.T) {
	c := newTestAuth(fakeValidator(googlePayload(), nil))
	rec := doLogin(c, `{"credential":"x"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	var resp AuthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	tok, err := jwtauth.VerifyToken(c.tokenAuth, resp.Token)
	if err != nil {
		t.Fatalf("issued token rejected: %v", err)
	}
	if sub, _ := tok.Subject(); sub != "sub-ok" {
		t.Fatalf("sub = %q", sub)
	}
}

func TestLoginRejections(t *testing.T) {
	mod := func(f func(p *idtoken.Payload)) *idtoken.Payload {
		p := googlePayload()
		p.Claims = map[string]interface{}{"email": "me@example.com", "email_verified": true}
		f(p)
		return p
	}

	cases := []struct {
		name string
		v    idTokenValidator
		body string
		want int
	}{
		{"bad json", fakeValidator(googlePayload(), nil), `{`, http.StatusBadRequest},
		{"empty credential", fakeValidator(googlePayload(), nil), `{}`, http.StatusBadRequest},
		{"oversized body", fakeValidator(googlePayload(), nil), `{"credential":"` + strings.Repeat("a", maxLoginBody) + `"}`, http.StatusBadRequest},
		{"invalid google token", fakeValidator(nil, errors.New("idtoken: token expired")), `{"credential":"x"}`, http.StatusUnauthorized},
		{"wrong audience", fakeValidator(mod(func(p *idtoken.Payload) { p.Audience = "someone-else" }), nil), `{"credential":"x"}`, http.StatusUnauthorized},
		{"wrong issuer", fakeValidator(mod(func(p *idtoken.Payload) { p.Issuer = "https://evil.example" }), nil), `{"credential":"x"}`, http.StatusUnauthorized},
		{"unverified email", fakeValidator(mod(func(p *idtoken.Payload) { p.Claims["email_verified"] = false }), nil), `{"credential":"x"}`, http.StatusForbidden},
		{"missing email_verified", fakeValidator(mod(func(p *idtoken.Payload) { delete(p.Claims, "email_verified") }), nil), `{"credential":"x"}`, http.StatusForbidden},
		{"sub not whitelisted", fakeValidator(mod(func(p *idtoken.Payload) { p.Subject = "sub-other" }), nil), `{"credential":"x"}`, http.StatusForbidden},
		{"email not whitelisted", fakeValidator(mod(func(p *idtoken.Payload) { p.Claims["email"] = "other@example.com" }), nil), `{"credential":"x"}`, http.StatusForbidden},
		{"empty sub", fakeValidator(mod(func(p *idtoken.Payload) { p.Subject = "" }), nil), `{"credential":"x"}`, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doLogin(newTestAuth(tc.v), tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if strings.Contains(rec.Body.String(), "token") {
				t.Fatal("rejected login returned a token")
			}
		})
	}
}
