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
	"infra.local/platform/internal/usage"
)

type testMeter struct {
	admit  func(context.Context, usage.Attempt) error
	finish func(context.Context, string, string) error
}

func (m testMeter) Admit(ctx context.Context, a usage.Attempt) error { return m.admit(ctx, a) }
func (m testMeter) Finish(ctx context.Context, id, outcome string) error {
	return m.finish(ctx, id, outcome)
}

func TestAccountingFailsClosedAndFinalizesAfterCancellation(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer upstream.Close()
	client, err := NewClient(context.Background(), upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	auth := authFunc(func(context.Context, string) (access.KeyIdentity, error) {
		return access.KeyIdentity{ChainID: 1, ProjectID: "project", OrganizationID: "org", KeyID: "key"}, nil
	})
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"quota", &usage.Limited{RetryAfter: 42}, 429},
		{"outage", errors.New("database unavailable"), 503},
		{"ambiguous commit", context.DeadlineExceeded, 503},
		{"duplicate", usage.ErrDuplicate, 503},
		{"revoked", usage.ErrKey, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			meter := testMeter{admit: func(context.Context, usage.Attempt) error { return tc.err }, finish: func(context.Context, string, string) error { t.Fatal("non-admitted finalized"); return nil }}
			h := NewHandler(auth, client, meter)
			r := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`))
			r.Header.Set("Authorization", "Bearer valid")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status || calls.Load() != before {
				t.Fatalf("status %d calls %d", w.Code, calls.Load()-before)
			}
			if tc.status == 429 && w.Header().Get("Retry-After") != "42" {
				t.Fatal("missing retry guidance")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := false
	meter := testMeter{admit: func(ctx context.Context, a usage.Attempt) error {
		if a.OrganizationID != "org" || len(a.ID) != 32 {
			t.Error("missing attribution")
		}
		cancel()
		return nil
	}, finish: func(ctx context.Context, id, outcome string) error {
		finished = true
		if ctx.Err() != nil || outcome != "canceled" {
			t.Errorf("completion %s %v", outcome, ctx.Err())
		}
		return nil
	}}
	h := NewHandler(auth, client, meter)
	r := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`)).WithContext(ctx)
	r.Header.Set("Authorization", "Bearer valid")
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if !finished {
		t.Fatal("disconnected request not finalized")
	}
}

func TestCompletionFailureKeepsProviderResponseAndEmitsMetric(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer upstream.Close()
	client, err := NewClient(context.Background(), upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	auth := authFunc(func(context.Context, string) (access.KeyIdentity, error) { return access.KeyIdentity{ChainID: 1}, nil })
	meter := testMeter{admit: func(context.Context, usage.Attempt) error { return nil }, finish: func(ctx context.Context, id, outcome string) error {
		if outcome != "succeeded" {
			t.Error(outcome)
		}
		return errors.New("offline")
	}}
	h := NewHandler(auth, client, meter)
	r := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber"}`))
	r.Header.Set("Authorization", "Bearer valid")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || len(w.Header().Get("X-Usage-Attempt-ID")) != 32 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.Metrics(w, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(w.Body.String(), "infra_usage_completion_errors_total 1") || !strings.Contains(w.Body.String(), `status_class="2xx"} 1`) {
		t.Fatal(w.Body.String())
	}
}
