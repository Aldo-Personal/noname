# Project usage and limits

Issue #10 adds admission accounting to every configured gateway. The same PostgreSQL
database must be used by all replicas. Redis is not required. See ADR 0004 for the
accounting model and its tradeoffs.

## Operator setup

1. Stop or disable forwarding on older gateways before rolling out enforcement; old
   binaries can bypass the new limits.
2. Export DATABASE_URL and run `make migrate` to apply additive migration 0002.
3. Restart the API and gateways with the new binaries. No OIDC changes are required.
4. Run `go run ./cmd/usage-maintenance` with DATABASE_URL and APP_ENV=development.
   Schedule the compiled command at least once per minute before operating a sustained
   workload. No scheduler is installed by this change. Each pass handles at most 100
   stale reservations, 1,000 expired attempts and 1,000 expired aggregates within a
   30-second deadline. Repeat passes to drain a backlog; nonzero exit means retry the
   maintenance pass, never the provider call. Alert if oldest pending age grows.

Without a maintenance scheduler, quota enforcement still works and reservations stay
durable, but stale outcomes and expired history accumulate. This is an explicit operator
prerequisite, not an implicit goroutine pretending to be a durable worker. The existing
worker process remains a placeholder for later event/webhook delivery.

## Meaning of usage

Each valid admitted read consumes one unit (cost version 1) and one minute slot before
the provider is contacted. Default limits: 10,000 units per UTC day and 60 requests per
fixed UTC minute. Owner controls range from zero to 100,000/day and 600/minute; zero in
either field pauses RPC. These are development ceilings, not pricing or plan entitlements.
UTC midnight is 01:00 in Lagos. Fixed windows can allow a burst across their boundaries.

Auth/validation failures, local saturation and quota rejection consume no units.
Once admitted, errors, timeouts and disconnected clients retain their unit. Success means
the gateway obtained a valid provider result, not proof the caller received it. A process
crash or unconfirmed final write leaves pending usage; maintenance marks it unknown after
30 seconds. Unknown does not trigger automatic replay or refund. No money is charged:
billable_units is zero, and these records are not a financial ledger.

Internal random attempt IDs deduplicate admission/completion persistence. They are unrelated
to a caller's JSON-RPC ID. Repeating an HTTP call is a new attempt and consumes another
unit. There are no transparent upstream retries. Retain terminal attempts for 30 days and
daily aggregates for 365 days, in bounded maintenance batches. Pending records never expire
silently. Limits updates do not reset already consumed usage.

## Contracts

`GET /v1/projects/{project}/usage` requires the owner's browser session and returns:

```json
{
  "limits": {"dailyUnits":10000,"minuteRequests":60},
  "usedUnits":7,"remainingUnits":9993,"resetsAt":"2026-10-01T00:00:00Z",
  "days":[{"day":"2026-09-30","units":7,"pending":0,"succeeded":5,
    "upstreamError":1,"timedOut":0,"canceled":0,"unknown":1}]
}
```

Days includes up to seven UTC dates with recorded usage, newest first. The snapshot is
transactionally consistent. `PUT /v1/projects/{project}/limits` requires the same session,
the configured Origin, and both integer fields dailyUnits/minuteRequests. It returns the
saved limits. Unknown projects and other tenants' projects return 404; invalid input 400.
API keys cannot manage limits. OpenAPI: api/openapi.yaml.

SDK: `client.usage(projectId, signal?)` and
`client.setLimits(projectId, {dailyUnits: 5000, minuteRequests: 30})` on InfraClient.
The console shows today's consumption, remaining units, reset time, seven-day outcomes
and an editable limits form. Refresh explicitly to fetch current usage; it is not live streaming.

Gateway quota rejection: HTTP 429, `{ "code": "project_limit_exceeded" }`, and Retry-After
seconds until the relevant window resets. Accounting unavailability: HTTP 503 with
code accounting_unavailable; no provider call occurs. Admitted responses include
X-Usage-Attempt-ID for support correlation. The server SDK exposes 429/503 via EthereumError.
Provider response semantics otherwise match docs/ethereum-rpc.md. Keep API keys out of
query strings, browser bundles and logs.

## Operations and recovery

Configured gateway `GET /metrics` exposes Prometheus text: RPC status-class counts,
duration sum/count, current in-flight requests, accounting admission failures and completion
write failures. These are process-local and reset on restart. No tenant, key, method input,
URL or request body appears in labels. Restrict /metrics at production ingress.
Use latency percentiles from load tests; the runtime summary exposes averages, not quantiles.

If PostgreSQL is unavailable, forwarding fails closed. Completion errors emit a sanitized
log with only the internal attempt ID and increment infra_usage_completion_errors_total.
Restore database access, run maintenance, and inspect pending/unknown usage. Do not reset
budgets or replay calls as outage recovery. No Redis failure mode exists in this design.

Diagnostic SQL (operator only):

```sql
SELECT count(*) AS pending, min(created_at) AS oldest_pending
FROM usage_attempts WHERE outcome='pending';
SELECT day, sum(units) AS admitted_units, sum(unknown) AS unknown
FROM usage_days GROUP BY day ORDER BY day DESC LIMIT 7;
```

Rollback: disable forwarding first, keep migration 0002, then roll back binaries if needed.
Never drop the usage tables or revert to an unmetered gateway while forwarding stays on.
Future schema changes require new migrations. Keep production startup blocked until the
operations gate, realistic load tests and provider commercial terms are satisfied.
