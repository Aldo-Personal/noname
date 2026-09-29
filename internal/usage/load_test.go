package usage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"infra.local/platform/internal/access"
	"infra.local/platform/internal/ethereum"
	"infra.local/platform/internal/usage"
)

type loadAuth struct{ identity access.KeyIdentity }

func (a loadAuth) AuthenticateKey(context.Context, string) (access.KeyIdentity, error) {
	return a.identity, nil
}

// A reproducible bounded workload, not a production capacity benchmark.
func TestGatewayLoad(t *testing.T) {
	for _, providerError := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider_error_%t", providerError), func(t *testing.T) {
			first, second, a := fixture(t)
			ctx := context.Background()
			if err := first.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 50, MinuteRequests: 600}); err != nil {
				t.Fatal(err)
			}
			var upstreamCalls atomic.Int32
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Method string `json:"method"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if req.Method != "eth_chainId" {
					upstreamCalls.Add(1)
					time.Sleep(2 * time.Millisecond)
					if providerError {
						w.WriteHeader(502)
						return
					}
				}
				w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
			}))
			defer provider.Close()
			c1, err := ethereum.NewClient(ctx, provider.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c1.Close()
			c2, err := ethereum.NewClient(ctx, provider.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer c2.Close()
			auth := loadAuth{access.KeyIdentity{ChainID: 1, KeyID: a.KeyID, ProjectID: a.ProjectID, OrganizationID: a.OrganizationID}}
			g1 := httptest.NewServer(ethereum.NewHandler(auth, c1, first))
			defer g1.Close()
			g2 := httptest.NewServer(ethereum.NewHandler(auth, c2, second))
			defer g2.Close()
			httpClient := &http.Client{Timeout: 15 * time.Second}
			defer httpClient.CloseIdleConnections()
			const requests = 80
			latencies := make([]time.Duration, requests)
			var admitted, rejected atomic.Int32
			var wg sync.WaitGroup
			started := time.Now()
			for worker := 0; worker < 8; worker++ {
				wg.Add(1)
				go func(worker int) {
					defer wg.Done()
					for i := worker; i < requests; i += 8 {
						base := g1.URL
						if i%2 == 1 {
							base = g2.URL
						}
						request, err := http.NewRequest("POST", base+"/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`))
						if err != nil {
							t.Error(err)
							return
						}
						request.Header.Set("Content-Type", "application/json")
						request.Header.Set("Authorization", "Bearer test")
						start := time.Now()
						response, err := httpClient.Do(request)
						if err != nil {
							t.Error(err)
							return
						}
						_, err = io.Copy(io.Discard, response.Body)
						response.Body.Close()
						if err != nil {
							t.Error(err)
							return
						}
						latencies[i] = time.Since(start)
						expected := 200
						if providerError {
							expected = 502
						}
						switch response.StatusCode {
						case expected:
							admitted.Add(1)
						case 429:
							rejected.Add(1)
						default:
							t.Errorf("unexpected status %d", response.StatusCode)
						}
					}
				}(worker)
			}
			wg.Wait()
			elapsed := time.Since(started)
			if admitted.Load() != 50 || rejected.Load() != 30 || upstreamCalls.Load() != 50 {
				t.Fatalf("admitted %d rejected %d provider calls %d", admitted.Load(), rejected.Load(), upstreamCalls.Load())
			}
			summary, err := first.Summary(ctx, a.OrganizationID, a.ProjectID)
			if err != nil {
				t.Fatal(err)
			}
			if summary.UsedUnits != 50 || summary.Days[0].Pending != 0 {
				t.Fatal(summary)
			}
			if providerError && summary.Days[0].UpstreamError != 50 {
				t.Fatal(summary)
			}
			if !providerError && summary.Days[0].Succeeded != 50 {
				t.Fatal(summary)
			}
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			t.Logf("2 gateways, 8 clients, 80 requests: elapsed=%s throughput=%.1f HTTP req/s p50=%s p95=%s admitted=50 quota_rejected=30", elapsed, float64(requests)/elapsed.Seconds(), latencies[requests/2], latencies[(requests*95/100)-1])
		})
	}
}
