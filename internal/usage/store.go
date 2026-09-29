// Package usage owns admission budgets and durable request accounting.
package usage

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("project not found")
var ErrInvalid = errors.New("invalid limits or attempt")
var ErrKey = errors.New("invalid key")
var ErrDuplicate = errors.New("duplicate attempt")

type Limited struct{ RetryAfter int }

func (e *Limited) Error() string { return "project limit exceeded" }

type Store struct{ Pool *pgxpool.Pool }
type Limits struct {
	DailyUnits     int64 `json:"dailyUnits"`
	MinuteRequests int64 `json:"minuteRequests"`
}
type Day struct {
	Day           string `json:"day"`
	Units         int64  `json:"units"`
	Pending       int64  `json:"pending"`
	Succeeded     int64  `json:"succeeded"`
	UpstreamError int64  `json:"upstreamError"`
	TimedOut      int64  `json:"timedOut"`
	Canceled      int64  `json:"canceled"`
	Unknown       int64  `json:"unknown"`
}
type Summary struct {
	Limits         Limits    `json:"limits"`
	UsedUnits      int64     `json:"usedUnits"`
	RemainingUnits int64     `json:"remainingUnits"`
	ResetsAt       time.Time `json:"resetsAt"`
	Days           []Day     `json:"days"`
}
type Attempt struct{ ID, OrganizationID, ProjectID, KeyID, Method string }

func Cost(method string) (int64, int) {
	switch method {
	case "eth_chainId", "eth_blockNumber", "eth_getBalance", "eth_getCode", "eth_getTransactionReceipt":
		return 1, 1
	}
	return 0, 0
}

func ensure(ctx context.Context, tx pgx.Tx, org, project string) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM projects WHERE id=$1 AND organization_id=$2)`, project, org).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	_, err := tx.Exec(ctx, `INSERT INTO project_limits(project_id) VALUES($1) ON CONFLICT DO NOTHING`, project)
	return err
}

func (s *Store) Admit(ctx context.Context, a Attempt) error {
	units, version := Cost(a.Method)
	if units == 0 || len(a.ID) != 32 {
		return ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := ensure(ctx, tx, a.OrganizationID, a.ProjectID); err != nil {
		return err
	}
	var limits Limits
	var minute time.Time
	var minuteUsed int64
	err = tx.QueryRow(ctx, `SELECT daily_units,minute_requests,minute_start,minute_used FROM project_limits WHERE project_id=$1 FOR UPDATE`, a.ProjectID).Scan(&limits.DailyUnits, &limits.MinuteRequests, &minute, &minuteUsed)
	if err != nil {
		return err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	var valid bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM api_keys WHERE id=$1 AND project_id=$2 AND revoked_at IS NULL AND expires_at>$3)`, a.KeyID, a.ProjectID, now).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return ErrKey
	}
	var duplicate bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM usage_attempts WHERE id=$1)`, a.ID).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate {
		return ErrDuplicate
	}
	now = now.UTC()
	day := now.Truncate(24 * time.Hour)
	currentMinute := now.Truncate(time.Minute)
	if !minute.Equal(currentMinute) {
		minuteUsed = 0
	}
	if _, err := tx.Exec(ctx, `INSERT INTO usage_days(project_id,day) VALUES($1,$2) ON CONFLICT DO NOTHING`, a.ProjectID, day); err != nil {
		return err
	}
	var used int64
	if err := tx.QueryRow(ctx, `SELECT units FROM usage_days WHERE project_id=$1 AND day=$2`, a.ProjectID, day).Scan(&used); err != nil {
		return err
	}
	if used+units > limits.DailyUnits {
		return &Limited{int(day.Add(24*time.Hour).Sub(now).Seconds()) + 1}
	}
	if minuteUsed+1 > limits.MinuteRequests {
		return &Limited{int(currentMinute.Add(time.Minute).Sub(now).Seconds()) + 1}
	}
	_, err = tx.Exec(ctx, `INSERT INTO usage_attempts(id,project_id,key_id,method,cost_version,units,day,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, a.ID, a.ProjectID, a.KeyID, a.Method, version, units, day, now)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE usage_days SET units=units+$3,pending=pending+$3 WHERE project_id=$1 AND day=$2`, a.ProjectID, day, units); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_limits SET minute_start=$2,minute_used=$3 WHERE project_id=$1`, a.ProjectID, currentMinute, minuteUsed+1); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Finish(ctx context.Context, id, outcome string) error {
	switch outcome {
	case "succeeded", "upstream_error", "timed_out", "canceled", "unknown":
	default:
		return ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var project, old string
	var day time.Time
	err = tx.QueryRow(ctx, `SELECT project_id,day,outcome FROM usage_attempts WHERE id=$1 FOR UPDATE`, id).Scan(&project, &day, &old)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if old != "pending" {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE usage_attempts SET outcome=$2,finished_at=clock_timestamp() WHERE id=$1`, id, outcome); err != nil {
		return err
	}
	// Column names are selected only from the closed outcome list above, never caller SQL.
	if _, err := tx.Exec(ctx, `UPDATE usage_days SET pending=pending-1,`+outcome+`=`+outcome+`+1 WHERE project_id=$1 AND day=$2`, project, day); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SetLimits(ctx context.Context, org, project string, limits Limits) error {
	if limits.DailyUnits < 0 || limits.DailyUnits > 100000 || limits.MinuteRequests < 0 || limits.MinuteRequests > 600 {
		return ErrInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := ensure(ctx, tx, org, project); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE project_limits SET daily_units=$2,minute_requests=$3 WHERE project_id=$1`, project, limits.DailyUnits, limits.MinuteRequests); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Summary(ctx context.Context, org, project string) (Summary, error) {
	result := Summary{Days: []Day{}}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT COALESCE(l.daily_units,10000),COALESCE(l.minute_requests,60),clock_timestamp() FROM projects p LEFT JOIN project_limits l ON l.project_id=p.id WHERE p.id=$1 AND p.organization_id=$2`, project, org).Scan(&result.Limits.DailyUnits, &result.Limits.MinuteRequests, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, ErrNotFound
	}
	if err != nil {
		return result, err
	}
	today := now.UTC().Truncate(24 * time.Hour)
	result.ResetsAt = today.Add(24 * time.Hour)
	rows, err := tx.Query(ctx, `SELECT day::text,units,pending,succeeded,upstream_error,timed_out,canceled,unknown FROM usage_days WHERE project_id=$1 AND day >= $2 AND day <= $3 ORDER BY day DESC LIMIT 7`, project, today.Add(-6*24*time.Hour), today)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.Day, &d.Units, &d.Pending, &d.Succeeded, &d.UpstreamError, &d.TimedOut, &d.Canceled, &d.Unknown); err != nil {
			return result, err
		}
		result.Days = append(result.Days, d)
		if d.Day == today.Format("2006-01-02") {
			result.UsedUnits = d.Units
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.RemainingUnits = max(0, result.Limits.DailyUnits-result.UsedUnits)
	return result, tx.Commit(ctx)
}

// Maintain performs bounded recovery/retention work. Re-running or concurrent runners are safe.
func (s *Store) Maintain(ctx context.Context) (int, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id FROM usage_attempts WHERE outcome='pending' AND created_at<clock_timestamp()-interval '30 seconds' ORDER BY created_at LIMIT 100`)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for i, id := range ids {
		if err := s.Finish(ctx, id, "unknown"); err != nil && !errors.Is(err, ErrNotFound) {
			return i, err
		}
	}
	_, err = s.Pool.Exec(ctx, `DELETE FROM usage_attempts WHERE id IN (SELECT id FROM usage_attempts WHERE outcome<>'pending' AND created_at<clock_timestamp()-interval '30 days' ORDER BY created_at LIMIT 1000)`)
	if err != nil {
		return len(ids), err
	}
	_, err = s.Pool.Exec(ctx, `DELETE FROM usage_days WHERE (project_id,day) IN (SELECT d.project_id,d.day FROM usage_days d WHERE day<(clock_timestamp() AT TIME ZONE 'UTC')::date-365 AND NOT EXISTS(SELECT 1 FROM usage_attempts a WHERE a.project_id=d.project_id AND a.day=d.day) ORDER BY day LIMIT 1000)`)
	return len(ids), err
}
