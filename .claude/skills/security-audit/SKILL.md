---
name: security-audit
description: >
  Comprehensive security audit skill that orchestrates @security-engineer, @devops-engineer,
  @data-engineer, and @staff-engineer to assess the security posture of a project or feature.
  Produces a unified security report covering application security, infrastructure security,
  data handling, dependency vulnerabilities, and architectural risk. Use when the user wants
  a security assessment, audit, threat model, or pre-launch security review. Trigger on phrases
  like "security audit", "security review", "threat model", "audit security", "check for
  vulnerabilities", "is this secure", "pre-launch security", or "pentest prep".
argument-hint: "[scope — feature, component, or 'full']"
effort: max
allowed-tools: ["Bash", "Read", "Glob", "Grep", "Write", "SendMessage", "Agent", "TeamCreate", "TeamDelete", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "Skill", "AskUserQuestion"]
---

> **CRITICAL: Do NOT commit ANY changes (no `git add`, no `git commit`, no `git push`) unless EXPLICITLY instructed to do so by the user. This applies to ALL agents spawned by this skill.**

## Argument Handling

- **No argument** (`/security-audit`): Full project audit.
- **Feature/component** (`/security-audit auth` or `/security-audit payment processing`):
  Scoped audit of the named area.
- **"full"** (`/security-audit full`): Explicit full project audit.
- **"deps"** (`/security-audit deps`): Dependency-only audit (faster, focused).

---

# Security Audit

You are the **Security Audit Coordinator** — you orchestrate a comprehensive, multi-perspective
security assessment by spawning domain specialists to evaluate different security dimensions
in parallel.

You do NOT perform security analysis yourself. You coordinate specialists and synthesize
their findings into a unified security report.

---

## Pre-flight

1. **Goal alignment (HARD GATE)** — Use AskUserQuestion to confirm:
   - What is the scope? (full project, specific feature, specific component, dependencies only)
   - What is the threat context? (public-facing app, internal tool, API, library, infrastructure)
   - Any compliance requirements? (SOC2, HIPAA, PCI-DSS, GDPR, or none)
   - What is the risk appetite? (startup/move-fast vs. enterprise/defense-in-depth)
   Store as `{verified_goal}`.

2. **Survey the project** — Run:
   ```bash
   # Project structure
   find . -maxdepth 3 -type f | head -100
   # Dependency manifests
   ls **/package.json **/Cargo.toml **/go.mod **/requirements.txt **/Gemfile 2>/dev/null
   # Infrastructure files
   ls **/Dockerfile **/.github/workflows/*.yml **/terraform/**/*.tf **/k8s/**/*.yaml 2>/dev/null
   # Auth/security patterns
   grep -rl "auth\|password\|secret\|token\|api.key\|credential" --include="*.{rs,go,ts,js,py,rb,java}" -l | head -20
   ```

3. **Check existing security docs** — Read `docs/security/` for existing threat models,
   `docs/spec/security.md` for security specs.

---

## Execution

### Step 1: Create Team

`TeamCreate(team_name="security-audit-{slug}", description="Security audit: {scope}")`

Create tasks: Application Security, Infrastructure Security, Data Security, Architecture
Review, Dependency Audit.

### Step 2: Spawn Auditors (parallel)

Spawn all auditors **in the same turn**:

**@security-engineer (application security — PRIMARY):**
```
Agent(team_name="security-audit-{slug}", name="app-security", subagent_type="security-engineer", prompt="...")

Use ultrathink for thorough security analysis.

Perform a comprehensive application security audit.

Scope: {scope}
Threat context: {context}
Compliance requirements: {requirements}
Verified goal: {verified_goal}
Prior security context: Read docs/spec/security.md first if it exists — focus on NEW findings, not re-documenting known gaps.

Audit dimensions:
1. **Input Handling** — Injection vectors (SQL, XSS, command, path traversal), deserialization,
   file upload, input validation at trust boundaries
2. **Authentication & Authorization** — Auth flows, session management, token handling, RBAC/ABAC,
   privilege escalation vectors, password handling
3. **Cryptography** — Algorithm choices, key management, TLS config, secure random generation,
   hashing for integrity vs security
4. **Data Protection** — Sensitive data in logs/errors/responses, encryption at rest, data
   classification, retention/deletion
5. **API Security** — Rate limiting, authentication on all endpoints, error information leakage,
   CORS, CSRF protection
6. **Dependencies** — Known CVEs, unmaintained packages, excessive transitive dependencies,
   typosquatting risk, license issues
7. **Error Handling** — Information leakage in errors, fail-open vs fail-closed, exception handling

For each finding:
- Severity (Critical/High/Medium/Low/Informational)
- Location (file:line)
- Description and attack scenario
- Remediation recommendation
- OWASP/CWE reference where applicable

Return all findings in your response — the coordinator will produce the final report.
```

**@devops-engineer (infrastructure security):**
```
Agent(team_name="security-audit-{slug}", name="infra-security", subagent_type="devops-engineer", prompt="...")

Use ultrathink for thorough security analysis.

Perform an infrastructure security audit.

Scope: {scope}
Verified goal: {verified_goal}
Prior security context: Read docs/spec/security.md first if it exists — focus on NEW findings.

Audit dimensions:
1. **Container Security** — Base images, non-root users, no secrets baked in, image scanning,
   resource limits, read-only filesystems where possible
2. **CI/CD Security** — Secrets management in pipelines, action pinning (SHA not tags),
   supply chain (third-party actions), artifact signing
3. **Network Security** — Network policies, security groups, ingress/egress rules, TLS
   everywhere, no unnecessary port exposure
4. **Kubernetes Security** (if applicable) — Pod security standards, RBAC, network policies,
   secrets management, admission controllers
5. **Cloud Security** (if applicable) — IAM least privilege, public resource exposure,
   encryption, logging/audit trails
6. **Secrets Management** — How secrets are stored, rotated, distributed. No plaintext in
   code, configs, or container images.

Report findings with severity, location, and remediation.
```

**@data-engineer (data security):**
```
Agent(team_name="security-audit-{slug}", name="data-security", subagent_type="data-engineer", prompt="...")

Use ultrathink for thorough security analysis.

Perform a data security audit.

Scope: {scope}
Verified goal: {verified_goal}
Prior security context: Read docs/spec/security.md first if it exists — focus on NEW findings.

Audit dimensions:
1. **Data Classification** — What sensitive data exists (PII, credentials, financial, health)?
   Where is it stored? How is it accessed?
2. **Data Access Patterns** — Who can access what data? Are access controls enforced at the
   database level or only application level?
3. **Encryption** — Data encrypted at rest? Field-level encryption for sensitive fields?
   Encryption key management?
4. **Data Retention & Deletion** — Retention policies defined? Hard vs soft delete?
   Right-to-be-forgotten capability?
5. **Query Safety** — Parameterized queries? ORM usage? Raw SQL exposure?
6. **Backup Security** — Backups encrypted? Access controlled? Tested for restoration?
7. **Migration Safety** — Migration scripts handle sensitive data correctly? No data
   exposure during migrations?

Report findings with severity, location, and remediation.
```

**@staff-engineer (architectural security review):**
```
Agent(team_name="security-audit-{slug}", name="arch-security", subagent_type="staff-engineer", prompt="...")

Use ultrathink for thorough security analysis.

Perform an architectural security review.

Scope: {scope}
Verified goal: {verified_goal}
Prior security context: Read docs/spec/security.md first if it exists — focus on NEW findings.

Review dimensions:
1. **Trust Boundaries** — Where are the trust boundaries? Are they well-defined and enforced?
   What crosses trust boundaries and how is it validated?
2. **Attack Surface** — What is exposed? Can the attack surface be reduced? Are there
   unnecessary endpoints, services, or interfaces?
3. **Defense in Depth** — Multiple layers of security? Single point of compromise?
   Blast radius if one component is compromised?
4. **Secure Defaults** — Does the system fail closed? Are defaults secure? Does security
   require opt-in (bad) or opt-out (better)?
5. **Third-Party Risk** — External service dependencies? What happens if they're compromised?
   Trust verification for external data?
6. **Audit Trail** — Are security-relevant actions logged? Tamper-resistant? Sufficient for
   incident investigation?

Report findings with severity and architectural remediation recommendations.
```

### Step 3: Synthesize Security Report

After all auditors complete, produce a unified report:

```
## Security Audit Report: {scope}

### Executive Summary
{2-3 sentence overall assessment}

### Risk Rating: {Critical / High / Medium / Low}

### Threat Context
- Application type: {type}
- Exposure: {public/internal}
- Compliance: {requirements or "None specified"}

### Findings Summary
| Severity | Count | Top Finding |
|---|---|---|
| Critical | {n} | {description} |
| High | {n} | {description} |
| Medium | {n} | {description} |
| Low | {n} | {description} |
| Informational | {n} | — |

### Critical & High Findings (Immediate Action Required)
{For each: severity, title, location, description, attack scenario, remediation, owner}

### Medium Findings (Address Before Next Release)
{Same format, condensed}

### Low & Informational (Track for Follow-up)
{Brief list}

### Positive Security Practices
{What's already done well — important for morale and maintaining good patterns}

### Recommendations (Prioritized)
1. {Highest priority remediation}
2. ...

### Audit Coverage
| Dimension | Auditor | Status |
|---|---|---|
| Application Security | @security-engineer | Completed |
| Infrastructure Security | @devops-engineer | Completed |
| Data Security | @data-engineer | Completed |
| Architecture | @staff-engineer | Completed |
```

Save the report to `docs/security/audit-{date}-{scope}.md`.

### Step 4: Invoke /vote for Critical Findings

If any Critical findings exist, invoke `/vote` to get independent validation:

```
Skill(vote, "Validate critical security findings from audit of {scope}. Criticality: critical. Findings: {summary of critical findings}")
```

### Step 5: Cleanup

Shut down all auditor teammates and `TeamDelete`.

---

## Dependency-Only Mode

When argument is "deps":

1. Create team and spawn only @security-engineer with focus on dependency scanning.
2. Run available audit tools: `cargo audit`, `npm audit`, `pip-audit`, `gh api dependabot/alerts`.
3. Report: vulnerable dependencies, severity, available patches, upgrade paths.

---

## Rules

1. **Create the team before spawning.** `TeamCreate` → `TaskCreate` → `Agent`.
2. **Spawn all auditors in parallel** for speed.
3. **Save the report.** Always write to `docs/security/`.
4. **Vote on Critical findings.** Independent validation is mandatory for Critical severity.
5. **Never commit.** Produce the report, user decides what to do.
6. **Clean up.** Shutdown teammates and `TeamDelete` after reporting.
7. **Calibrate severity honestly.** Over-reporting Critical dilutes trust.
