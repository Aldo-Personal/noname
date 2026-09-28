# Contributing

Read docs/engineering.md. Install the toolchains in .go-version and .nvmrc, then `npm ci`.
Use `make fmt` for Go formatting and `make check` before requesting review.

Never code or commit directly on main. Main changes only through reviewed pull-request merges.
Use dev or short-lived feature branches; agent branches use codex/.
Commit each important, coherent change after its relevant checks pass.
Reference the tracking GitHub issue in commits and PRs once it exists.
Use short-lived branches from main. Agent branches use `codex/`; human branches may
use `feat/`, `fix/`, or `docs/`. Keep each PR focused and reviewable.
Prefer rebase merges to preserve meaningful verified commits. Squash only when explicitly
chosen to clean up disposable history. Use Conventional Commit PR titles, such as
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

GitHub main protection was configured for issue #7: PR required, verify check required
with an up-to-date branch, conversations resolved, linear history, administrator
enforcement, and no force pushes/deletion. Required independent approval count is zero
for the current solo-maintainer workflow; raise it when another reviewer joins.

## Commit and issue granularity

Create issues for independently reviewable outcomes, not individual files or tiny fixes.
Keep a rolling backlog of roughly five milestones in docs/roadmap.md. A milestone
normally has one PR and several coherent commits. Commit after a meaningful tested
checkpoint; do not wait until the entire milestone is finished or commit every keystroke.
Group a behavior change with its tests and necessary compatibility changes. Avoid mixing
unrelated refactors with features. Every checkpoint should build and pass relevant tests.
Document dependency order: a commit is not automatically safe to revert alone merely
because it is small. Revert dependent commits together/in reverse order; migrations
and onchain side effects may need forward repair. Use additive API/schema changes.

Start from current origin/main after a PR is merged. When stacking on an unmerged PR,
name that dependency and retarget/rebase after it lands, without force-pushing another
person's work. Never merge to main as a side effect of completing an implementation.
