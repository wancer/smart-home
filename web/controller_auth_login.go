package web

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"smart-home/config"

	"github.com/go-chi/jwtauth/v5"
	"google.golang.org/api/oauth2/v2"
)

type AuthController struct {
	cfg       *config.GoogleOauth
	tokenAuth *jwtauth.JWTAuth
}

func NewAuthController(
	cfg *config.GoogleOauth,
	tokenAuth *jwtauth.JWTAuth,
) *AuthController {
	return &AuthController{
		cfg:       cfg,
		tokenAuth: tokenAuth,
	}
}

type AuthRequest struct {
	Credentials string `json:"credential"`
	ClientId    string `json:"clientId"`
}

type AuthResponse struct {
	Token string `json:"token"`
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var parsed AuthRequest
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	var httpClient = &http.Client{}
	oauth2Service, err := oauth2.New(httpClient)
	tokenInfoCall := oauth2Service.Tokeninfo()
	tokenInfoCall.IdToken(parsed.Credentials)
	tokenInfo, err := tokenInfoCall.Do()
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if !slices.Contains(c.cfg.AllowedEmails, tokenInfo.Email) {
		slog.Error("[auth][login] not allowed", "err", err)
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	tokenData := map[string]interface{}{"user_id": 123}
	_, tokenString, err := c.tokenAuth.Encode(tokenData)
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	slog.Info("[auth][login] success")
	response := AuthResponse{
		Token: tokenString,
	}
	json.NewEncoder(w).Encode(response)
}

func (c *AuthController) Verify(w http.ResponseWriter, r *http.Request) {
}
