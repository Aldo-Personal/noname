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

- Never develop or commit on main; use dev or codex/* feature branches.
- Commit important verified changes separately with descriptive messages.
- Track work in GitHub issues and link commits/PRs when access is available.
- Main receives changes only by PR merge; do not push main directly.
