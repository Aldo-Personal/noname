# Releases and compatibility

Backend starts at 0.1.0 in VERSION. SDK has an independent package version.
Public API uses /v1; webhook payloads and persisted messages need explicit schemas
when introduced. No silent breaking changes, even at 0.x once customers depend on them.
Use additive evolution and a documented deprecation/migration window agreed before launch.

Release process: merge passing PR; update VERSION and CHANGELOG; tag backend/vX.Y.Z
or sdk/vX.Y.Z as appropriate. Build once, record commit and artifact digest, test in
staging, promote the identical artifact. Never move a published tag or overwrite a
published package. SDK stays private until publishing ownership is configured.
Build backend with Docker ARG VERSION, or equivalent Go -ldflags injection.

Database migrations: add ordered immutable files once storage is introduced. Never
edit applied migrations. Expand schema, migrate/backfill, switch readers, then contract
in a later release. Test upgrades from prior schema and lock/runtime impact.
Do not auto-run migrations from every API replica. Application rollback requires
schema compatibility; destructive schema/chain changes generally need forward recovery.
Release automation/publishing credentials are intentionally not configured yet.
