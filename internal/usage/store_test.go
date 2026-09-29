package usage_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
	"infra.local/platform/internal/access"
	"infra.local/platform/internal/platform/database"
	"infra.local/platform/internal/usage"
)

func id() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func fixture(t *testing.T) (*usage.Store, *usage.Store, usage.Attempt) {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	admin, err := database.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	schema := "usage_" + id()
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	p1, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	p2, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		p1.Close()
		p2.Close()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	if err := database.Migrate(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx, p1); err != nil {
		t.Fatal(err)
	}
	if err := database.Check(ctx, p1); err != nil {
		t.Fatal(err)
	}
	a := &access.Store{Pool: p1}
	owner, _, err := a.Login(ctx, "https://test.example", "owner", "owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	project, err := a.CreateProject(ctx, owner.OrganizationID, "Usage test")
	if err != nil {
		t.Fatal(err)
	}
	key, err := a.IssueKey(ctx, owner.OrganizationID, project.ID, "Test")
	if err != nil {
		t.Fatal(err)
	}
	return &usage.Store{Pool: p1}, &usage.Store{Pool: p2}, usage.Attempt{ID: id(), OrganizationID: owner.OrganizationID, ProjectID: project.ID, KeyID: key.Key.ID, Method: "eth_blockNumber"}
}

func TestTwoReplicasEnforceBudgetAndDeduplicate(t *testing.T) {
	s, other, a := fixture(t)
	ctx := context.Background()
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 7, MinuteRequests: 600}); err != nil {
		t.Fatal(err)
	}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			attempt := a
			attempt.ID = id()
			store := s
			if i%2 == 1 {
				store = other
			}
			err := store.Admit(ctx, attempt)
			if err == nil {
				admitted.Add(1)
				if err := store.Finish(ctx, attempt.ID, "succeeded"); err != nil {
					t.Error(err)
				}
				if err := store.Finish(ctx, attempt.ID, "upstream_error"); err != nil {
					t.Error(err)
				}
			} else {
				var limit *usage.Limited
				if !errors.As(err, &limit) {
					t.Error(err)
				}
			}
		}(i)
	}
	wg.Wait()
	if admitted.Load() != 7 {
		t.Fatalf("admitted %d", admitted.Load())
	}
	summary, err := s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UsedUnits != 7 || summary.RemainingUnits != 0 || summary.Days[0].Succeeded != 7 || summary.Days[0].Pending != 0 {
		t.Fatalf("bad accounting: %+v", summary)
	}
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 8, MinuteRequests: 600}); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); !errors.Is(err, usage.ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := s.Finish(ctx, a.ID, "timed_out"); err != nil {
		t.Fatal(err)
	}
	summary, err = s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UsedUnits != 8 || summary.Days[0].TimedOut != 1 {
		t.Fatal(summary)
	}
}

func TestLimitsIsolationWindowsAndOutages(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	if _, err := s.Summary(ctx, "other-org", a.ProjectID); !errors.Is(err, usage.ErrNotFound) {
		t.Fatal("cross tenant summary", err)
	}
	if err := s.SetLimits(ctx, "other-org", a.ProjectID, usage.Limits{}); !errors.Is(err, usage.ErrNotFound) {
		t.Fatal("cross tenant limits", err)
	}
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 100001, MinuteRequests: 60}); !errors.Is(err, usage.ErrInvalid) {
		t.Fatal(err)
	}
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 5, MinuteRequests: 1}); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	a.ID = id()
	var limit *usage.Limited
	if err := s.Admit(ctx, a); !errors.As(err, &limit) {
		t.Fatal("minute limit", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE project_limits SET minute_start=clock_timestamp()-interval '2 minutes' WHERE project_id=$1`, a.ProjectID); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 1, MinuteRequests: 600}); err != nil {
		t.Fatal(err)
	}
	a.ID = id()
	if err := s.Admit(ctx, a); !errors.As(err, &limit) {
		t.Fatal("lowering reset counters", err)
	}
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 100, MinuteRequests: 0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); !errors.As(err, &limit) {
		t.Fatal("zero did not disable", err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE api_keys SET revoked_at=clock_timestamp() WHERE id=$1`, a.KeyID); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); !errors.Is(err, usage.ErrKey) {
		t.Fatal("revoked", err)
	}
	s.Pool.Close()
	if err := s.Admit(ctx, a); err == nil {
		t.Fatal("database outage allowed admission")
	}
}

func TestRecoveryAndRetention(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE usage_attempts SET created_at=clock_timestamp()-interval '40 seconds' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := s.Maintain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Finish(ctx, a.ID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Days[0].Unknown != 1 || summary.Days[0].Pending != 0 {
		t.Fatal(summary)
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE usage_attempts SET created_at=clock_timestamp()-interval '31 days' WHERE id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_attempts`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("old attempts retained")
	}
	summary, err = s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UsedUnits != 1 {
		t.Fatal("retention erased aggregate")
	}
}

func TestCosts(t *testing.T) {
	for _, method := range []string{"eth_chainId", "eth_blockNumber", "eth_getBalance", "eth_getCode", "eth_getTransactionReceipt"} {
		if units, version := usage.Cost(method); units != 1 || version != 1 {
			t.Fatal(method)
		}
	}
	if units, _ := usage.Cost("eth_sendRawTransaction"); units != 0 {
		t.Fatal("write cost accepted")
	}
}
