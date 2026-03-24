---
name: release
description: >
  End-to-end release coordination skill that orchestrates the full release lifecycle:
  readiness assessment from all agents, changelog generation, version bumping, security
  sign-off, and go/no-go decision. Uses @release-manager as the primary coordinator with
  input from @staff-engineer, @security-engineer, @sdet, @devops-engineer, and @technical-writer.
  Trigger on phrases like "cut a release", "prepare release", "release v1.2", "ship it",
  "version bump", "are we ready to release", "go/no-go", or "prepare changelog".
argument-hint: "[version or scope]"
effort: high
maxTurns: 40
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Write", "Edit", "SendMessage", "Agent", "TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill", "AskUserQuestion"]
---

> **CRITICAL: Do NOT commit ANY changes (no `git add`, no `git commit`, no `git push`) unless EXPLICITLY instructed to do so by the user. This applies to ALL agents spawned by this skill.**

## Argument Handling

- **No argument** (`/release`): Determine scope from git history since last tag.
- **Version** (`/release v1.2.0` or `/release 1.2.0`): Use specified version.
- **Scope keyword** (`/release patch`, `/release minor`, `/release major`): Auto-determine
  version based on bump type from last tag.
- **"check"** (`/release check`): Run readiness assessment only, skip version bump and changelog.

---

# Release

You are the **Release Orchestrator** — you coordinate the full release lifecycle by spawning
agents to assess readiness from every perspective, then synthesize their input into a go/no-go
decision.

You do NOT make the final go/no-go call yourself — you present the assessment to the operator
and they decide.

---

## Pre-flight

1. **Goal alignment (HARD GATE)** — Use AskUserQuestion to confirm:
   - What type of release? (major, minor, patch, hotfix, pre-release)
   - What is the scope? (everything since last tag, specific features, specific branch)
   - Any special concerns? (breaking changes, data migrations, coordinated deployments)

2. **Determine last release** — Run:
   ```bash
   git tag --sort=-v:refname | head -5
   ```
   Set `{last-tag}` to the most recent tag. If no tags exist, set `{last-tag}` to the root
   commit (`git rev-list --max-parents=0 HEAD`) and note this is the first release.

3. **Gather release scope** — Run:
   ```bash
   git diff --stat {last-tag}..HEAD
   git log --format="%h %s (%an)" {last-tag}..HEAD
   ```

4. **Check issue tracker state** — Run `command -v docket && docket board --json` to check for
   blocking issues. If docket is not available, run `gh issue list --label blocker --state open`
   as a fallback, or skip if neither is available.

---

## Execution

### Step 1: Create Team

`TeamCreate(team_name="release-{version-slug}", description="Release coordination for {version}")`

Create tasks for each phase: Readiness Assessment, Changelog, Version Bump, Go/No-Go.

### Step 2: Readiness Assessment (parallel)

Spawn one agent per dimension. Each receives only the scope context — their agent definition
already contains domain-specific review criteria.

**Prompt template for all readiness reviewers:**
```
Agent(team_name="release-{version-slug}", name="{dimension}", subagent_type="{agent-type}", prompt="...")

Release readiness assessment for {version}. Use ultrathink for thorough analysis.
Scope: all changes since {last-tag} (run `git diff {last-tag}..HEAD`).
Report: READY / NOT READY / READY WITH CONCERNS + findings (blockers, concerns, suggestions).
```

**Reviewers:** @staff-engineer (code quality + architecture), @security-engineer (security),
@sdet (testing), @devops-engineer (deployment).

### Step 3: Changelog Generation

After readiness assessments complete, spawn @release-manager:

```
Agent(team_name="release-{version-slug}", name="release-coord", subagent_type="release-manager", prompt="...")

Generate the changelog and prepare release artifacts. Use ultrathink for thorough analysis.

Release version: {version}
Last release: {last-tag}
Changes since last release:
{git log output}

Readiness assessment summary:
- Code quality: {staff-engineer verdict}
- Security: {security-engineer verdict}
- Testing: {sdet verdict}
- Deployment: {devops-engineer verdict}

Requirements:
- Generate changelog entries following Keep a Changelog format
- Determine version bump if not specified (analyze changes for breaking/feature/fix)
- Identify all files that need version bumps
- Prepare the version bump edits (describe what to change, do NOT edit files yet)
- Produce the release readiness report per your agent instructions
- Present go/no-go recommendation with rationale
```

### Step 4: Documentation Check

Spawn @technical-writer to verify docs are release-ready:

```
Agent(team_name="release-{version-slug}", name="doc-check", subagent_type="technical-writer", prompt="...")

Verify documentation is ready for release {version}.

Changes since {last-tag}:
{git diff --stat output}

Check:
- README reflects current state
- API docs updated for any API changes
- Migration guide exists if there are breaking changes
- CHANGELOG is up to date (or will be updated as part of this release)
- No docs reference removed features or outdated behavior

Report: READY / NOT READY + specific gaps to address.
```

### Step 5: Go/No-Go Presentation

Synthesize all assessments into a unified report for the operator:

```
## Release Readiness: {version}

### Scope
- Commits: {count} since {last-tag}
- Files changed: {count}
- Contributors: {list}

### Readiness Matrix
| Dimension | Reviewer | Verdict | Key Findings |
|---|---|---|---|
| Code Quality | @staff-engineer | {verdict} | {one-line} |
| Security | @security-engineer | {verdict} | {one-line} |
| Testing | @sdet | {verdict} | {one-line} |
| Deployment | @devops-engineer | {verdict} | {one-line} |
| Documentation | @technical-writer | {verdict} | {one-line} |

### Blockers
{list or "None"}

### Concerns
{list or "None"}

### Changelog Preview
{changelog content from @release-manager}

### Version: {version}
Files to update: {list from @release-manager}

### Recommendation: {GO / NO-GO / GO WITH CONDITIONS}
{rationale}
```

Use AskUserQuestion to present the report and ask:
- **"Go"** — Proceed to apply version bumps and finalize changelog.
- **"Go with conditions"** — Proceed but note accepted risks.
- **"No-go"** — Abort. List what needs to be addressed.
- **"Revise"** — Address specific concerns and re-assess.

### Step 6: Apply Release Artifacts (if Go)

If the operator approves:

1. Apply version bumps to identified files using Edit tool.
2. Write/update CHANGELOG file with the generated entries.
3. Remind the user: "Changes are ready but NOT committed. Review with `git diff`, then
   commit and tag when satisfied."

### Step 7: Cleanup

Shut down all teammates and `TeamDelete`.

---

## Hotfix Mode

When the argument contains "hotfix" or the operator specifies hotfix:

1. **Expedited assessment** — Spawn only @staff-engineer and @security-engineer (parallel).
   Skip @technical-writer.
2. **Always patch version** — No major/minor bumps for hotfixes.
3. **Minimal changelog** — One entry describing the fix.
4. **Fast path** — Skip plan presentation for trivial hotfixes (single commit, single file).

---

## Rules

1. **Create the team before spawning.** `TeamCreate` → `TaskCreate` → `Agent`.
2. **Spawn readiness reviewers in parallel** for speed.
3. **Never skip security review.** Even for hotfixes.
4. **Operator decides go/no-go.** You present, they decide.
5. **Never commit or tag.** Prepare artifacts, user commits.
6. **Clean up.** Shutdown teammates and `TeamDelete` after completion.
