package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReadinessTransitions(t *testing.T) {
	var ready atomic.Bool
	h := Handler("api", &ready)
	for _, state := range []bool{false, true, false} {
		ready.Store(state)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
		want := 503
		if state {
			want = 200
		}
		if w.Code != want {
			t.Fatalf("got %d want %d", w.Code, want)
		}
	}
}
func TestStatusContract(t *testing.T) {
	var ready atomic.Bool
	w := httptest.NewRecorder()
	Handler("api", &ready).ServeHTTP(w, httptest.NewRequest("GET", "/v1/status", nil))
	var status Status
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || status.Service != "api" || status.Mode != "scaffold" || status.Version == "" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(w.Header().Get("X-Request-ID")) != 32 {
		t.Fatal("missing request ID")
	}
}
func TestGatewayDisabled(t *testing.T) {
	var ready atomic.Bool
	w := httptest.NewRecorder()
	Handler("gateway", &ready).ServeHTTP(w, httptest.NewRequest("POST", "/rpc", strings.NewReader(`{}`)))
	if w.Code != 503 {
		t.Fatalf("unsafe gateway status %d", w.Code)
	}
}
func TestConfig(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	if _, err := Address("api"); err == nil {
		t.Fatal("production must be blocked")
	}
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDR", "127.0.0.1:abc")
	if _, err := Address("api"); err == nil {
		t.Fatal("invalid port accepted")
	}
	t.Setenv("HTTP_ADDR", "")
	addr, err := Address("api")
	if err != nil || addr != "127.0.0.1:8080" {
		t.Fatalf("%s %v", addr, err)
	}
}
