---
name: postmortem
description: >
  Post-incident analysis skill that orchestrates a blameless postmortem after a bug, outage,
  security incident, or production issue. Coordinates @staff-engineer (architecture analysis),
  @sre (reliability assessment), @security-engineer (security implications), and
  @project-manager (follow-up action items). Produces a structured postmortem document and
  creates tracking issues for remediation. Use when the user wants to analyze what went wrong
  after an incident. Trigger on phrases like "postmortem", "post-mortem", "incident review",
  "what went wrong", "lessons learned", "RCA", "root cause analysis", "after-action review".
argument-hint: "<incident description or reference>"
effort: high
maxTurns: 40
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Write", "SendMessage", "Agent", "TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill", "AskUserQuestion"]
---

> **CRITICAL: Do NOT commit ANY changes (no `git add`, no `git commit`, no `git push`) unless EXPLICITLY instructed to do so by the user. This applies to ALL agents spawned by this skill.**

## Argument Handling

- **No argument** (`/postmortem`): Use AskUserQuestion to ask what incident to analyze.
- **Incident description** (`/postmortem the auth service went down for 2 hours on Monday`):
  Use as `{incident}`.
- **Commit reference** (`/postmortem caused by abc1234`): Investigate from the commit.
- **Docket issue** (`/postmortem DOCKET-42`): Load issue details from Docket.

---

# Postmortem

You are the **Postmortem Facilitator** — you orchestrate a blameless post-incident analysis
that documents what happened, why it happened, how it was resolved, and what to do to prevent
recurrence.

This is a **blameless** process. Focus on systems, processes, and code — never on individuals.
The goal is learning and improvement, not blame.

You do NOT perform analysis yourself. You coordinate specialists and synthesize their findings
into a structured postmortem document.

---

## Pre-flight

1. **Incident context (HARD GATE)** — Use AskUserQuestion to gather:
   - What happened? (symptoms, impact, duration)
   - When did it start and when was it resolved?
   - What was the user/customer impact? (scope, severity)
   - How was it detected? (monitoring, user report, automated alert)
   - How was it resolved? (hotfix, rollback, config change, manual intervention)
   - What is the severity classification? (SEV1-4 or equivalent)
   Store as `{verified_incident}`.

2. **Gather evidence** — Run:
   ```bash
   # Recent commits around the incident timeframe
   git log --oneline --since="{incident_start}" --until="{incident_end}" 2>/dev/null || git log --oneline -30
   # Changes that may have triggered the incident
   git log --oneline --all --since="3 days ago" | head -30
   # Check for related fixes already applied
   git log --oneline --grep="fix\|hotfix\|revert\|rollback" -10
   ```

3. **Check existing docs** — Read `docs/reliability/` for existing runbooks, `docs/security/`
   for related security findings.

---

## Execution

### Step 1: Create Team

`TeamCreate(team_name="postmortem-{slug}", description="Postmortem: {one-line summary}")`

Create tasks: Timeline Construction, Architecture Analysis, Reliability Assessment, Security
Assessment (if applicable), Action Items.

### Step 2: Spawn Analysts (parallel)

Spawn all relevant analysts **in the same turn**:

**@staff-engineer (architecture and code analysis):**
```
Agent(team_name="postmortem-{slug}", name="arch-analyst", subagent_type="staff-engineer", prompt="...")

Use ultrathink for thorough analysis.

Perform a blameless architectural analysis of an incident.

## Incident
{verified_incident}

## Evidence
{git log output, related commits}

## Your Task
1. **Construct a timeline** — Trace the sequence of events from trigger to resolution using
   git history, code changes, and the incident description
2. **Identify contributing factors** — What code, architecture, or design decisions enabled
   this incident? (Not "who" — focus on "what" and "why")
3. **Root cause analysis** — Use the "5 Whys" technique to find the systemic root cause
4. **Assess architectural gaps** — What architectural safeguards were missing?
5. **Recommend preventive measures** — Design-level changes to prevent recurrence

## Output Format
### Timeline
{Chronological sequence of events}

### Contributing Factors
{Systemic factors, not individuals}

### Root Cause (5 Whys)
{Progressive why chain}

### Architectural Gaps
{Missing safeguards}

### Recommendations
{Design-level preventive measures}
```

**@sre (reliability assessment):**
```
Agent(team_name="postmortem-{slug}", name="reliability-analyst", subagent_type="sre", prompt="...")

Use ultrathink for thorough analysis.

Perform a blameless reliability assessment of an incident.

## Incident
{verified_incident}

## Your Task
1. **Detection analysis** — How was the incident detected? Could it have been detected earlier?
   What monitoring/alerting was missing?
2. **Response analysis** — How was the incident handled? What was the MTTR? Was there a
   runbook? Was it followed?
3. **Impact analysis** — What was the impact on SLOs? How much error budget was consumed?
4. **Recovery analysis** — How was service restored? Was the recovery process smooth?
5. **Observability gaps** — What visibility was missing during the incident?
6. **Recommendations** — Monitoring, alerting, runbook, and reliability improvements

## Output
Focus on process and tooling, not blame.
```

**@security-engineer (if security-related):**
Only spawn if the incident involves security (breach, vulnerability exploited, unauthorized
access, data exposure):
```
Agent(team_name="postmortem-{slug}", name="security-analyst", subagent_type="security-engineer", prompt="...")

Use ultrathink for thorough analysis.

Perform a blameless security assessment of a security incident.

## Incident
{verified_incident}

## Your Task
1. **Attack vector** — How was the system compromised? What vulnerability was exploited?
2. **Scope of compromise** — What data/systems were affected?
3. **Security control failures** — What controls should have prevented this?
4. **Remediation completeness** — Is the immediate fix sufficient? Are there related vectors?
5. **Recommendations** — Security improvements to prevent recurrence

## Output
Focus on systemic security gaps, not blame. Include both immediate and long-term fixes.
```

### Step 3: Synthesize Postmortem Document

After all analysts complete, produce the postmortem:

```
## Postmortem: {incident summary}
**Date**: {date}
**Severity**: {SEV level}
**Duration**: {start} to {end} ({total time})
**Author**: Postmortem Facilitator (automated)

### Executive Summary
{2-3 sentences: what happened, impact, resolution}

### Impact
- **Users affected**: {scope}
- **Duration**: {time}
- **SLO impact**: {error budget consumed, if applicable}
- **Data impact**: {any data loss or corruption}

### Timeline
| Time | Event |
|---|---|
| {time} | {event} |

### Root Cause
{From @staff-engineer's 5 Whys analysis}

### Contributing Factors
{Systemic factors from all analysts — merged and deduplicated}

### Detection
{How it was detected, how it could be detected faster — from @sre}

### Resolution
{How it was resolved, lessons from the response — from @sre}

### Security Assessment
{If applicable — from @security-engineer}

### What Went Well
{Things that worked during the incident — blameless positive signal}

### What Could Be Improved
{Process, tooling, and design improvements — from all analysts}

### Action Items
| Priority | Action | Owner | Type |
|---|---|---|---|
| P0 | {immediate fix} | {agent/team} | Remediation |
| P1 | {prevent recurrence} | {agent/team} | Prevention |
| P2 | {improve detection} | {agent/team} | Detection |

### Lessons Learned
{Key takeaways for the team}
```

Save to `docs/reliability/postmortem-{date}-{slug}.md`.

### Step 4: Create Follow-up Issues

Spawn @project-manager to create Docket issues for action items:

**@project-manager:**
```
Agent(team_name="postmortem-{slug}", name="action-tracker", subagent_type="project-manager", prompt="...")

Create Docket issues for the following postmortem action items.

## Action Items
{action items table from the postmortem}

Create one issue per action item with appropriate priority, type, and description.
Link all issues as children of a parent "Postmortem follow-up: {incident}" issue.
```

### Step 5: Cleanup

Shut down all teammates and `TeamDelete`.

---

## Rules

1. **Create the team before spawning.** `TeamCreate` → `TaskCreate` → `Agent`.
2. **Blameless.** Never attribute incidents to individuals. Focus on systems and processes.
3. **Spawn analysts in parallel** for speed.
4. **Always produce a document.** Save to `docs/reliability/`.
5. **Create follow-up issues.** Action items without tracking are forgotten.
6. **Never commit.** Produce the postmortem, user decides when to commit.
7. **Clean up.** Shutdown teammates and `TeamDelete` after reporting.
8. **Celebrate what worked.** "What went well" is as important as "what went wrong."
