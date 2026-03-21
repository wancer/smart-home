package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"smart-home/config"

	"google.golang.org/api/oauth2/v2"
)

type AuthController struct {
	cfg *config.GoogleOauth
}

func NewAuthController(cfg *config.GoogleOauth) *AuthController {
	return &AuthController{
		cfg: cfg,
	}
}

type MyRequest struct {
	Credentials string `json:"credential"`
	ClientId    string `json:"clientId"`
}

func (c *AuthController) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Accept")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}
	fmt.Println(string(body))

	var parsed MyRequest
	err = json.Unmarshal(body, &parsed)
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	// ctx := context.Background()
	var httpClient = &http.Client{}
	oauth2Service, err := oauth2.New(httpClient)
	tokenInfoCall := oauth2Service.Tokeninfo()
	tokenInfoCall.IdToken(parsed.Credentials)
	tokenInfo, err := tokenInfoCall.Do()
	if err != nil {
		slog.Error("[auth][login] error", "err", err)
		http.Error(w, http.StatusText(500), 500)
		return
	}

	fmt.Println(tokenInfo)

	good := slices.Contains(c.cfg.AllowedEmails, tokenInfo.Email)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(good)
}

// SendRequest sends an HTTP request with the given method, URL, access token, body, and headers.
func SendRequest(ctx context.Context, methodType, url, accessToken string, body io.Reader, headers map[string]string) ([]byte, error) {

	// Create a new HTTP request with the provided context, method, URL, and body.
	serverRequest, err := http.NewRequestWithContext(ctx, methodType, url, body)
	if err != nil {
		return nil, err // Return error if request creation fails.
	}

	// Set headers for the request.
	for key, value := range headers {
		serverRequest.Header.Set(key, value)
	}

	// Execute the HTTP request.
	resp, err := http.DefaultClient.Do(serverRequest)
	if err != nil {
		return nil, err // Return error if request execution fails.
	}
	defer resp.Body.Close() // Ensure response body is closed after reading.

	// Read and return the response body.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err // Return error if reading response body fails.
	}

	return respBody, nil
}
