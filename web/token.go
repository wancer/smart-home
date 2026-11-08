package web

import (
	"time"

	"github.com/go-chi/jwtauth/v5"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

const (
	jwtIssuer   = "smart-home"
	jwtAudience = "smart-home-web"
	jwtSkew     = 30 * time.Second
)

// NewTokenAuth returns an HS256-only JWT verifier/signer. Tokens must carry
// sub, iss, aud, iat, nbf and exp, with iss/aud matching this service.
// Tokens signed with any other algorithm (including "none") fail to decode.
func NewTokenAuth(secret []byte) *jwtauth.JWTAuth {
	return jwtauth.New("HS256", secret, nil,
		jwt.WithIssuer(jwtIssuer),
		jwt.WithAudience(jwtAudience),
		jwt.WithAcceptableSkew(jwtSkew),
		jwt.WithRequiredClaim(jwt.SubjectKey),
		jwt.WithRequiredClaim(jwt.IssuedAtKey),
		jwt.WithRequiredClaim(jwt.NotBeforeKey),
		jwt.WithRequiredClaim(jwt.ExpirationKey),
	)
}

// IssueToken signs a token for sub, valid from now for ttl.
func IssueToken(ja *jwtauth.JWTAuth, sub string, ttl time.Duration, now time.Time) (string, error) {
	_, s, err := ja.Encode(map[string]interface{}{
		jwt.SubjectKey:    sub,
		jwt.IssuerKey:     jwtIssuer,
		jwt.AudienceKey:   []string{jwtAudience},
		jwt.IssuedAtKey:   now,
		jwt.NotBeforeKey:  now,
		jwt.ExpirationKey: now.Add(ttl),
	})
	return s, err
}
