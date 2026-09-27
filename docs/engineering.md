# Engineering handbook

## Boundaries

Keep one Go module. Modules own business rules and database writes. HTTP handlers
translate requests; business logic belongs outside transport handlers. Cross-module
operations use explicit APIs. Add interfaces at real substitution boundaries, not
for every struct. Prefer small named packages over generic utils and premature frameworks.
See architecture.md for planned modules; do not create empty layers before they are used.

## Code

Go: gofmt, explicit errors, context propagation, external-call deadlines, bounded
concurrency, owned goroutine lifetimes, graceful shutdown, explicit dependencies.
No mutable global business state or panic for normal failures. Runtime configuration
must validate at startup. The release-version variable is build metadata only.
TypeScript: strict mode, unknown at external boundaries, runtime validation, no
unexplained any/ts-ignore. Browser code must never receive backend credentials.
Use integer minor units or appropriate decimals for money and exact integers for
chain amounts. No floats for financial calculations. SQL must be parameterized.
Enforce tenant ownership at every relevant operation; add negative authorization tests.

## Reliability invariants

Jobs are at-least-once: deduplicate durable side effects with database constraints and
idempotency records. Goroutines are not durable jobs. Use an outbox when committing
business state and scheduling work must succeed together. Bound retries with jitter,
dead-letter handling, expiry and replay controls. Queue selection remains an ADR before
implementing consumers; do not add multiple brokers speculatively.
An RPC timeout is an unknown transaction outcome: reconcile before resubmitting.
Separate submission, inclusion and finality. Handle reorganizations in event processing.
Never charge twice for redelivery. Usage and adjustments must be reconstructable.
Webhook delivery needs signing, SSRF defenses, bounded response sizes and timeouts.
Signing/custody needs a separate security design; never store raw private keys in tables.

## Dependencies and verification

Commit lockfiles. Review dependency purpose, maintenance and license. Pin Go/toolchain
and release artifacts; lock JS transitive versions with npm ci. Weekly dependency PRs
must pass CI. CI actions should be pinned to reviewed commit SHAs before production.
Run go vet, race tests, TypeScript checks, SDK tests, builds and vulnerability checks.
Add integration tests against real PostgreSQL/queue when those adapters are introduced.
Use contract tests for published interfaces and fuzz parsers handling untrusted payloads.
Add failure tests before enabling billing, retries or multi-tenant operations.
Do not log tokens, signing material or customer request bodies by default.

## Changes

Record significant decisions with context, alternatives and consequences in docs/decisions.
Policies are authoritative here; AGENTS.md and CONTRIBUTING.md link to them.
No production deployment until the checklist in operations.md is satisfied.
