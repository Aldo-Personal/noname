# Contributing

Read docs/engineering.md. Install the toolchains in .go-version and .nvmrc, then `npm ci`.
Use `make fmt` for Go formatting and `make check` before requesting review.

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
