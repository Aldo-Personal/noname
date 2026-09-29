# ADR 0004: PostgreSQL admission and durable usage

Status: accepted for development. Issue #10; follows ADR 0003.

PostgreSQL is the single authority for project limits, reservations and usage outcomes.
Do not add Redis until measured contention justifies another consistency boundary.
Lock one project's limits row during admission, then atomically reserve a daily unit,
increment the minute counter and insert a unique attempt. All replicas use this path.
Do not hold a database transaction across the provider request. Database failure or
ambiguous commit fails closed: no provider call unless admission commit succeeds.

Version 1 assigns one usage unit to each of the five allowed read methods. These are
admission units, not estimates of provider costs or money. Billable units are always zero.
Daily UTC budget defaults to 10,000; fixed UTC minute limit defaults to 60. Owners can
set 0 (disable) through 100,000 daily units and 600 requests/minute. These are development
safety ceilings, not purchased entitlements. Fixed windows permit bursts at boundaries.
Lowering limits never resets counters; raising a limit can restore capacity. PostgreSQL's
clock after acquiring the project lock defines windows; clients cannot set timestamps.

Generate a random internal attempt ID for each HTTP request, independent of JSON-RPC id.
Duplicate admission with the same attempt ID never increments counters or forwards again.
HTTP retries are new attempts and consume new units. No provider retry/failover is enabled.
Validate/authenticate before admission; recheck key expiry/revocation within admission.
Requests denied before admission consume no units and appear only in operational metrics.

Final outcomes: succeeded (valid provider result, regardless of downstream delivery),
upstream_error, timed_out, canceled (context canceled before a successful result), unknown
(process died or final persistence could not be confirmed). All keep their reserved unit.
Completion uses a fresh, bounded 2-second context even after client disconnection. It is
idempotent and updates the attempt and daily aggregate atomically. A successful provider
response is still returned if completion persistence fails: the durable pending reservation
remains, an operational error counter/log is emitted, and reconciliation marks it unknown.
Do not replay unknown requests or refund uncertain consumption automatically.

The bounded maintenance command marks pending attempts older than 30 seconds unknown.
Gateway operation deadline is 10 seconds, followed by a 2-second completion context
and at most 2 seconds of rollback cleanup if needed.
After a terminal outcome, later completion is a no-op; unknown cannot silently become
success. Daily aggregates remain reconstructable from retained attempts. Retain terminal
attempts 30 days and aggregates 365 days; delete only in bounded batches, never pending
attempts or aggregates still referenced by retained attempts. Dedupe is bounded by attempt
retention; internal IDs are not a public forever-idempotency contract. No historical
provider backfill is attempted. Before billing, define financial retention separately.

Tenant-scoped usage/limits APIs use the existing session authorization and Origin checks.
Metrics are per-process, contain no tenant IDs or secrets, and are not a billing ledger.
Per-project row contention, synchronous writes and the database pool constrain throughput;
benchmark two gateways with shared PostgreSQL and report measurements, not capacity promises.
Production remains blocked. Deploy migration before new binaries; rollback to a gateway
without accounting would bypass limits, so disable forwarding during such a rollback.
