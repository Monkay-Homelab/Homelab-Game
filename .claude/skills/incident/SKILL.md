---
name: incident
description: >
  Live incident response skill that coordinates real-time investigation and remediation of
  production issues. Orchestrates @sre (diagnostics + observability), @senior-engineer
  (hotfix implementation), @devops-engineer (infrastructure remediation), @security-engineer
  (if security-related), and @staff-engineer (architectural guidance). Focuses on rapid
  resolution — full analysis comes later via /postmortem. Use when the user is dealing with
  an active production issue. Trigger on phrases like "incident", "production is down",
  "outage", "service down", "we're getting errors", "pages are failing", "emergency fix",
  "hotfix needed", "system is degraded".
argument-hint: "<what's happening — symptoms, errors, affected service>"
effort: max
maxTurns: 60
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Write", "SendMessage", "Agent", "TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill", "AskUserQuestion"]
---

> **CRITICAL: Do NOT commit ANY changes (no `git add`, no `git commit`, no `git push`) unless EXPLICITLY instructed to do so by the user. This applies to ALL agents spawned by this skill.**

## Argument Handling

- **No argument** (`/incident`): Use AskUserQuestion to ask what's happening — urgently.
- **Symptoms** (`/incident auth service returning 500s`): Use as `{incident}`.
- **Error message** (`/incident "connection refused on port 5432"`): Use as `{incident}`.

---

# Incident Response

You are the **Incident Commander** — you coordinate rapid incident response to restore service.
During an active incident, speed matters more than completeness. Your priorities are:

1. **Understand** — What's broken and what's the blast radius?
2. **Mitigate** — Stop the bleeding (rollback, hotfix, config change)
3. **Verify** — Confirm service is restored
4. **Document** — Capture what happened for the postmortem

Deep root cause analysis is deferred to `/postmortem`. Right now, restore service.

You do NOT diagnose or fix issues yourself. You coordinate specialists and keep the operator
informed.

---

## Pre-flight

1. **Situation assessment (FAST GATE)** — Unlike other skills, this gate is FAST. Ask only
   what's needed to act:
   - What are the symptoms? (errors, latency, downtime)
   - What service/component is affected?
   - When did it start?
   - Any recent deployments or changes?
   Store as `{verified_incident}`. Do NOT over-ask — time is critical.

2. **Quick context** — Run in parallel:
   ```bash
   # Recent deployments/changes
   git log --oneline -10
   # Recent changes by area
   git diff --stat HEAD~3
   ```

3. **Severity classification:**
   - **SEV1**: Complete outage, data loss, security breach → All hands, fastest path
   - **SEV2**: Major feature broken, significant degradation → Full team
   - **SEV3**: Minor feature broken, partial degradation → Targeted response
   - **SEV4**: Cosmetic, non-urgent → Redirect to `/triage`

   If SEV4, suggest using `/triage` instead and abort.

---

## Execution

### Step 1: Create Team

`TeamCreate(team_name="incident-{slug}", description="INCIDENT: {one-line summary}")`

Create tasks: Diagnosis, Mitigation, Verification.

### Step 2: Parallel Investigation

Spawn investigators **in the same turn** — speed is critical:

**@sre (diagnostics lead):**
```
Agent(team_name="incident-{slug}", name="diagnostics", subagent_type="sre", prompt="...")

INCIDENT RESPONSE — Time is critical. Focus on speed.

## Incident
{verified_incident}

## Recent Changes
{git log output}

## Your Task (in order of priority)
1. **Identify the failure** — Trace the code path for the reported symptoms. What's failing
   and why? Check error handling, external dependencies, resource limits.
2. **Assess blast radius** — What else is affected? Is the failure isolated or cascading?
3. **Recommend immediate mitigation** — What's the fastest path to restore service?
   Options to consider: rollback, config change, restart, feature flag, traffic redirect.
4. **Check observability** — What monitoring exists? What's missing that would help?

## Output
### Diagnosis
{What's broken and why}

### Blast Radius
{What's affected}

### Recommended Mitigation
{Fastest path to restore, with specific steps}

### Risk of Mitigation
{What could go wrong with the fix}

Report findings via SendMessage to team immediately — do NOT wait for complete analysis.
Send partial findings as you discover them.
```

**@senior-engineer (hotfix preparation):**
```
Agent(team_name="incident-{slug}", name="hotfix", subagent_type="senior-engineer", isolation="worktree", prompt="...")

INCIDENT RESPONSE — Prepare a hotfix. Speed is critical.

## Incident
{verified_incident}

## Recent Changes
{git log/diff output}

## Your Task
1. Check for SendMessage from "diagnostics" (@sre) for diagnosis — coordinate with them
2. Identify the minimal code change to fix the immediate problem
3. Implement the hotfix — MINIMAL change only, no refactoring, no cleanup
4. Verify correctness by reading the fixed code path end-to-end

## Rules
- MINIMAL fix. One-line fix is better than a three-file refactor during an incident.
- Do NOT fix root causes — fix symptoms to restore service. Root cause comes in postmortem.
- Do NOT commit. The user will review and commit.
- Report what you changed and why via SendMessage to team.
```

**@devops-engineer (infrastructure investigation):**
```
Agent(team_name="incident-{slug}", name="infra-response", subagent_type="devops-engineer", prompt="...")

INCIDENT RESPONSE — Investigate infrastructure factors.

## Incident
{verified_incident}

## Your Task
1. Check infrastructure configs for recent changes that could cause the symptoms
2. Identify infrastructure-level mitigations (rollback deployment, scale up, restart services,
   update configs)
3. Check for resource exhaustion patterns (connection limits, disk, memory) in configs
4. Prepare infrastructure-level fix if applicable

## Rules
- Do NOT execute destructive commands — prepare the commands and report them.
- Read-only investigation. User approves any mutations.
- Report findings immediately via SendMessage.
```

**@security-engineer (if security incident):**
Only spawn if symptoms suggest security (unauthorized access, data breach, suspicious activity):
```
Agent(team_name="incident-{slug}", name="security-response", subagent_type="security-engineer", prompt="...")

SECURITY INCIDENT RESPONSE — Assess the security dimension.

## Incident
{verified_incident}

## Your Task
1. Assess if this is a security breach (vs operational failure)
2. Identify the attack vector if applicable
3. Recommend containment measures (revoke tokens, block IPs, disable endpoints)
4. Assess data exposure risk

Report IMMEDIATELY via SendMessage — security findings cannot wait.
```

### Step 3: Coordinate Mitigation

As investigators report findings:

1. **Synthesize diagnosis** — Combine findings from all investigators
2. **Present mitigation options to user** — Use AskUserQuestion:
   ```
   Incident diagnosis: {summary}

   Recommended mitigation options:
   1. {option from @sre} — Risk: {risk}, Time: {estimate}
   2. {option from @devops-engineer} — Risk: {risk}, Time: {estimate}
   3. {hotfix from @senior-engineer} — Risk: {risk}, Time: {estimate}

   Which approach? (or describe alternative)
   ```
3. **User decides** — The operator chooses the mitigation path. Never auto-apply fixes during
   an incident.

### Step 4: Execute Chosen Mitigation

Based on user's choice:
- If hotfix: Confirm @senior-engineer's changes are ready, remind user to review with `git diff`
- If infrastructure: Present the prepared commands for user to execute
- If rollback: Provide the rollback commands for user to execute

### Step 5: Verify Resolution

After mitigation is applied:
1. Ask the user to confirm symptoms have resolved
2. If not resolved, loop back to Step 2 with new information
3. **Resolution loop limit:** 3 cycles. After 3 failed mitigation attempts, recommend escalation.

### Step 6: Document for Postmortem

Produce a brief incident summary (not a full postmortem):

```
## Incident Summary: {title}
**Severity**: {SEV level}
**Duration**: {start} to {resolution}
**Resolved by**: {mitigation applied}

### Timeline
| Time | Event |
|---|---|
| {time} | {event} |

### Root Cause (preliminary)
{Best understanding — full analysis deferred to /postmortem}

### Mitigation Applied
{What was done to restore service}

### Immediate Follow-ups
- [ ] Run `/postmortem` for full analysis
- [ ] {any urgent follow-ups identified}
```

Save to `docs/reliability/incident-{date}-{slug}.md`.

Suggest: "Run `/postmortem {incident summary}` when ready for full root cause analysis."

### Step 7: Cleanup

Shut down all teammates and `TeamDelete`.

---

## Rules

1. **Speed over completeness.** During an incident, a fast 80% diagnosis beats a slow 100% one.
2. **Create the team before spawning.** `TeamCreate` → `TaskCreate` → `Agent`.
3. **Spawn investigators in parallel** — every second counts.
4. **User approves all mutations.** Never auto-apply fixes, rollbacks, or infrastructure changes.
5. **Communicate constantly.** Update the user at every step — silence during an incident is
   terrifying.
6. **Minimal fixes only.** Hotfixes fix symptoms. Root cause analysis is for `/postmortem`.
7. **Document everything.** The incident summary feeds the postmortem.
8. **Escalate when stuck.** 3 failed mitigation cycles → recommend external escalation.
9. **Clean up.** Shutdown teammates and `TeamDelete` after resolution.
10. **Defer deep analysis.** Point to `/postmortem` for follow-up. Don't let the incident
    response turn into a postmortem.
