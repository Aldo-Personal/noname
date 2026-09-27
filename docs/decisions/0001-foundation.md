# ADR 0001: Go services and a TypeScript workspace

Status: accepted for the scaffold.

Context: small full-stack team building EVM developer infrastructure with a later Move
integration. Need independent workers, maintainability and measured scalability.

Decision: one repository and one Go module; API/gateway/worker entry points. React/Vite
dashboard and private TypeScript SDK managed with npm workspaces. Standard-library
HTTP server until a demonstrated need warrants more dependencies. PostgreSQL planned
for durable state; Redis available locally; durable broker selection deferred.

Alternatives: all TypeScript simplifies language count; Rust gives more low-level
control. Go balances concurrency and operational simplicity for the backend.

Consequences: two language toolchains; no service-to-service network boundaries yet.
Must load-test real workloads rather than infer capacity from language choice.
No Next.js dependency is needed for an authenticated dashboard with a separate API.
