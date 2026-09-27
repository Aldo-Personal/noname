# Operations and production gate

## Development

GET /healthz is process liveness. GET /readyz is scaffold HTTP readiness; worker returns
503 because it has no durable consumer. Gateway POST /rpc returns 503. No credentials
or provider URLs are accepted yet. APP_ENV=production is deliberately rejected.
Docker Compose is local only, with loopback ports and disposable development credentials.
`docker compose down` preserves PostgreSQL; deleting its volume destroys local data.

## Before enabling production

- Implement tenant isolation, authentication, API-key rotation, quotas and audit events.
- Replace scaffold readiness with dependency/consumer checks appropriate to each process.
- Select and test durable queue/outbox, deduplication, crash recovery and dead letters.
- Integrate durable metering and reconcile usage; define unavailable-dependency behavior.
- Configure secret storage, TLS/ingress, least-privilege network/database access.
- Add metrics/traces: latency, errors, saturation, upstream health, queue age and billing lag.
- Set numeric SLOs and recovery objectives based on measured workloads and budget.
- Exercise restore from backup and compatible rollout/rollback in staging.
- Run load tests with representative methods, payloads, concurrency and upstream failures.
- Configure CI secret scanning, Go vulnerability scanning and immutable action/image pins.
- Review payment-provider eligibility before enabling real billing.
- Set real on-call/security contacts and branch protections in the hosting platform.

Do not remove the production guard merely to make a deployment pass.

## Incident procedure

Identify impacted operation and tenants. Pause affected side effects if duplicate
billing/submission is possible. Preserve request/job IDs without collecting secrets.
Check dependencies and queue age. Restore a known compatible image or use documented
forward repair. Reconcile unknown transaction outcomes before replay. Record customer
impact, recovery actions and a follow-up regression test. Never promise zero data loss
without validated recovery evidence.
