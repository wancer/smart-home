package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// Every route except login must reject requests without a valid JWT.
func TestRoutesRequireAuth(t *testing.T) {
	auth := newTestAuth(t, time.Hour)
	_, srv := newTestServer(t, auth)
	tok, _, _ := auth.IssueForFirstUser()

	params := regexp.MustCompile(`\{[^}]+\}`)
	checked := 0
	_ = chi.Walk(srv.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method+" "+route == "POST /api/auth/login" {
			return nil
		}
		checked++
		path := params.ReplaceAllString(route, "1")
		reqs := map[string]*http.Request{
			"no token":      httptest.NewRequest(method, path, nil),
			"garbage token": httptest.NewRequest(method, path+"?jwt=garbage", nil),
		}
		reqs["garbage token"].Header.Set("Authorization", "Bearer garbage")
		if route == "/api/ws" {
			// The WebSocket only accepts ?jwt=, never a header.
			r := httptest.NewRequest(method, path, nil)
			r.Header.Set("Authorization", "Bearer "+tok)
			reqs["header token on ws"] = r
		} else {
			// REST routes only accept the header, never ?jwt=.
			reqs["query token on rest"] = httptest.NewRequest(method, path+"?jwt="+tok, nil)
		}
		for name, req := range reqs {
			rec := httptest.NewRecorder()
			srv.router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s (%s): %d, want 401", method, route, name, rec.Code)
			}
		}
		return nil
	})
	if checked < 20 {
		t.Fatalf("only %d routes checked", checked)
	}

	req := httptest.NewRequest("GET", "/api/auth/verify", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify with valid token: %d", rec.Code)
	}
}
