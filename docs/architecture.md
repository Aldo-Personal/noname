# Architecture

Status: Ethereum project access and optional read-only RPC implemented; billing and jobs are planned.

```text
React dashboard -> TypeScript SDK -> API
Customer SDKs --------------------> Gateway -> configured Ethereum provider
API -> PostgreSQL/outbox -> durable queue -> Workers (planned)
```

API owns the control plane. Gateway owns authenticated provider access; durable admission accounting and limits use PostgreSQL (#10).
Workers own asynchronous delivery and reconciliation. Separate binaries allow
independent scaling while keeping one codebase. No broker is selected yet.

Implemented: OIDC sessions, personal organizations, Ethereum projects, API-key lifecycle
and bounded five-method read-only RPC with durable reservations and shared quotas. See decisions/0003-ethereum-rpc.md and ethereum-rpc.md.
Planned capabilities: team membership, contracts, transactions, events/webhooks,
billing. Each owns its writes; no module may bypass another's invariants.
Usage is durable accounting input, not just a metrics counter. Redis is optional
ephemeral infrastructure, never the sole authority for balances or billing.

The authenticated project/API-key slice includes PostgreSQL tenant-isolation tests.
Then select the queue and implement outbox delivery before adding background side effects.
The first provider-neutral HTTP adapter requires an operator-configured Ethereum mainnet
endpoint. Choose provider commercial arrangements before using it for customers.
Move support will have separate transaction/contract semantics and reuse control-plane
capabilities only. Avoid a universal chain abstraction before implementing the first adapter.

Current API contract is api/openapi.yaml. SDK status types are manually maintained and
tested; introduce schema generation/compatibility enforcement before publishing broader APIs.
