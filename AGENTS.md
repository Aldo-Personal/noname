# Coding agent instructions

Read CONTRIBUTING.md and docs/engineering.md before modifying code. Read relevant
architecture decisions under docs/decisions. User instructions take precedence.

- Keep changes scoped and preserve unrelated work.
- Never claim unimplemented features or unmeasured performance.
- Do not remove the production startup guard until operations prerequisites are met.
- Never introduce private-key custody, fake billing, or an open unauthenticated RPC proxy.
- Do not modify applied migrations or published API contracts incompatibly.
- Run relevant checks; normally `make check`. Report failures and unrun checks honestly.
- Update documentation and tests with behavior changes.
- Never commit secrets, logs containing credentials, dependency directories or build output.
- Do not publish packages, deploy, or push changes unless authorized.
