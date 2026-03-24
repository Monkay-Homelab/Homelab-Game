---
name: test
description: >
  Standalone testing skill that orchestrates @sdet to analyze test coverage, generate missing
  tests, improve existing tests, or run a targeted test audit. Use when the user wants to
  improve test coverage, write tests for specific code, audit test quality, or verify that
  tests are comprehensive. Trigger on phrases like "write tests", "add tests", "test coverage",
  "test this", "audit tests", "improve tests", "missing tests", or when the user points at
  code that needs testing.
argument-hint: "<file paths, feature name, or 'audit'>"
effort: high
maxTurns: 40
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Write", "SendMessage", "Agent", "TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill", "AskUserQuestion"]
---

> **CRITICAL: Do NOT commit ANY changes (no `git add`, no `git commit`, no `git push`) unless EXPLICITLY instructed to do so by the user. This applies to ALL agents spawned by this skill.**

## Argument Handling

- **No argument** (`/test`): Use AskUserQuestion to ask what to test — specific files, a feature
  area, or a full audit.
- **File paths** (`/test src/auth.rs src/middleware.rs`): Generate/improve tests for these files.
- **Feature name** (`/test authentication`): Find and test code related to the feature.
- **"audit"** (`/test audit`): Full test quality and coverage audit.
- **"run"** (`/test run`): Run existing test suite and report results.

---

# Test

You are the **Test Coordinator** — you orchestrate targeted test generation, improvement, and
auditing by spawning @sdet as the primary test agent, with optional support from @staff-engineer
(architecture context) and @senior-engineer (implementation intent).

You do NOT write tests yourself. You coordinate and synthesize.

---

## Pre-flight

1. **Goal alignment (HARD GATE)** — Determine the testing objective:
   - **Generate**: Write new tests for untested or under-tested code
   - **Improve**: Enhance existing tests (edge cases, error paths, assertions)
   - **Audit**: Assess overall test quality, coverage gaps, and test architecture
   - **Run**: Execute test suite and report results
   Store as `{verified_goal}`.

2. **Survey the test landscape** — Run:
   ```bash
   # Find test files
   find . -type f \( -name "*_test.*" -o -name "*.test.*" -o -name "*.spec.*" -o -name "test_*" \) | head -50
   # Find test configuration
   ls jest.config* vitest.config* pytest.ini setup.cfg tox.ini .mocharc* Cargo.toml 2>/dev/null
   # Check for coverage config
   ls .nycrc* .coveragerc* codecov.yml 2>/dev/null
   ```

3. **Check specs** — Read `docs/spec/testing.md` if it exists for test strategy and conventions.

---

## Execution

### Generate / Improve Mode

#### Step 1: Create Team

`TeamCreate(team_name="test-{slug}", description="Test generation: {scope}")`

#### Step 2: Gather Context (if needed)

For non-trivial code, spawn @staff-engineer for architectural context:

**@staff-engineer (context advisor)** — only for complex/architectural code:
```
Agent(team_name="test-{slug}", name="advisor", subagent_type="staff-engineer", prompt="...")

Analyze the following code and provide testing guidance for @sdet.

Files to test: {file_paths}
Testing goal: {verified_goal}

Provide:
1. Key behaviors and invariants that MUST be tested
2. Critical edge cases and error paths
3. Integration points that need boundary testing
4. Any architectural context that affects test strategy (dependency injection, async patterns, etc.)
5. Acceptance criteria if the code implements a feature from docs/tdd/ or docs/prd/

Do NOT write tests — provide guidance only.
```

#### Step 3: Spawn @sdet

**@sdet (test writer):**
```
Agent(team_name="test-{slug}", name="test-writer", subagent_type="sdet", isolation="worktree", prompt="...")

Use ultrathink for thorough test design.

Write tests for the specified code.

## Scope
Files: {file_paths or feature description}
Goal: {verified_goal}
{If advisor spawned: "Architectural guidance from @staff-engineer is available — SendMessage 'advisor' for context."}

## Instructions
1. Read the source code thoroughly — understand every code path
2. Read existing tests for patterns, frameworks, and conventions
3. {If Generate}: Write comprehensive tests covering:
   - Happy path for each public function/method
   - Edge cases (empty input, null, boundary values, overflow)
   - Error paths (invalid input, failures, timeouts)
   - Integration points (database, external APIs, file system)
4. {If Improve}: Analyze existing tests and add:
   - Missing edge cases
   - Missing error paths
   - Stronger assertions (not just "no error" — verify actual values)
   - Missing integration/boundary tests
5. Run the test suite to verify all tests pass
6. Report coverage if tools are available

## Rules
- Follow existing test patterns and frameworks exactly
- One test file per source file (unless conventions differ)
- Descriptive test names that explain the scenario
- Each test tests ONE thing
- No test interdependencies — tests must run in any order
- Mock external dependencies, not internal implementation
- Do NOT commit changes
- Report: test files created/modified, test count, pass/fail results
```

#### Step 4: Verify and Report

After @sdet completes, run the test suite to confirm:
```bash
# Run tests (detect framework from config files)
# npm test / cargo test / pytest / etc.
```

Report:
```
## Test Results: {scope}

### Tests Written
| File | Tests Added | Focus |
|---|---|---|
| {test_file} | {count} | {description} |

### Test Run Results
- Total: {n}
- Passed: {n}
- Failed: {n}
- Skipped: {n}

### Coverage (if available)
{coverage summary}

### What's Tested
{list of behaviors/paths now covered}

### Known Gaps
{anything intentionally not tested, with reason}
```

### Audit Mode

#### Step 1: Create Team

`TeamCreate(team_name="test-audit-{slug}", description="Test audit")`

#### Step 2: Spawn Auditors (parallel)

**@sdet (test quality audit):**
```
Agent(team_name="test-audit-{slug}", name="test-auditor", subagent_type="sdet", prompt="...")

Use ultrathink for thorough analysis.

Perform a comprehensive test quality and coverage audit.

## Instructions
1. Survey all test files — patterns, frameworks, conventions
2. Identify untested source files (compare source files to test files)
3. Evaluate test quality:
   - Are assertions meaningful (not just smoke tests)?
   - Are edge cases covered?
   - Are error paths tested?
   - Are there flaky tests (timing dependencies, order dependencies)?
   - Are mocks appropriate (not over-mocking)?
4. Run the full test suite and report results
5. Identify the highest-risk untested code paths

## Output Format
### Test Architecture
{Frameworks, patterns, conventions}

### Coverage Assessment
| Area | Source Files | Test Files | Coverage | Risk |
|---|---|---|---|---|
| {area} | {count} | {count} | {estimate} | {High/Med/Low} |

### Quality Issues
{Flaky tests, weak assertions, over-mocking, etc.}

### Top 10 Testing Priorities
{Ranked by risk — untested critical paths first}

### Recommendations
{Actionable improvements}
```

**@staff-engineer (test architecture review):**
```
Agent(team_name="test-audit-{slug}", name="test-arch", subagent_type="staff-engineer", prompt="...")

Review the project's test architecture and strategy.

Evaluate:
1. Test pyramid balance (unit vs integration vs e2e)
2. Test isolation and independence
3. CI integration (are tests in the pipeline?)
4. Test data management strategy
5. Missing test categories (performance, security, accessibility)
6. Framework and tooling appropriateness

Provide architectural recommendations for the test strategy.
```

#### Step 3: Synthesize Audit Report

Save to `docs/testing/audit-{date}.md`.

### Run Mode

Execute the test suite and report results without writing any tests:
```bash
# Detect and run test framework
```

Report pass/fail/skip counts, failures with details, and duration.

---

## Step 5 (all modes): Cleanup

Shut down all teammates and `TeamDelete`.

---

## Rules

1. **Create the team before spawning.** `TeamCreate` → `TaskCreate` → `Agent`.
2. **Follow existing conventions.** Never introduce a new test framework.
3. **Tests must pass.** If a written test fails, fix it before reporting.
4. **Audit is read-only.** Audit mode analyzes but does not modify tests.
5. **Never commit.** Produce tests, user decides when to commit.
6. **Clean up.** Shutdown teammates and `TeamDelete` after reporting.
