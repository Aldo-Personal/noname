package ethereum

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"infra.local/platform/internal/access"
)

type authFunc func(context.Context, string) (access.KeyIdentity, error)

func (f authFunc) AuthenticateKey(ctx context.Context, key string) (access.KeyIdentity, error) {
	return f(ctx, key)
}

func TestHandlerBoundary(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("X-Secret") != "" {
			t.Error("customer headers forwarded")
		}
		w.Header().Set("Set-Cookie", "provider-secret=x")
		w.Header().Set("X-Provider-Secret", "hidden")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer s.Close()
	c, err := NewClient(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	auth := authFunc(func(ctx context.Context, key string) (access.KeyIdentity, error) {
		switch key {
		case "valid":
			return access.KeyIdentity{ChainID: 1}, nil
		case "database-down":
			return access.KeyIdentity{}, errors.New("secret database URL")
		case "wrong-chain":
			return access.KeyIdentity{ChainID: 2}, nil
		}
		return access.KeyIdentity{}, access.ErrNotFound
	})
	h := NewHandler(auth, c)
	valid := `{"jsonrpc":"2.0","id":9007199254740993,"method":"eth_chainId","params":[]}`
	for _, tt := range []struct {
		name, key, body string
		status          int
	}{
		{"valid", "valid", valid, 200},
		{"missing", "", valid, 401}, {"unknown", "unknown", valid, 401}, {"revoked", "revoked", valid, 401},
		{"database", "database-down", valid, 503}, {"chain", "wrong-chain", valid, 403},
		{"batch", "valid", "[]", 400}, {"notification", "valid", `{"jsonrpc":"2.0","method":"eth_chainId"}`, 400},
		{"write", "valid", `{"jsonrpc":"2.0","id":1,"method":"eth_sendRawTransaction","params":[]}`, 400},
		{"oversize", "valid", strings.Repeat(" ", MaxRequestBytes+1), 413},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := calls.Load()
			r := httptest.NewRequest("POST", "/rpc", strings.NewReader(tt.body))
			r.Header.Set("Content-Type", "application/json")
			if tt.key != "" {
				r.Header.Set("Authorization", "Bearer "+tt.key)
			}
			r.Header.Set("Cookie", "infra_session=secret")
			r.Header.Set("X-Secret", "secret")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status %d body %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "secret") || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Provider-Secret") != "" {
				t.Fatal("provider or database details exposed")
			}
			if tt.status != 200 && calls.Load() != before {
				t.Fatal("unauthorized or invalid request reached upstream")
			}
			if tt.status == 200 && !strings.Contains(w.Body.String(), `"id":9007199254740993`) {
				t.Fatal("ID precision lost")
			}
		})
	}
	for _, header := range []string{"text/plain", ""} {
		r := httptest.NewRequest("POST", "/rpc", strings.NewReader(valid))
		r.Header.Set("Authorization", "Bearer valid")
		r.Header.Set("Content-Type", header)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 415 {
			t.Fatal(w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/rpc", strings.NewReader(valid))
	r.Header.Add("Authorization", "Bearer valid")
	r.Header.Add("Authorization", "Bearer other")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("ambiguous credential accepted")
	}
}

func TestCapacityBoundsAuthorization(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	h := NewHandler(authFunc(func(context.Context, string) (access.KeyIdentity, error) {
		close(started)
		<-release
		return access.KeyIdentity{}, access.ErrNotFound
	}), nil)
	h.slots = make(chan struct{}, 1)
	r := func() *http.Request {
		r := httptest.NewRequest("POST", "/rpc", nil)
		r.Header.Set("Authorization", "Bearer valid")
		return r
	}
	done := make(chan struct{})
	go func() { defer close(done); h.ServeHTTP(httptest.NewRecorder(), r()) }()
	<-started
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r())
	if w.Code != 503 || !strings.Contains(w.Body.String(), "gateway_busy") {
		t.Fatal("capacity did not reject immediately")
	}
	// Release before returning so the test owns the complete goroutine lifetime.
	release <- struct{}{}
	<-done
}
