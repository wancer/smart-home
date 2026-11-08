package web

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

var testSecret = []byte("0123456789abcdef0123456789abcdef-test")

// sign builds a token from claims and signs it with alg/key, bypassing IssueToken.
func sign(t *testing.T, alg jwa.SignatureAlgorithm, key []byte, claims map[string]any) string {
	t.Helper()
	tok := jwt.New()
	for k, v := range claims {
		if err := tok.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	b, err := jwt.Sign(tok, jwt.WithKey(alg, key))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func validClaims(now time.Time) map[string]any {
	return map[string]any{
		jwt.SubjectKey:    "1234567890",
		jwt.IssuerKey:     jwtIssuer,
		jwt.AudienceKey:   []string{jwtAudience},
		jwt.IssuedAtKey:   now,
		jwt.NotBeforeKey:  now,
		jwt.ExpirationKey: now.Add(time.Hour),
	}
}

func without(m map[string]any, key string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func with(m map[string]any, key string, v any) map[string]any {
	out := without(m, key)
	out[key] = v
	return out
}

// unsigned builds an alg=none token by hand.
func unsigned(t *testing.T, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return base64.RawURLEncoding.EncodeToString(b)
	}
	c := map[string]any{}
	for k, v := range claims {
		if tm, ok := v.(time.Time); ok {
			v = tm.Unix()
		}
		c[k] = v
	}
	return enc(map[string]string{"alg": "none", "typ": "JWT"}) + "." + enc(c) + "."
}

func TestIssueTokenRoundTrip(t *testing.T) {
	ja := NewTokenAuth(testSecret)
	s, err := IssueToken(ja, "sub-1", time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tok, err := jwtauth.VerifyToken(ja, s)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if sub, _ := tok.Subject(); sub != "sub-1" {
		t.Fatalf("sub = %q", sub)
	}
}

func TestVerifyToken(t *testing.T) {
	ja := NewTokenAuth(testSecret)
	now := time.Now()
	good := validClaims(now)

	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid", sign(t, jwa.HS256(), testSecret, good), true},
		{"within clock skew", sign(t, jwa.HS256(), testSecret, with(good, jwt.ExpirationKey, now.Add(-10*time.Second))), true},
		{"expired", sign(t, jwa.HS256(), testSecret, with(good, jwt.ExpirationKey, now.Add(-time.Minute))), false},
		{"not yet valid", sign(t, jwa.HS256(), testSecret, with(good, jwt.NotBeforeKey, now.Add(time.Minute))), false},
		{"no exp", sign(t, jwa.HS256(), testSecret, without(good, jwt.ExpirationKey)), false},
		{"no iat", sign(t, jwa.HS256(), testSecret, without(good, jwt.IssuedAtKey)), false},
		{"no nbf", sign(t, jwa.HS256(), testSecret, without(good, jwt.NotBeforeKey)), false},
		{"no sub", sign(t, jwa.HS256(), testSecret, without(good, jwt.SubjectKey)), false},
		{"no iss", sign(t, jwa.HS256(), testSecret, without(good, jwt.IssuerKey)), false},
		{"wrong iss", sign(t, jwa.HS256(), testSecret, with(good, jwt.IssuerKey, "evil")), false},
		{"no aud", sign(t, jwa.HS256(), testSecret, without(good, jwt.AudienceKey)), false},
		{"wrong aud", sign(t, jwa.HS256(), testSecret, with(good, jwt.AudienceKey, []string{"other"})), false},
		{"wrong key", sign(t, jwa.HS256(), []byte("another-secret-another-secret-xx"), good), false},
		{"HS512", sign(t, jwa.HS512(), testSecret, good), false},
		{"alg none", unsigned(t, good), false},
		{"legacy user_id token", sign(t, jwa.HS256(), testSecret, map[string]any{"user_id": 123}), false},
		{"garbage", "not.a.jwt", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := jwtauth.VerifyToken(ja, tc.token)
			if tc.ok && err != nil {
				t.Fatalf("expected accept, got %v", err)
			}
			if !tc.ok && err == nil {
				t.Fatal("expected reject, got accept")
			}
		})
	}
}

func TestUnsignedHelperIsWellFormed(t *testing.T) {
	if parts := strings.Split(unsigned(t, map[string]any{"a": 1}), "."); len(parts) != 3 {
		t.Fatal("bad alg=none token shape")
	}
}
