package usage_test

import (
	"context"
	"testing"

	"infra.local/platform/internal/usage"
)

func TestAdmissionAndCompletionRollbackAtomically(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 10, MinuteRequests: 10}); err != nil {
		t.Fatal(err)
	}
	// Fail the final admission write, after the attempt and daily reservation were inserted.
	if _, err := s.Pool.Exec(ctx, `CREATE FUNCTION reject_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$; CREATE TRIGGER reject_budget BEFORE UPDATE ON project_limits FOR EACH ROW EXECUTE FUNCTION reject_write()`); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); err == nil {
		t.Fatal("injected transaction failure ignored")
	}
	var attempts int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_attempts`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 0 || summary.UsedUnits != 0 {
		t.Fatal("partial admission persisted")
	}
	if _, err := s.Pool.Exec(ctx, `DROP TRIGGER reject_budget ON project_limits`); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `CREATE TRIGGER reject_aggregate BEFORE UPDATE ON usage_days FOR EACH ROW EXECUTE FUNCTION reject_write()`); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, a.ID, "succeeded"); err == nil {
		t.Fatal("completion failure ignored")
	}
	var outcome string
	if err := s.Pool.QueryRow(ctx, `SELECT outcome FROM usage_attempts WHERE id=$1`, a.ID).Scan(&outcome); err != nil {
		t.Fatal(err)
	}
	summary, err = s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != "pending" || summary.Days[0].Pending != 1 {
		t.Fatal("partial completion persisted")
	}
	if _, err := s.Pool.Exec(ctx, `DROP TRIGGER reject_aggregate ON usage_days`); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, a.ID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, a.ID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	summary, err = s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UsedUnits != 1 || summary.Days[0].Succeeded != 1 {
		t.Fatal(summary)
	}
}

func TestDailyRolloverAndAggregateRetention(t *testing.T) {
	s, _, a := fixture(t)
	ctx := context.Background()
	if err := s.SetLimits(ctx, a.OrganizationID, a.ProjectID, usage.Limits{DailyUnits: 1, MinuteRequests: 600}); err != nil {
		t.Fatal(err)
	}
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, a.ID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	// Move the closed fixture day into yesterday; current-day admission must regain capacity.
	if _, err := s.Pool.Exec(ctx, `UPDATE usage_days SET day=day-1; UPDATE usage_attempts SET day=day-1`); err != nil {
		t.Fatal(err)
	}
	a.ID = id()
	if err := s.Admit(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Finish(ctx, a.ID, "succeeded"); err != nil {
		t.Fatal(err)
	}
	summary, err := s.Summary(ctx, a.OrganizationID, a.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.UsedUnits != 1 || len(summary.Days) != 2 {
		t.Fatal(summary)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO usage_days(project_id,day) VALUES($1,(clock_timestamp() AT TIME ZONE 'UTC')::date-366)`, a.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Maintain(ctx); err != nil {
		t.Fatal(err)
	}
	var days int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM usage_days`).Scan(&days); err != nil {
		t.Fatal(err)
	}
	if days != 2 {
		t.Fatal("expired aggregate not pruned")
	}
}
