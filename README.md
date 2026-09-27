# Infra

Go and TypeScript foundation for a Nigeria-first EVM developer platform.
This repository is a development scaffold, not a production blockchain service.

## Quick start

Use Go 1.26.7 and Node 24 LTS (`nvm use`). Node 25 can run the development checks but CI uses 24.

```sh
npm ci
make api
# In a second terminal:
make web
```

Open http://localhost:5173. The dashboard uses `@infra/sdk` to call `/v1/status`
through Vite's local proxy. API: http://127.0.0.1:8080.
`make gateway` starts port 8081; `make worker` starts port 8082.
All processes support SIGINT/SIGTERM shutdown. `HTTP_ADDR` overrides their bind address.
Environment variables must be exported; `.env` is not automatically loaded.

```sh
make check       # formatting, vet, race tests, TS checks, SDK tests, web build, live process smoke tests
make build       # binaries in bin/, dashboard and SDK artifacts
make infra-up    # optional PostgreSQL and Redis; Docker required
make infra-down # preserves PostgreSQL volume
```

## Layout

- `cmd/{api,gateway,worker}`: process entry points.
- `internal/platform/server`: HTTP lifecycle, configuration, diagnostic routes.
- `apps/dashboard`: React/Vite application.
- `packages/sdk`: private TypeScript client, runtime validation and tests.
- `api/openapi.yaml`: public API contract.
- `docs`: engineering standards, architecture, release and operations policies.
- `.github`: verification workflow, dependency updates, PR template.

## Implemented versus planned

Implemented: three runnable Go processes, bounded HTTP timeouts, request IDs,
JSON lifecycle logs, graceful shutdown, health/readiness, status API, SDK and dashboard.
Gateway `/rpc` deliberately returns 503. Worker readiness deliberately returns 503
until a durable queue consumer exists. API/gateway readiness indicates scaffold HTTP
readiness only. `APP_ENV=production` fails startup.

Not implemented: authentication, tenants, contract deployment, upstream RPC, billing,
usage ledger, queue consumers, indexing, signing, metrics exporter, tracing or migrations.
PostgreSQL/Redis are optional development infrastructure and are not connected yet.
No throughput or production availability claim is made.

The Go module path `infra.local/platform` and private npm scope `@infra` are local
identifiers. Replace them after choosing a repository/publishing organization.
SDK publishing is disabled intentionally. No license is granted by this scaffold.

Read [contributing](CONTRIBUTING.md), [engineering](docs/engineering.md),
[architecture](docs/architecture.md), and [operations](docs/operations.md).
