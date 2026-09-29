# Usage workload verification

Run from the repository with a dedicated TEST_DATABASE_URL:

```sh
go test -race -count=1 -v ./internal/usage -run TestGatewayLoad
```

The test creates and drops an isolated schema. Two HTTP gateway instances use independent
database pools against the same PostgreSQL database. Eight concurrent clients send 80
eth_blockNumber requests with a shared 50-unit daily allowance. A local provider fixture
adds 2 ms per read. The second scenario returns provider HTTP errors instead of results.
Both assert exactly 50 provider calls, 30 quota rejections and 50 durable units; rejected
requests never reach the provider. Results are checked against the daily outcomes.

Observed on the local macOS development machine with Docker PostgreSQL and Go's race
detector enabled (one bounded run, not a capacity benchmark):

| Provider scenario | Elapsed | All HTTP requests/sec | p50 | p95 | Admitted / rejected |
| --- | --- | --- | --- | --- | --- |
| Successful reads | 436 ms | 183.4 | 41.6 ms | 85.1 ms | 50 / 30 |
| Provider errors | 377 ms | 212.3 | 37.1 ms | 76.2 ms | 50 / 30 |

Throughput includes fast quota rejections. The quota is the deliberate saturation point;
these figures are not sustained upstream throughput or the system's maximum capacity.
The sample is small, runs entirely on one machine, and excludes real network/provider
latency, multiple tenants, long runs, TLS termination and production ingress. Instrumented
Go and shared development resources affect timings. No performance threshold is asserted
in CI; correctness invariants are asserted.

Other regression tests inject a failure at the last admission write and at aggregate
completion to prove atomic rollback; exercise duplicate persistence, lower/zero limits,
minute/day rollover, revoked keys, database outages, stale reservations and retention.
Gateway tests verify fail-closed accounting, bounded local capacity, client cancellation,
and completion-failure metrics. Tenant API tests cover unauthorized access and Origin.

Before production, run sustained many-project tests using actual service processes on
separate hosts, representative provider latency and payloads, and realistic retry traffic.
Measure PostgreSQL lock/pool contention, connection counts, CPU/memory, percentile latency,
reconciliation lag and recovery after process termination/database failover. Use those
results to set SLOs and decide whether quota admission needs a different architecture.
