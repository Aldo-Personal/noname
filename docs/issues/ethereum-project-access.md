# Add Ethereum project access and API-key lifecycle

## Outcome
A developer can sign in, create an Ethereum mainnet project, generate a project API
key, authenticate a request, rotate/revoke the key, and see revocation take effect.

## Implementation
- PostgreSQL schema and explicit versioned migrations.
- Maintained OIDC verification and local Keycloak development sign-in.
- Database-backed opaque browser sessions and server-side tenant ownership.
- Project create/list and API-key create/list/revoke/atomic rotation.
- Raw API keys displayed once; only hashes stored; no browser token persistence.
- TypeScript SDK and dashboard workflow.
- Ethereum mainnet chain ID 1; no RPC forwarding or signing in this milestone.

## Acceptance
- Cross-tenant reads and writes are rejected.
- Invalid/expired/revoked keys are rejected; rotation invalidates the old key.
- Sign-in verifies issuer, audience, signature, expiry, state, nonce and PKCE.
- Mutations require the configured browser origin.
- Sessions/projects/keys survive application restart.
- Integration tests use PostgreSQL; CI exercises migrations and access tests.
- Existing checks remain green; important changes are separately committed off main.

## Out of scope
Paid billing, blockchain submission, embedded wallets, durable jobs and public deployment.

## Tracking
GitHub: https://github.com/Aldo-Personal/noname/issues/7
Branch: codex/ethereum-project-access.
