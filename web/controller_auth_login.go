package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"smart-home/config"
	"time"

	"github.com/go-chi/jwtauth/v5"
	"google.golang.org/api/idtoken"
)

const maxLoginBody = 64 << 10 // 64 KiB; a Google ID token is ~1-2 KiB

var googleIssuers = []string{"accounts.google.com", "https://accounts.google.com"}

// idTokenValidator verifies a Google ID token's signature, exp and aud.
type idTokenValidator func(ctx context.Context, idToken, audience string) (*idtoken.Payload, error)

type AuthController struct {
	cfg       *config.GoogleOauth
	tokenAuth *jwtauth.JWTAuth
	ttl       time.Duration
	validate  idTokenValidator
	now       func() time.Time
}

func NewAuthController(
	cfg *config.GoogleOauth,
	tokenAuth *jwtauth.JWTAuth,
	ttl time.Duration,
) *AuthController {
	return &AuthController{
		cfg:       cfg,
		tokenAuth: tokenAuth,
		ttl:       ttl,
		validate:  idtoken.Validate,
		now:       time.Now,
	}
}

type AuthRequest struct {
	Credentials string `json:"credential"`
}

type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	r.Body = http.MaxBytesReader(w, r.Body, maxLoginBody)
	var parsed AuthRequest
	if err := json.NewDecoder(r.Body).Decode(&parsed); err != nil || parsed.Credentials == "" {
		slog.Warn("[auth][login] bad request", "err", err)
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	// Signature, exp and aud (== our client ID from config) are checked here.
	payload, err := c.validate(r.Context(), parsed.Credentials, c.cfg.ClientID)
	if err != nil {
		slog.Warn("[auth][login] invalid id token", "err", err)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	// idtoken.Validate does not check the issuer.
	if !slices.Contains(googleIssuers, payload.Issuer) {
		slog.Warn("[auth][login] invalid issuer", "iss", payload.Issuer)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	email, _ := payload.Claims["email"].(string)
	verified, _ := payload.Claims["email_verified"].(bool)
	sub := payload.Subject

	if sub == "" || !verified ||
		!slices.Contains(c.cfg.AllowedSubs, sub) ||
		!slices.Contains(c.cfg.AllowedEmails, email) {
		slog.Warn("[auth][login] not allowed", "sub", sub, "email", email, "emailVerified", verified)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	now := c.now()
	tokenString, err := IssueToken(c.tokenAuth, sub, c.ttl, now)
	if err != nil {
		slog.Error("[auth][login] sign error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[auth][login] success", "sub", sub)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(AuthResponse{
		Token:     tokenString,
		ExpiresAt: now.Add(c.ttl).Unix(),
	})
}

func (c *AuthController) Verify(w http.ResponseWriter, r *http.Request) {
}
