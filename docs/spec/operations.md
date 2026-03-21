---
project: "project"
maturity: "proof-of-concept"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "Operational posture of the Homelab the Game backend, desktop client, and supporting infrastructure"
owner: "@staff-engineer"
dependencies:
  - architecture.md
  - security.md
---

# Operations Specification

## 1. Overview

This document describes the current operational state of the Homelab the Game project as of
2026-03-20. It is based on a thorough examination of the codebase and reflects what actually
exists -- not what is planned or aspirational. The project is in a proof-of-concept stage from
an operations standpoint: there is a functioning backend and desktop client, but nearly all
operational infrastructure (CI/CD, monitoring, alerting, deployment automation, rollback
procedures) is absent.

## 2. Runtime Architecture

### 2.1 Backend (Go)

The backend is a single Go binary (`apps/backend/cmd/server/main.go`) that runs as a monolithic
HTTP server using Go's standard library `net/http`. It has no process manager, supervisor,
or graceful shutdown logic.

**Process lifecycle:**
- Started manually with `go run ./cmd/server/` or by executing the pre-built `server` binary
  (`apps/backend/server`, 14.8 MB, checked into `.gitignore`)
- Listens on a configurable port (default `:8080`) via `http.ListenAndServe`
- No graceful shutdown handler -- the server does not trap SIGTERM/SIGINT, so in-flight
  requests will be terminated abruptly on process kill
- No PID file, no systemd unit, no process supervision of any kind observed in the repository

**Connection pooling:**
- Uses `pgx/v5` connection pool (`pgxpool`) with `MaxConns=20`, `MinConns=2`
- Pool is closed via `defer pool.Close()` on the main goroutine -- adequate for development,
  but will not execute during unclean termination (SIGKILL)
- Database connection string uses `sslmode=disable` (PostgreSQL on localhost)

**WebSocket hub:**
- A single in-memory `ws.Hub` manages all connected WebSocket clients (map of userID to
  `*websocket.Conn`)
- Ping/pong keepalive at 30s/45s intervals; comment in code notes this is designed to survive
  an nginx reverse proxy
- No connection limit or backpressure mechanism -- all concurrent connections are held in memory
- Old connections for the same user are forcibly closed when a new connection arrives (single
  session enforcement)

### 2.2 Desktop Client (Tauri + React)

- Vite dev server on port 3000 (`host: 0.0.0.0`, `allowedHosts: ['game.homelab.living']`)
- Production build via `tsc && vite build` -- outputs static assets
- API URL defaults to `https://api.homelab.living` (overridable via `VITE_API_URL`)
- No Tauri-specific deployment packaging (auto-update, installer generation) is configured
  in the repository

### 2.3 Database (PostgreSQL + TimescaleDB)

- PostgreSQL with TimescaleDB extension for time-series tables (`resource_history`, `event_log`)
- 9 sequential migration files (`001_initial_schema.sql` through `009_global_cu_store.sql`)
  plus one-off wipe scripts
- Migrations are applied manually via `psql` -- there is no migration tool, no version tracking
  table, no up/down migration pattern
- One migration file has been renamed to `.APPLIED` extension as a manual tracking mechanism
  (`007_wipe_player_progress.APPLIED`)

## 3. Configuration Management

### 3.1 Environment Variables

Configuration is loaded from environment variables with a hand-rolled `.env` file parser in
`main.go`. There is no configuration validation, no required-variable enforcement (except a
warning for `JWT_SECRET`), and no support for configuration profiles or environments.

| Variable | Default | Purpose |
|---|---|---|
| `PORT` | `8080` | HTTP listen port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `homelab_game` | Database user |
| `DB_PASSWORD` | (empty) | Database password |
| `DB_NAME` | `homelab_game` | Database name |
| `JWT_SECRET` | (random per start) | JWT signing key |
| `ENV` | (unset) | When set to `production`, disables dev CORS origins |
| `CORS_ORIGINS` | (unset) | Additional comma-separated allowed origins |

**Gap:** The `.env` file (`apps/backend/.env`) contains a hardcoded database password and
JWT secret. While `.env` is in `.gitignore`, the file exists on disk in the working tree. There
is no secrets management solution (no Vault, no SOPS, no sealed secrets).

### 3.2 Hardcoded Configuration

Several operational parameters are hardcoded rather than configurable:

- Database pool sizes (`MaxConns=20`, `MinConns=2`) in `database/db.go`
- Rate limit windows (all fixed at 1 minute) and thresholds in `middleware/ratelimit.go`
- WebSocket ping/pong intervals (30s/45s) in `ws/hub.go`
- JWT token expiry (24 hours) in `auth/jwt.go`
- Request body size limit (64 KB) in `middleware/bodylimit.go`
- CORS allowed origins (partially hardcoded list including `game.homelab.living`,
  `homelab.living`, and `192.168.3.107:3000`) across both `middleware/cors.go` and `ws/hub.go`

## 4. CI/CD Pipeline

**Status: Does not exist.**

There is no `.github/` directory, no CI/CD configuration files of any kind (no GitHub Actions,
no GitLab CI, no Jenkins, no Makefile, no build scripts). The ROADMAP explicitly lists
"CI/CD pipeline setup" as a Phase 13 (Polish & Launch) item that has not been started.

**Current build process:**
- Backend: `go build ./cmd/server/` (manual)
- Desktop: `pnpm build` (manual)
- Tests: `go test ./...` (manual -- and there are zero test files in the backend)
- No linting, formatting, or static analysis tooling is configured

## 5. Deployment

**Status: Manual, ad-hoc.**

There are no deployment scripts, no Dockerfiles, no docker-compose files, no Kubernetes
manifests, no Terraform/Pulumi/Ansible, no systemd unit files, and no Makefile in the
repository.

**Inferred deployment model** (from CLAUDE.md and code clues):
- Everything runs on a single homelab VM (as confirmed by the project's memory files and
  CLAUDE.md: "Infrastructure is self-hosted on a homelab VM")
- The Go backend binary is run directly on the host
- An nginx reverse proxy sits in front of the backend (inferred from the WebSocket ping
  comment: "keeps connection alive through nginx proxy" and the `X-Forwarded-For` /
  `X-Real-IP` header handling in the rate limiter)
- The Vite dev server appears to also be exposed through the proxy at `game.homelab.living`,
  with API traffic going to `api.homelab.living`
- PostgreSQL runs locally (`DB_HOST=localhost`, `sslmode=disable`)

**Rollback procedure: None defined.** There is no versioning, no release tagging, no
artifact registry. Rolling back would require manually reverting a git commit and rebuilding.
Database migrations are forward-only with no down migrations.

## 6. Monitoring and Observability

**Status: Effectively absent.**

### 6.1 Health Check

A single health endpoint exists:

```
GET /health -> 200 {"status":"ok"}
```

This is a shallow check -- it does not verify database connectivity, connection pool health,
or any downstream dependencies. It always returns 200 if the HTTP server is listening.

### 6.2 Logging

- All logging uses Go's standard `log` package (unstructured, to stdout)
- Log messages are sparse and limited to:
  - Startup: "Connected to database", "Homelab Game API starting on :8080"
  - WebSocket: "WebSocket client connected", "WebSocket client disconnected",
    "WebSocket upgrade error: ..."
  - Fatal: database connection failure, HTTP server failure
- No request logging middleware (no access logs)
- No structured logging (no JSON, no log levels, no correlation IDs)
- No log aggregation or shipping

### 6.3 Metrics

**None.** There is no metrics collection (no Prometheus, no StatsD, no OpenTelemetry).
No counters for requests, errors, latencies, or game actions. No database query timing.
No connection pool metrics exposed.

### 6.4 Tracing

**None.** No distributed tracing, no request IDs, no span context propagation.

### 6.5 Alerting

**None.** No alerting rules, no PagerDuty/OpsGenie integration, no alert definitions.

### 6.6 Dashboards

**None.** No Grafana, no Kibana, no custom dashboard.

## 7. Rate Limiting and Traffic Management

### 7.1 Rate Limiting

The project has a custom in-memory rate limiter (`middleware/ratelimit.go`) with the
following configuration:

| Endpoint Category | Limit | Key |
|---|---|---|
| Auth (register, login) | 10/min | Client IP |
| Game actions | 7200/min (~120/sec) | User ID (falls back to IP) |
| Social actions | 180/min | User ID (falls back to IP) |

**Implementation details:**
- Visitors map cleaned every 60 seconds (entries older than 1 minute)
- Counter resets completely after 1 minute of inactivity (not sliding window)
- Uses `X-Forwarded-For` (first IP) or `X-Real-IP` for client identification --
  trusts proxy headers without validation of the proxy source
- State is entirely in-memory -- lost on restart, not shared across instances

### 7.2 Request Size Limiting

- Global 64 KB body size limit via `http.MaxBytesReader`

### 7.3 CORS

- Allowed origins: `game.homelab.living`, `homelab.living` (http and https)
- Dev mode (when `ENV != "production"`): additionally allows `localhost:3000`,
  `127.0.0.1:3000`, `192.168.3.107:3000`
- Extra origins configurable via `CORS_ORIGINS` env var

**Gap:** The WebSocket upgrader in `ws/hub.go` has a separate, hardcoded origin allowlist
that does not read `CORS_ORIGINS` or respect the `ENV` variable. These two origin lists can
drift.

## 8. Database Operations

### 8.1 Migrations

- 9 numbered migration files in `apps/backend/internal/database/migrations/`
- Applied manually via `psql -d homelab_game -f <file>`
- No migration framework (no goose, no golang-migrate, no Flyway)
- No down/rollback migrations
- No migration state tracking table -- the developer renames files to `.APPLIED` as a
  manual record
- One standalone `wipe_player_progress.sql` script exists for full progress reset during
  development

### 8.2 Backup and Recovery

**Not implemented.** There is no backup strategy, no pg_dump automation, no point-in-time
recovery configuration, no WAL archiving.

### 8.3 Connection Management

- pgxpool with 20 max connections, 2 min connections
- No connection health checks beyond the initial `pool.Ping` at startup
- No query timeouts configured (uses default pgx behavior)
- All queries use direct pool.Exec/QueryRow with request-scoped contexts

## 9. Load Testing

A stress test tool exists at `stress-tests/` (Go binary). This is the project's only
operational tooling.

**Capabilities:**
- Simulates N concurrent players (configurable, default 100)
- Phases: registration, warm-up state fetch, optional WebSocket connections, main action loop
- Configurable action rate (default 500ms per player), duration, ramp-up period
- Reports throughput (req/sec), latency percentiles (P50/P90/P95/P99), error rate,
  rate-limited count
- Live stats printed every 5 seconds during test

**Usage:**
```bash
cd stress-tests && go build -o stresstest .
./stresstest -url http://localhost:8080 -players 500 -duration 2m -rate 200ms
./stresstest -players 200 -ws    # with WebSocket connections
```

## 10. Security Operations

Covered in detail by the security spec; operational highlights only here:

- JWT tokens expire after 24 hours, signed with HS256
- Passwords hashed with bcrypt (default cost)
- No audit logging
- No intrusion detection
- No vulnerability scanning in the build pipeline (no build pipeline exists)
- Database credentials stored in plaintext `.env` file
- Database connection uses `sslmode=disable`
- No TLS termination in the Go server itself (assumed to be handled by nginx)

## 11. Concurrency and State Safety

### 11.1 Per-User Locking

The `GameHandler` uses a `userMutexMap` to serialize game actions per user. This prevents
race conditions from concurrent requests for the same player.

**Gap:** The mutex map grows unboundedly -- user locks are never evicted. For a long-running
server with many registered users, this is a memory leak (one `sync.Mutex` per user who has
ever performed an action).

### 11.2 In-Memory State

Several pieces of operational state exist only in memory:

- Rate limiter visitor counts
- WebSocket client connections
- Per-user action mutexes

All of this state is lost on server restart. This is acceptable for a single-instance
deployment but would prevent horizontal scaling without architectural changes.

## 12. Error Handling

- Errors are returned as JSON `{"error":"..."}` with appropriate HTTP status codes
- No error classification, error codes, or error tracking (no Sentry, no Bugsnag)
- Several error paths in `PerformAction` silently `continue` on bulk persistence failures
  rather than rolling back -- partial state corruption is possible during bulk operations
- No circuit breakers for database calls
- No retry logic for transient failures

## 13. Operational Gaps Summary

This table summarizes the gap between current state and what would be needed for a production
service. Items are ordered by risk if the system were to go live as-is.

| Category | Current State | Risk if Unaddressed |
|---|---|---|
| Graceful shutdown | Not implemented | Data loss on deploy; in-flight requests dropped |
| Database backups | Not implemented | Unrecoverable data loss on disk failure |
| CI/CD pipeline | Not implemented | Manual builds error-prone; no automated quality gates |
| Monitoring/metrics | Not implemented | Blind to performance degradation and errors |
| Structured logging | Not implemented | Cannot diagnose issues in production |
| Alerting | Not implemented | Outages discovered by users, not operators |
| Migration tooling | Manual psql | Risk of missed or double-applied migrations |
| Health check depth | Shallow (HTTP only) | Load balancer routes to unhealthy instances |
| Deployment automation | Not implemented | Manual deploys are slow and error-prone |
| Error tracking | Not implemented | Bugs surface as user complaints |
| Request logging | Not implemented | No visibility into traffic patterns or abuse |
| TLS for database | Disabled | Credentials sent in cleartext (mitigated by localhost) |
| Secrets management | Plaintext .env | Credential exposure risk |
| Test suite | Empty (0 test files) | No regression safety net |
| Rollback procedure | Not defined | Cannot quickly revert bad deploys |

## 14. What Works Well

Despite the gaps, the following operational aspects are sound for the project's current
proof-of-concept stage:

- **Server-authoritative design**: All game state mutations are validated server-side, which
  is the correct foundation for a multiplayer game
- **Connection pooling**: pgxpool is properly configured with sensible min/max connections
- **Rate limiting**: Multi-tier rate limiting (IP-based for auth, user-based for game actions)
  provides basic abuse protection
- **Per-user action locking**: Prevents race conditions that could corrupt game state
- **WebSocket keepalive**: Proper ping/pong with timeouts prevents connection zombies
- **Body size limits**: 64 KB limit prevents large payload abuse
- **Stress test tooling**: Having a load test tool this early is unusual and valuable --
  it enables data-driven capacity planning before launch
- **CORS configuration**: Proper origin allowlisting with environment-aware dev mode
- **Clean separation of concerns**: The backend code is well-structured with clear boundaries
  (handlers, middleware, engine, queries), which will make adding operational tooling
  straightforward when the time comes
