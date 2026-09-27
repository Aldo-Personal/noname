# Contributing

Read docs/engineering.md. Install the toolchains in .go-version and .nvmrc, then `npm ci`.
Use `make fmt` for Go formatting and `make check` before requesting review.

Never code or commit directly on main. Main changes only through reviewed pull-request merges.
Use dev or short-lived feature branches; agent branches use codex/.
Commit each important, coherent change after its relevant checks pass.
Reference the tracking GitHub issue in commits and PRs once it exists.
Use short-lived branches from main. Agent branches use `codex/`; human branches may
use `feat/`, `fix/`, or `docs/`. Keep each PR focused and reviewable.
Use squash merges and Conventional Commit PR titles, such as
`fix(webhooks): prevent duplicate billing on retry`. Allowed types: feat, fix,
refactor, docs, test, chore, ci, perf, build. Mark breaking changes explicitly.

Main must have required CI (`verify`), no force pushes, and independent review when
another maintainer is available. Hosting-side settings are not configured by these files.
Owners must configure branch protections, secret scanning and real CODEOWNERS after
the remote/team exists. Do not add fictional owners.

Definition of done: behavior implemented, relevant tests pass, compatibility checked,
docs updated, operational impact and recovery understood. Explain exceptions in the PR.
No blanket coverage target; prioritize billing, authorization, persistence and failure paths.

Install repository hooks with `git config core.hooksPath .githooks`. Hooks reject
commits and pushes to main. These are local safeguards, not a replacement for GitHub
branch protection. Create a tracking issue before implementation when GitHub access
is available; otherwise save its exact draft in docs/issues and report the blocker.
