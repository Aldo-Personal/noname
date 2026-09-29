package ethereum_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"infra.local/platform/internal/access"
	"infra.local/platform/internal/ethereum"
	"infra.local/platform/internal/platform/database"
)

func TestPostgresKeyLifecycleThroughGateway(t *testing.T) {
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatal(err)
	}
	schema := pgx.Identifier{"rpc_test_" + hex.EncodeToString(suffix)}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", strings.Trim(schema, `"`))
	u.RawQuery = q.Encode()
	pool, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	s := &access.Store{Pool: pool}
	p, _, err := s.Login(ctx, "https://issuer.test", "rpc-alice", "alice@example.test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := s.CreateProject(ctx, p.OrganizationID, "RPC integration")
	if err != nil {
		t.Fatal(err)
	}
	key, err := s.IssueKey(ctx, p.OrganizationID, project.ID, "Server")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x1"}`))
	}))
	defer upstream.Close()
	client, err := ethereum.NewClient(ctx, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	h := ethereum.NewHandler(s, client)
	check := func(secret string, status int) {
		t.Helper()
		before := calls.Load()
		r := httptest.NewRequest("POST", "/rpc", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"eth_blockNumber","params":[]}`))
		r.Header.Set("Authorization", "Bearer "+secret)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("got %d expected %d: %s", w.Code, status, w.Body.String())
		}
		if status != 200 && calls.Load() != before {
			t.Fatal("denied request reached provider")
		}
	}
	check(key.Secret, 200)
	rotated, err := s.RotateKey(ctx, p.OrganizationID, project.ID, key.Key.ID)
	if err != nil {
		t.Fatal(err)
	}
	check(key.Secret, 401)
	check(rotated.Secret, 200)
	if err := s.RevokeKey(ctx, p.OrganizationID, project.ID, rotated.Key.ID); err != nil {
		t.Fatal(err)
	}
	check(rotated.Secret, 401)
	expired, err := s.IssueKey(ctx, p.OrganizationID, project.ID, "Expired")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE api_keys SET expires_at=now()-interval '1 second' WHERE id=$1`, expired.Key.ID); err != nil {
		t.Fatal(err)
	}
	check(expired.Secret, 401)
	check("infra_sk_"+strings.Repeat("0", 64), 401)
	pool.Close()
	check(expired.Secret, 503)
}
