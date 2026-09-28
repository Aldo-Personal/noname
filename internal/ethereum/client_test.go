package ethereum

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		wantErr    bool
	}{
		{"valid", `{"jsonrpc":"2.0","id":1,"result":"0xff"}`, 200, false},
		{"wrong id", `{"jsonrpc":"2.0","id":2,"result":"0xff"}`, 200, true},
		{"missing id", `{"jsonrpc":"2.0","result":"0xff"}`, 200, true},
		{"wrong version", `{"jsonrpc":"1.0","id":1,"result":"0xff"}`, 200, true},
		{"both", `{"jsonrpc":"2.0","id":1,"result":"0xff","error":null}`, 200, true},
		{"error", `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"secret-url"}}`, 200, true},
		{"html", `<html>secret-url</html>`, 200, true},
		{"status", `secret-url`, 429, true},
		{"oversize", strings.Repeat(" ", MaxResponseBytes+1), 200, true},
		{"bad result", `{"jsonrpc":"2.0","id":1,"result":12}`, 200, true},
		{"trailing", `{"jsonrpc":"2.0","id":1,"result":"0xff"} {}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var req request
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("credential forwarded")
				}
				if req.Method == "eth_chainId" {
					w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
					return
				}
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer s.Close()
			c, err := NewClient(context.Background(), s.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			_, err = c.call(context.Background(), request{JSONRPC: "2.0", ID: json.RawMessage(`"customer"`), Method: "eth_blockNumber", Params: []json.RawMessage{}})
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("provider error leaked")
			}
			if calls != 2 {
				t.Fatalf("unexpected retries: %d calls", calls)
			}
		})
	}
}

func TestStartupAndRedirects(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "http://example.com", "https://user:secret@example.com", "https://example.com/#secret"} {
		if _, err := NewClient(context.Background(), raw); err == nil {
			t.Fatalf("accepted %s", raw)
		}
	}
	for _, body := range []string{`{"jsonrpc":"2.0","id":1,"result":"0x89"}`, `{}`} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(body)) }))
		_, err := NewClient(context.Background(), s.URL)
		s.Close()
		if err == nil {
			t.Fatal("invalid chain accepted")
		}
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("redirect followed") }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer s.Close()
	if _, err := NewClient(context.Background(), s.URL); err == nil {
		t.Fatal("redirect accepted")
	}
}

func TestTimeoutAndCancellation(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req request
		json.NewDecoder(r.Body).Decode(&req)
		if req.Method == "eth_chainId" {
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
			return
		}
		<-r.Context().Done()
	}))
	defer s.Close()
	c, err := NewClient(context.Background(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := c.call(ctx, request{JSONRPC: "2.0", Method: "eth_blockNumber"}); !errors.Is(err, errTimeout) {
		t.Fatalf("timeout: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := c.call(ctx, request{JSONRPC: "2.0", Method: "eth_blockNumber"}); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
