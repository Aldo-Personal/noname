# Infra

Nigeria-first developer infrastructure, starting with Ethereum mainnet (chain ID 1).
Development milestone: OIDC sign-in, personal organizations, projects, and API keys.
RPC forwarding, transactions, wallets and billing are not enabled. Production startup
remains blocked until the safeguards in docs/operations.md are implemented.

## Local setup

Requires Go 1.26.7, Node 24 LTS (`nvm use`), npm and Docker Desktop.

```sh
npm ci
git config core.hooksPath .githooks
docker compose up -d --wait postgres
docker compose up -d keycloak
cp .env.example .env
set -a
source .env
set +a
make migrate
make api
```

Keycloak can take a minute on first boot. It is ready when
http://localhost:8090/realms/infra-dev/.well-known/openid-configuration responds.
If API startup reports OIDC discovery failure, wait for Keycloak startup and retry.
In another terminal, from this directory:

```sh
make web
```

Open **http://localhost:5173** (use localhost, matching the configured OIDC origin).
Sign in as `developer` with password `local-development-only`. These are disposable
local fixture credentials, not production credentials. New local users can also register.
The first login creates a personal organization. Create a project, generate a key,
copy it once, and use Test key. The console supports atomic rotation and revocation.
Do not put server API keys into browser applications or source control.

Local Keycloak admin: http://localhost:8090/admin, user `local-admin`, password
`local-admin-development-only`. Realm changes are imported only when the realm does
not exist; editing dev/keycloak/realm.json does not overwrite an existing realm.

## Ports and existing services

Defaults: API 8080, gateway 8081, worker 8082, dashboard 5173, Keycloak 8090.
Do not stop an unrelated process occupying a port. A second local callback is allowed
for this verification setup:

```sh
# With .env exported:
HTTP_ADDR=127.0.0.1:8091 PUBLIC_ORIGIN=http://localhost:5174 make api
# Another terminal:
INFRA_API_URL=http://127.0.0.1:8091 npm run dev -w @infra/dashboard -- --port 5174
```

Use http://localhost:5174 for that setup. PUBLIC_ORIGIN must match the browser origin.
The Vite proxy is development-only; production requires explicit routing and HTTPS.
Go does not auto-load .env. Session keys and raw API keys are never stored in localStorage.
Keycloak sign-out is separate from application sign-out: the app revokes its own session;
an existing identity-provider session can still permit a subsequent sign-in.

## Commands and checks

```sh
make check       # formats, vet, Go race tests, SDK tests, TS builds, process smoke tests
make build       # API/gateway/worker binaries and JS build output
make migrate     # explicitly apply checksum-verified, transactional migrations
npm run format   # format TypeScript, CSS and JS tests/scripts
make fmt         # format Go
# Run database integration tests; each test creates/drops an isolated schema:
TEST_DATABASE_URL='postgres://infra:local-development-only@127.0.0.1:5432/infra?sslmode=disable' make integration
```

Without TEST_DATABASE_URL, PostgreSQL integration tests explicitly skip. CI supplies
a dedicated PostgreSQL service and runs these tests. Use a dedicated local/test database
with permission to create schemas, never a production database. Smoke tests clear access
configuration and exercise the minimal HTTP processes. Integration tests cover signed
OIDC identities, PKCE, replay, CSRF, tenant boundaries, sessions, key expiry and rotation.

## Layout and entry points

- `cmd/api`, `cmd/gateway`, `cmd/worker`: Go service entry points.
- `cmd/migrate`: explicit migration entry point.
- `internal/access`: identity, organization/project/key persistence and HTTP handlers.
- `internal/platform/database`: PostgreSQL pool and embedded versioned migrations.
- `internal/platform/server`: configuration, lifecycle, routing, readiness and shutdown.
- `apps/dashboard`: React/Vite console; `src/main.tsx` is the browser entry point.
- `packages/sdk`: TypeScript client with Zod runtime response validation.
- `api/openapi.yaml`: API contract, including browser-session and bearer-key routes.
- `dev/keycloak`: local OIDC fixture; `docs`: decisions, policies, and operations.

The dashboard depends on the compiled SDK. Rebuild it after SDK edits:
`npm run build -w @infra/sdk`. Status remains `/v1/status`. Access routes require the
database and OIDC configuration; without it they return 503. With access enabled,
readiness checks PostgreSQL. Gateway `/rpc` and worker readiness still return 503.

Keys currently authorize only `GET /v1/key-check`, using `Authorization: Bearer <key>`.
They expire after 90 days. Session routes require an HttpOnly cookie; writes also require
the configured Origin. Raw keys appear only on creation/rotation. List endpoints show
the newest 100 records. Team invitations, pagination, production abuse controls and
retention jobs remain future work.

## Workflow

Never commit or push directly to main. Use dev or codex/* branches, create tracking
issues, and commit coherent verified milestones. Local hooks enforce the main restriction;
GitHub main protection requires a PR, up-to-date passing verify CI, resolved conversations,
and linear history; administrators are included. Force pushes/deletion are disabled.
Independent approvals are currently optional for the solo-maintainer workflow. Current milestone:
https://github.com/Aldo-Personal/noname/issues/7.

The local Go module path and private npm scope are placeholders for future publishing.
No package publication or production deployment is configured. See CONTRIBUTING.md,
docs/engineering.md, docs/releases.md and docs/operations.md.
