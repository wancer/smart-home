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
)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	ja := NewTokenAuth(testSecret)
	// Controllers are unused by these tests; nil receivers are fine for routing.
	srv := NewWebServer(&config.WebConfig{Cors: &config.CorsConfig{}}, ja, NewWebSocketServer(),
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	hs := httptest.NewServer(srv.router)
	t.Cleanup(hs.Close)
	return hs, srv
}

func wsURL(hs *httptest.Server, token string) string {
	return "ws" + strings.TrimPrefix(hs.URL, "http") + "/api/ws?jwt=" + token
}

func TestWebSocketRejectsBadToken(t *testing.T) {
	hs, _ := newTestServer(t)
	_, resp, err := websocket.DefaultDialer.Dial(wsURL(hs, "garbage"), nil)
	if err == nil {
		t.Fatal("expected dial to fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %v", resp)
	}
}

func TestWebSocketClosesOnTokenExpiry(t *testing.T) {
	hs, _ := newTestServer(t)
	token, err := IssueToken(NewTokenAuth(testSecret), "sub-1", 2*time.Second, time.Now())
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
