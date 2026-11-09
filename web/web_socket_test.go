package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"smart-home/config"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	privateauth "github.com/wancer/go-private-auth"
)

func newTestAuth(t *testing.T, ttl time.Duration) *privateauth.Auth {
	t.Helper()
	a, err := privateauth.New(privateauth.Config{
		Issuer:         "smart-home",
		Audience:       "smart-home-web",
		Secret:         "0123456789abcdef0123456789abcdef-test",
		TTL:            ttl,
		GoogleClientID: "cid",
		Allowed:        []privateauth.User{{Sub: "sub-1", Email: "me@example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newTestServer(t *testing.T, auth *privateauth.Auth) (*httptest.Server, *Server) {
	t.Helper()
	// Controllers are unused by these tests; nil receivers are fine for routing.
	srv := NewWebServer(&config.WebConfig{Cors: &config.CorsConfig{}}, auth, NewWebSocketServer(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	hs := httptest.NewServer(srv.router)
	t.Cleanup(hs.Close)
	return hs, srv
}

func wsURL(hs *httptest.Server, token string) string {
	return "ws" + strings.TrimPrefix(hs.URL, "http") + "/api/ws?jwt=" + token
}

func TestWebSocketRejectsBadToken(t *testing.T) {
	hs, _ := newTestServer(t, newTestAuth(t, time.Hour))
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(hs, "garbage"), nil)
	if err == nil {
		t.Fatal("expected dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", resp)
	}
}

func TestWebSocketClosesOnTokenExpiry(t *testing.T) {
	auth := newTestAuth(t, 2*time.Second)
	hs, _ := newTestServer(t, auth)
	token, _, err := auth.IssueForFirstUser()
	if err != nil {
		t.Fatal(err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(hs, token), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	start := time.Now()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()

	var ce *websocket.CloseError
	if !errors.As(err, &ce) || ce.Code != wsCloseTokenExpired {
		t.Fatalf("expected close %d, got %v", wsCloseTokenExpired, err)
	}
	// exp has second precision, so the close lands 1-2 s after dialing.
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("closed too late: %s", d)
	}
}
