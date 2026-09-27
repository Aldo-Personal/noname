# Architecture

Status: Ethereum project access implemented; RPC forwarding, billing and jobs are planned.

```text
React dashboard -> TypeScript SDK -> API
Customer SDKs --------------------> Gateway -> EVM providers (planned)
API -> PostgreSQL/outbox -> durable queue -> Workers (planned)
```

API owns the control plane. Gateway owns authenticated, metered provider access.
Workers own asynchronous delivery and reconciliation. Separate binaries allow
independent scaling while keeping one codebase. No broker is selected yet.

Implemented: OIDC sessions, personal organizations, Ethereum projects and API-key lifecycle.
Planned capabilities: team membership, contracts, transactions, events/webhooks,
usage, billing. Each owns its writes; no module may bypass another's invariants.
Usage is durable accounting input, not just a metrics counter. Redis is optional
ephemeral infrastructure, never the sole authority for balances or billing.

The authenticated project/API-key slice includes PostgreSQL tenant-isolation tests.
Then select the queue and implement outbox delivery before adding background side effects.
Add one explicitly supported EVM provider adapter with method restrictions and budgets.
Choose actual launch networks and provider commercial arrangements before forwarding.
Move support will have separate transaction/contract semantics and reuse control-plane
capabilities only. Avoid a universal chain abstraction before implementing the first adapter.

Current API contract is api/openapi.yaml. SDK status types are manually maintained and
tested; introduce schema generation/compatibility enforcement before publishing broader APIs.
