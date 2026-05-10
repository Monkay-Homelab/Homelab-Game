# Branch Protection Rules for `main`

This document describes the recommended GitHub branch protection settings for the `main`
branch of the Homelab-Game repository. Since `main` is the production branch (code runs
directly on the homelab VM), these rules prevent accidental breakage from direct pushes
or unreviewed merges.

## How to Configure

1. Go to **Settings > Branches** in the GitHub repository
   (`https://github.com/Monkay-Homelab/Homelab-Game/settings/branches`).
2. Click **Add branch protection rule** (or edit the existing rule for `main`).
3. Set **Branch name pattern** to `main`.
4. Apply the settings described below.
5. Click **Save changes** at the bottom of the page.

## Recommended Settings

### Require a pull request before merging

**Enable this.** Select "Require a pull request before merging."

- **Required approvals**: Set to **0** (single-developer project; self-merge is expected).
  Increase to 1 when additional contributors join.
- **Dismiss stale pull request approvals when new commits are pushed**: Enable when approvals
  are required.
- **Require review from Code Owners**: Leave disabled until a `CODEOWNERS` file is added.

**Why**: Forces all changes to go through a PR, which triggers CI checks and creates an
auditable record. Even for a solo developer, this prevents accidental pushes of work-in-progress
code to production and ensures every change has a linked diff and description.

### Require status checks to pass before merging

**Enable this.** Select "Require status checks to pass before merging."

Add the following **required status checks** (these correspond to the job names in
`.github/workflows/build.yml`):

| Status Check Name | What It Validates |
|---|---|
| `backend` | Go compilation and `go test ./...` pass |
| `frontend` | pnpm install, TypeScript typecheck on `packages/shared/` |

- **Require branches to be up-to-date before merging**: **Enable**. This ensures the PR
  branch has been rebased onto the latest `main` before merging, so CI results reflect the
  actual code that will land on `main`. Without this, a PR could pass CI on stale code and
  break `main` after merge.

**Why**: These are the only two CI jobs today. They catch Go test failures, compilation
errors, and TypeScript type errors before code reaches production. As CI coverage expands
(linting, frontend build, security scanning), add those checks here too.

### Require conversation resolution before merging

**Leave disabled** for now. Enable when the team grows and PR review comments become common.

### Require signed commits

**Leave disabled** for now. Useful for supply-chain integrity but adds friction for a solo
project. Revisit if the project accepts external contributions.

### Require linear history

**Leave disabled.** Merge commits are acceptable for this project size. Enable if you want
to enforce rebase-only workflow.

### Require deployments to succeed before merging

**Leave disabled.** The project has no deployment environments configured in GitHub.

### Lock branch

**Leave disabled.** This makes the branch read-only, which is not appropriate for an active
development branch.

### Do not allow bypassing the above settings

**Enable this** even for administrators. The single biggest risk is a solo developer bypassing
protections "just this once" and pushing a broken change directly to production. If you need
to make an emergency fix, create a short-lived PR and merge it -- the CI pipeline is fast
enough that this adds minimal delay.

**Why**: Branch protection is only effective if it cannot be bypassed. Admin bypass undermines
the safety net. The build workflow completes in under 5 minutes, so the overhead of going
through a PR even for hotfixes is small.

### Rules applied to everyone including administrators

**Enable this** (same reasoning as above).

## Settings to Leave Disabled

| Setting | Why Disabled |
|---|---|
| Restrict who can push to matching branches | Unnecessary for a single-developer repo |
| Allow force pushes | Force pushes to `main` rewrite production history and can break deployments |
| Allow deletions | Deleting the production branch would be catastrophic |

## Future Additions

As CI coverage expands per the gaps identified in `_documents/spec/review-strategy.md`,
add these status checks when the corresponding workflow jobs are created:

- **Go linter** (`golangci-lint`) -- catches style drift and subtle bugs
- **Frontend build** (`pnpm build`) -- catches build failures before deploy
- **Frontend lint** (`eslint`) -- enforces code consistency
- **Desktop typecheck** (`pnpm typecheck` on `apps/desktop/`) -- currently only `packages/shared/` is checked
- **Security scanning** (`gosec`, Dependabot, or similar) -- catches vulnerable dependencies
