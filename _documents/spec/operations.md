---
project: "homelab-the-game"
maturity: "experimental"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Operational runbook and infrastructure reference for the Homelab the Game monorepo"
owner: "@staff-engineer"
dependencies:
  - ../../CLAUDE.md
  - ../tdd/horizontal-scaling.md
  - ../tdd/migrate-kubernetes.md
---

# Operations Specification

This document describes the operational infrastructure, deployment process, monitoring, and
incident response for Homelab the Game **as it actually exists today**. It is intended for
developers contributing code who need to understand how the system runs, how to deploy changes,
and what to do when things break.

---

## 1. Infrastructure Overview

### Single-VM Architecture

Everything runs on a single homelab VM. There are no separate dev, staging, or production
environments. The live game and the development codebase share the same machine.

| Component | Technology | Location |
|---|---|---|
| Backend API | Go binary in distroless container | Docker Swarm service, 2 replicas |
| Frontend | Vite-built SPA in nginx container | Docker Swarm service, 1 replica |
| Database | PostgreSQL 16 + TimescaleDB | Docker Swarm service, 1 replica, persistent volume |
| Cache | Redis 7 (Alpine) | Docker Swarm service, 1 replica, ephemeral (no persistence) |
| Ingress | Traefik v3.6 | Docker Swarm service, 1 replica, manager-only |
| TLS Termination | External nginx (NOT on this VM) | Separate reverse proxy |
| Container Registry | GitHub Container Registry (ghcr.io) | GitHub-hosted |
| CI/CD | GitHub Actions | GitHub-hosted |

### Network Topology

```
Internet
   |
External nginx (TLS termination, not on this VM)
   |
   +-- game.homelab.living  --> :80 --> Traefik --> frontend (nginx, port 80)
   +-- api.homelab.living   --> :80 --> Traefik --> backend (Go, port 8085)
```

Docker Swarm defines two overlay networks:

- **frontend**: Traefik, backend, and frontend containers. Traefik routes traffic via
  Swarm service labels.
- **backend**: Backend, database, Redis, and migrations containers. Not reachable from
  Traefik or the internet.

The backend is attached to both networks because it must be reachable by Traefik (frontend
network) and must reach PostgreSQL and Redis (backend network). The `traefik.docker.network`
label explicitly tells Traefik to use the frontend network for service discovery, which is
required because Traefik cannot resolve overlay IPs on the backend network.

### Dev URLs

In addition to the production URLs, the following dev URLs point to this machine:

- `dev-game.homelab.living` -- frontend dev access
- `dev-api.homelab.living` -- backend dev access

These are configured in the CORS middleware (`apps/backend/internal/api/middleware/cors.go`)
and WebSocket upgrader (`apps/backend/internal/api/ws/hub.go`) as allowed origins.

---

## 2. Container Images

Three Docker images are built by GitHub Actions and pushed to GHCR on every push to `main`.
Pull requests run tests and typecheck but do not build or push images.

### 2.1 Backend (`homelab-game-backend`)

- **Dockerfile**: `apps/backend/Dockerfile`
- **Build stage**: `golang:1.25-alpine` -- compiles the server binary and a separate
  healthcheck binary
- **Runtime stage**: `gcr.io/distroless/static-debian12` -- minimal, no shell, no package
  manager
- **Binaries copied**: `/server` (main API) and `/healthcheck` (HTTP check against `/health`)
- **Runs as**: `nonroot:nonroot`
- **Exposed port**: 8080 (overridden to 8085 via `PORT` env var in docker-stack.yml)
- **Healthcheck**: Built-in `HEALTHCHECK` directive runs `/healthcheck` binary every 10s

### 2.2 Frontend (`homelab-game-frontend`)

- **Dockerfile**: `apps/desktop/Dockerfile`
- **Build stage**: `node:22-alpine` with pnpm -- builds the Vite SPA
- **Runtime stage**: `nginx:1.27-alpine` -- serves static files
- **Build arg**: `VITE_API_URL` (defaults to `https://api.homelab.living`, set at build time
  in CI)
- **nginx config**: `apps/desktop/nginx.conf` -- SPA fallback, aggressive caching for
  `/assets/`, no-cache on `index.html`, gzip enabled
- **Healthcheck**: `wget --spider -q http://localhost/` every 10s

### 2.3 Migrations (`homelab-game-migrations`)

- **Dockerfile**: `apps/backend/migrations.Dockerfile`
- **Base image**: `postgres:16-alpine` (provides `psql`, `pg_isready`)
- **Entrypoint**: `apps/backend/migrations-entrypoint.sh`
- **Behavior**: Waits for PostgreSQL readiness, ensures TimescaleDB extension, creates a
  `schema_migrations` tracking table, then applies each numbered `.sql` file in
  `apps/backend/internal/database/migrations/` that has not already been recorded. Idempotent.
- **Runs as a Swarm service** with `restart_policy: on-failure, max_attempts: 3`. It is NOT
  a one-shot job -- it runs as a service that exits after completing, and Swarm restarts it
  only on failure.

**Image tags**: Every push to `main` produces two tags per image: `sha-<commit>` and `latest`.

---

## 3. CI/CD Pipeline

### GitHub Actions Workflow

**File**: `.github/workflows/build.yml`

**Trigger**: Push to `main` or pull request targeting `main`.

**Jobs** (run in parallel):

| Job | Steps (PR) | Steps (Push to main) |
|---|---|---|
| `backend` | Checkout, setup Go (version from go.mod), `go test ./...` | + Docker Buildx, GHCR login, build+push backend image, build+push migrations image |
| `frontend` | Checkout, setup pnpm, setup Node 22, `pnpm install --frozen-lockfile`, typecheck shared package | + Docker Buildx, GHCR login, build+push frontend image with `VITE_API_URL` build arg |

**Key details**:

- Go version is read from `apps/backend/go.mod` (currently Go 1.25).
- pnpm lockfile is enforced (`--frozen-lockfile`).
- Docker layer caching uses GitHub Actions cache (`type=gha`).
- GHCR authentication uses `GITHUB_TOKEN` (no additional secrets needed for image push).

### What CI Does NOT Do

- **No automated deployment.** Images are pushed to GHCR but must be pulled and deployed
  manually via `docker stack deploy` or `docker service update`.
- **No frontend build verification on PR.** The frontend job typechecks the shared package
  but does not run `pnpm build` for the desktop app on PRs. Build failures would only surface
  on push to `main`.
- **No integration tests.** Only Go unit tests and TypeScript typecheck run in CI.
- **No linting.** There is no Go linter (golangci-lint) or JavaScript linter (ESLint)
  configured in CI.
- **No database migration validation.** SQL migration files are not tested against a real
  database in CI.

---

## 4. Docker Swarm Stack

### Stack Definition

**File**: `docker-stack.yml` (project root)

Deployed via:
```bash
docker stack deploy -c docker-stack.yml homelab-the-game
```

### Service Configuration

| Service | Image | Replicas | Update Strategy | Restart Policy | Networks | Placement |
|---|---|---|---|---|---|---|
| `traefik` | `traefik:v3.6` | 1 | -- | on-failure (5 attempts) | frontend | manager only |
| `migrations` | `ghcr.io/.../homelab-game-migrations:latest` | 1 | -- | on-failure (3 attempts) | backend | any |
| `backend` | `ghcr.io/.../homelab-game-backend:latest` | 2 | start-first, 1 at a time, 10s delay | on-failure (5 attempts) | backend + frontend | any |
| `frontend` | `ghcr.io/.../homelab-game-frontend:latest` | 1 | start-first, 1 at a time, 10s delay | any (always restart) | frontend | any |
| `db` | `timescale/timescaledb:latest-pg16` | 1 | -- | on-failure (5 attempts) | backend | manager only |
| `redis` | `redis:7-alpine` | 1 | -- | on-failure (5 attempts) | backend | manager only |

### Backend Specifics

- **2 replicas** behind Traefik with sticky sessions (cookie-based affinity:
  `_backend_affinity`). Sticky sessions ensure WebSocket connections stay on the same replica.
- **Rolling updates**: `start-first` order means a new replica is started before the old one
  is stopped, enabling zero-downtime deploys.
- **CORS middleware** configured at both the Traefik level (via deploy labels) and the Go
  application level (`middleware/cors.go`). The Traefik-level CORS allows
  `https://game.homelab.living`.
- **Healthcheck**: Docker Swarm uses the built-in `/healthcheck` binary. Traefik-level
  healthchecks were removed due to overlay network IP resolution issues (see commit `fe033ff`).

### Database

- **Persistent volume**: `pgdata` (local driver) mounted at `/var/lib/postgresql/data`.
- **Pinned to manager node** to ensure the volume is always accessible.
- **No backups configured.** There is no automated backup, pg_dump cron, or WAL archiving.

### Redis

- **Ephemeral**: No persistence (`--save ""`, `--appendonly no`).
- **128MB memory limit** with LRU eviction (`--maxmemory-policy allkeys-lru`).
- **Purpose**: Rate limit counters, WebSocket pub/sub broadcast, global donated CU cache,
  bitcoin price leader election.
- **Graceful degradation**: The backend starts and functions without Redis. Rate limiting
  falls back to in-memory (per-replica), WebSocket messages deliver locally only, CU cache
  uses local values, and bitcoin price leader election is disabled (all replicas update price).

### Secrets Management

Environment variables in `docker-stack.yml` reference shell variables (`${DB_PASSWORD}`,
`${JWT_SECRET}`) that must be set in the deploying shell. The Go config loader
(`internal/config/config.go`) also supports Docker secrets via `/run/secrets/<name>`, but the
stack file does not currently define any Swarm secrets -- it uses plain environment variables.

**Current secret surface**:
- `DB_PASSWORD` -- PostgreSQL password (env var)
- `JWT_SECRET` -- JWT signing key (env var). If unset, the server generates a random key and
  logs a warning; sessions do not persist across restarts.

---

## 5. Deployment Procedures

### Deploying a New Version (Swarm)

After a push to `main` triggers CI and images are pushed to GHCR:

```bash
# Pull latest images (Swarm does this on service update)
docker stack deploy -c docker-stack.yml homelab-the-game

# Or update individual services:
docker service update --image ghcr.io/monkay-homelab/homelab-game-backend:latest homelab-the-game_backend
docker service update --image ghcr.io/monkay-homelab/homelab-game-frontend:latest homelab-the-game_frontend
```

Rolling updates proceed with `start-first` order, meaning:
1. A new task (container) is started.
2. Once healthy (healthcheck passes), the old task is stopped.
3. Repeat for each replica.

There is a 10-second delay between replica updates. The backend's graceful shutdown handler
drains connections over a 10-second grace period on SIGTERM.

### Deploying Database Migrations

Migrations run automatically when the `migrations` service starts. On a fresh `docker stack
deploy`, migrations run before the backend can successfully connect (the backend will retry
on database connection failure via pgxpool).

For subsequent deploys, if new migration files are added:
1. A new migrations image must be built and pushed (happens automatically via CI on push to
   `main`).
2. The migrations service must be restarted or redeployed:
   ```bash
   docker service update --force homelab-the-game_migrations
   ```

**There is no rollback mechanism for migrations.** Migrations are forward-only. The
`schema_migrations` table tracks which numbered migrations have been applied.

### Running Without Swarm (Development)

Per CLAUDE.md, the backend can also run directly for development:
```bash
cd apps/backend
go run ./cmd/server/
```

This connects to the local PostgreSQL instance (configured via `.env` or defaults to
`localhost:5432`). The frontend dev server runs via:
```bash
cd apps/desktop
pnpm dev
```

---

## 6. Healthchecks

### Backend

- **Endpoint**: `GET /health` -- returns `{"status":"ok"}` with 200.
- **Implementation**: Hardcoded handler in `routes.go`. Does NOT check database connectivity,
  Redis availability, or any dependency health. It only confirms the HTTP server is accepting
  connections.
- **Healthcheck binary**: `cmd/healthcheck/main.go` -- a separate Go binary that makes an
  HTTP GET to `http://localhost:<PORT>/health`. This is used in the Dockerfile `HEALTHCHECK`
  directive and the Swarm service healthcheck.
- **Swarm healthcheck**: Runs every 10s, 3s timeout, 10s start period, 3 retries.

### Frontend

- **Swarm healthcheck**: `wget --spider -q http://127.0.0.1/` every 10s, 3s timeout, 5s
  start period, 3 retries.

### Database

- **Swarm healthcheck**: `pg_isready -U homelab_game -d homelab_game` every 10s, 3s timeout,
  30s start period, 5 retries.

### Redis

- **Swarm healthcheck**: `redis-cli ping` every 10s, 3s timeout, 5s start period, 3 retries.

### Gap: Dependency-Aware Health

The backend `/health` endpoint does not verify that it can actually serve requests (database
reachable, connection pool healthy). A degraded backend that has lost its database connection
will still report healthy and receive traffic from Traefik.

---

## 7. Logging

### Current State

The backend uses Go's standard `log` package (`log.Printf`, `log.Println`, `log.Fatalf`).
There is no structured logging library (no slog, zap, zerolog, or logrus). Log output is
plain text to stdout/stderr.

**What gets logged** (82 log statements across 10 files):

- Startup: database connection, Redis connection status, rate limit backend selection,
  WebSocket broadcasting mode, global CU cache initialization, bitcoin leader election status
- Runtime: WebSocket connect/disconnect, WebSocket send buffer drops, bitcoin leader
  acquisition/loss, CU cache refresh errors/slow queries, Redis pub/sub errors, rate limit
  Redis errors (fail-open)
- Shutdown: graceful shutdown initiation and errors
- Errors: generally logged at the point of occurrence with context (e.g., `[cu-cache]`,
  `[ws-pubsub]`, `[bitcoin-leader]` prefixes)

### What Is NOT Logged

- Request access logs (no HTTP request logging middleware)
- Request latency or response status codes
- User-identifying information in most log lines (WebSocket logs say "client connected" but
  not which user)
- Game action types or frequencies
- Error rates or aggregated metrics

### Log Access

In Docker Swarm, logs are accessible via:
```bash
docker service logs homelab-the-game_backend --follow
docker service logs homelab-the-game_frontend --follow
docker service logs homelab-the-game_db --follow
```

There is no log aggregation, log rotation policy, or log shipping. Docker's default logging
driver (json-file) is used with default settings (no max-size, no max-file).

---

## 8. Monitoring and Observability

### Current State: None

There is no monitoring, metrics collection, alerting, or dashboarding infrastructure.
Specifically:

- No Prometheus, Grafana, Datadog, or any metrics system
- No application-level metrics (request rates, error rates, latency percentiles)
- No infrastructure metrics collection beyond what Docker exposes natively
- No alerting for downtime, error spikes, or resource exhaustion
- No distributed tracing (OpenTelemetry, Jaeger, etc.)
- No uptime monitoring or synthetic checks

The only observability available is reading Docker service logs and checking Docker service
status:
```bash
docker service ls
docker service ps homelab-the-game_backend
```

### Stress Testing Tool

A load testing tool exists at `apps/stress-tests/` that can simulate multiple concurrent
players with WebSocket connections. It reports throughput (req/s), latency percentiles
(P50/P90/P95/P99), and error rates. This is a manual tool, not integrated into CI or run
on any schedule.

---

## 9. Graceful Shutdown and Connection Draining

The backend server (`cmd/server/main.go`) handles SIGTERM and SIGINT:

1. Signal received.
2. `http.Server.Shutdown()` called with a 10-second context timeout.
3. No new connections are accepted.
4. Existing HTTP requests complete (up to 10s).
5. WebSocket connections: the server closes, but there is no explicit WebSocket drain. Active
   WebSocket connections are terminated when the HTTP server shuts down.
6. Database pool is closed via `defer pool.Close()`.
7. Redis connection is closed via `defer rdb.Close()`.
8. Bitcoin price leader key is deleted from Redis on stop (`PriceLeader.Stop()`).

**Gap**: WebSocket clients are not gracefully disconnected with a close frame before shutdown.
They experience a connection drop and must reconnect to a surviving replica.

---

## 10. Horizontal Scaling

The backend is designed to run with 2+ replicas (currently `replicas: 2` in the stack).
The following mechanisms enable multi-replica correctness:

### State Synchronization via Redis

| Concern | Single-Replica Behavior | Multi-Replica Behavior |
|---|---|---|
| Rate limiting | In-memory map | Redis INCR + EXPIRE (atomic Lua script) |
| WebSocket broadcast | Direct hub delivery | Redis pub/sub (`ws:broadcast` channel) |
| Global donated CU cache | Local value + DB refresh | Redis GET/SET + local fallback |
| Bitcoin price leader | Always runs | Redis-based leader election (SET NX, 15s TTL) |

### Sticky Sessions

Traefik uses cookie-based sticky sessions (`_backend_affinity`) to route a user's requests to
the same replica. This is required because WebSocket connections are stateful and the per-user
game tick goroutine runs on the replica that holds the WebSocket connection.

### Database Row Locking

Concurrent game actions for the same user across replicas are serialized using PostgreSQL
advisory locks or row-level locking (see `_documents/tdd/horizontal-scaling.md` for the full
design).

### Graceful Degradation

If Redis becomes unavailable:
- Rate limiting falls back to in-memory (per-replica, less accurate but functional)
- WebSocket messages deliver locally only (users on other replicas miss updates until
  reconnect)
- CU cache serves stale local values
- Bitcoin price updates may run on multiple replicas simultaneously (harmless duplication)
- The backend logs warnings but continues serving traffic

---

## 11. Database Operations

### Migration System

Migrations are sequential numbered SQL files in
`apps/backend/internal/database/migrations/`. The migration runner
(`migrations-entrypoint.sh`) is a shell script that:

1. Reads `DB_PASSWORD` from environment or Docker secret file.
2. Waits for PostgreSQL to be ready (`pg_isready` loop).
3. Ensures TimescaleDB extension is created.
4. Creates `schema_migrations` tracking table if it does not exist.
5. For each `NNN_*.sql` file, checks if the version number is already in
   `schema_migrations`. If not, applies it via `psql -f` and records the version.

**Current migrations** (14 numbered files):
001 through 014, covering initial schema, progression system, SaaS, throttle fixes, colo
prestige, endgame, customer growth, global CU store, bitcoin trading, overclock mode,
research tree, rack optimization, and game state indexes.

**Non-migration scripts**: `wipe_player_progress.sql` is a manual one-off script for
resetting all player progress while preserving user accounts and groups.

### Migration Gaps

- **No rollback/down migrations.** Every migration is forward-only. Rolling back a bad
  migration requires manual SQL.
- **No CI validation.** Migrations are not tested against a real database in CI. A
  syntactically invalid migration would only fail at deploy time.
- **No migration locking.** If two migration containers start simultaneously, they could
  race. The `schema_migrations` check is not transactional with the migration application
  (separate `psql` invocations).
- **Migration 007 is missing.** The sequence goes 006, 008 -- this is harmless but notable.

### Database Connection Pool

The backend uses `pgx/v5` with a connection pool configured as:
- Max connections: 50
- Min connections: 5
- SSL mode: `disable` (configured via `DB_SSLMODE` env var, since traffic stays on the
  overlay network)

### Manual Database Access

```bash
# From the VM (outside Docker):
cat /path/to/migration.sql | sudo -u postgres psql -d homelab_game

# Grant permissions on new tables:
echo "GRANT ALL ON <table_name> TO homelab_game;" | sudo -u postgres psql -d homelab_game
```

Inside Docker Swarm, the database is only reachable from the `backend` overlay network.

---

## 12. Backup and Recovery

### Current State: No Automated Backups

There are no automated database backups, no pg_dump cron jobs, no WAL archiving, and no
point-in-time recovery capability. The PostgreSQL data lives on a Docker volume (`pgdata`)
on the single VM.

### Recovery Scenarios

| Scenario | Recovery Path |
|---|---|
| Bad migration applied | Manual SQL to reverse changes. No down migrations exist. |
| Data corruption | No backup to restore from. Data is lost. |
| VM disk failure | Complete data loss. PostgreSQL volume is not replicated. |
| Accidental data deletion | No backup to restore from. `wipe_player_progress.sql` is available but destroys all game progress. |
| Need to roll back a deploy | `docker service update --rollback` reverts the service image. Database changes are not rolled back. |

---

## 13. Security Operational Concerns

### Authentication

- JWT-based authentication with `HS256` signing.
- JWT secret is an environment variable. If unset, a random secret is generated at startup
  (sessions lost on restart).
- Password hashing uses `bcrypt` via `golang.org/x/crypto`.
- Registration can be disabled via `REGISTRATION_ENABLED=false` env var.

### Rate Limiting

- Auth endpoints: 10 requests/minute per IP.
- Game actions: 7,200 requests/minute per user (120/sec).
- Social endpoints: 180 requests/minute per user.
- Rate limiting fails open on Redis errors (allows the request).

### Request Limits

- Max request body size: 64KB (`MaxBodySize` middleware).
- WebSocket max incoming message size: 64KB (`SetReadLimit(65536)`).

### CORS

Allowed origins are hardcoded in two places:
1. `internal/api/middleware/cors.go` -- HTTP CORS middleware
2. `internal/api/ws/hub.go` -- WebSocket upgrader `CheckOrigin`

Both allow the production domains, dev domains, and (in non-production mode) localhost:3000.
Additional origins can be added via the `CORS_ORIGINS` environment variable (comma-separated).

### TLS

TLS is terminated at an external nginx reverse proxy, not on this VM. Traffic between the
external proxy and Traefik is unencrypted HTTP on port 80. Traffic within the Docker overlay
networks is unencrypted. The `DB_SSLMODE=disable` setting reflects this.

---

## 14. Rollback Procedures

### Service Rollback

Docker Swarm supports automatic rollback:
```bash
docker service update --rollback homelab-the-game_backend
docker service update --rollback homelab-the-game_frontend
```

This reverts to the previous image. It does NOT roll back database migrations.

### Database Rollback

There is no automated database rollback. If a migration causes issues:
1. Identify the problematic changes.
2. Write and apply corrective SQL manually.
3. The `schema_migrations` table will still show the migration as applied.

### Full Stack Rollback

```bash
# Remove the stack entirely:
docker stack rm homelab-the-game

# Redeploy with a specific image tag:
# Edit docker-stack.yml to use sha-<commit> tags instead of :latest
docker stack deploy -c docker-stack.yml homelab-the-game
```

The database volume (`pgdata`) persists across stack removals.

---

## 15. Operational Gaps Summary

This section catalogs known operational gaps for transparency and prioritization.

| Gap | Severity | Impact |
|---|---|---|
| No automated database backups | **Critical** | Single point of data loss |
| No monitoring or alerting | **High** | Outages are discovered by users, not operators |
| No access/request logging | **High** | Cannot diagnose request-level issues or detect abuse patterns |
| No structured logging | Medium | Log parsing is fragile; no log aggregation possible |
| No log rotation/size limits | Medium | Docker json-file logs grow unbounded on disk |
| Health endpoint does not check dependencies | Medium | Unhealthy backend (lost DB) still receives traffic |
| No CI linting | Medium | Code style issues caught only in review |
| No CI integration tests | Medium | Database-dependent code is untested in CI |
| No migration rollback mechanism | Medium | Forward-only migrations require manual fixes |
| No migration CI validation | Low | Bad SQL discovered only at deploy time |
| JWT secret management | Low | Manual env var; no rotation mechanism |
| No WebSocket graceful drain on shutdown | Low | Clients experience hard disconnect during deploys |
| No automated deploy trigger | Low | Manual `docker service update` after CI |
| Missing migration 007 | Informational | Harmless gap in numbering |

---

## 16. Related Documents

- **TDD: Horizontal Scaling** (`_documents/tdd/horizontal-scaling.md`) -- Detailed design for
  multi-replica backend, Redis state externalization, and Traefik ingress.
- **TDD: Kubernetes Migration** (`_documents/tdd/migrate-kubernetes.md`) -- Future migration
  plan from Docker Swarm to k3s. Currently in draft status; Docker Swarm is the production
  orchestrator.
- **CLAUDE.md** (project root) -- Developer quick-reference for commands, architecture,
  and deployment.
