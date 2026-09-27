# ADR 0002: Ethereum access foundation

Status: accepted for development. Tracking issue: see docs/issues/ethereum-project-access.md.

Use Ethereum mainnet (chain ID 1) for projects. No upstream RPC forwarding or signing
is enabled. Keys currently authorize only GET /v1/key-check; they cannot manage accounts.

Use pgx's PostgreSQL pool (10 connections per API replica initially). Apply embedded,
checksum-verified SQL migrations explicitly with cmd/migrate, under a database advisory
lock. This small runner is adequate for the first transactional migration; revisit it
before online backfills or non-transactional DDL.

Use OpenID Connect authorization code + PKCE through coreos/go-oidc and x/oauth2.
Keycloak is a local-only identity provider; production provider selection remains open.
Identity is issuer+subject, never email. On first login create one personal organization;
team membership/invitations are deferred. Store opaque sessions by SHA-256 hash, expire
after 24h, and use HttpOnly SameSite=Lax cookies. HTTPS origins use Secure cookies.
Mutating session routes require the configured Origin; no wildcard CORS.

API keys contain 256 random bits, are hashed with SHA-256, expire after 90 days, and
are returned only on issue/rotation. Slow password hashing is unnecessary for these
high-entropy generated tokens. Never allow user-selected keys. Prefixes identify keys
without exposing the full credential. Rotation locks the old row, revokes it, and
creates the replacement in the same transaction. No authorization cache: subsequent
requests see committed revocation. Previously authorized in-flight work is not cancelled.

List endpoints return the newest 100 records; pagination is a prerequisite to larger
accounts. Local sign-in has no production abuse/rate-limit perimeter yet. Production
startup guard remains. Sessions/expired login flows need scheduled retention cleanup
before public rollout. No product billing or durable jobs are introduced here.
