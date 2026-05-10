---
project: "homelab-the-game"
maturity: "draft"
last_updated: "2026-03-24"
updated_by: "@staff-engineer"
scope: "Migration of Homelab the Game from single-VM deployment to Kubernetes (k3s) with Helm, GHCR, and GitHub Actions CI/CD"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/operations.md
---

# TDD: Kubernetes Migration

## 1. Problem Statement

### What

Migrate the Homelab the Game stack -- Go backend, React frontend, and PostgreSQL/TimescaleDB
database -- from manual single-VM deployment to Kubernetes (k3s), with container images pushed
to GitHub Container Registry (GHCR) and a GitHub Actions CI/CD pipeline for build and push.

### Why Now

The current deployment model has no automation, no rollback capability, no health-check-driven
restarts, and no reproducible builds. CLAUDE.md documents the deploy process as "kill the old
process and start a new one." The operations spec lists 15 operational gaps, nearly all of which
are addressed or significantly improved by containerization and orchestration. Kubernetes also
provides the foundation for future horizontal scaling, automated database backups, and
observability tooling.

### Constraints

- **No cluster exists yet.** This TDD produces artifacts only: Dockerfiles, Helm chart,
  GitHub Actions workflow. The actual cluster provisioning and deployment step are deferred.
- **Homelab-grade infrastructure.** k3s on a single node (or small cluster). No cloud managed
  services, no multi-region, no service mesh. Keep it simple.
- **Zero downtime is not required** during the initial migration. Brief maintenance windows are
  acceptable. Zero downtime becomes a goal post-migration via rolling updates.
- **Database state must be preserved.** Player data in PostgreSQL must survive the migration
  with zero loss. This is the single non-negotiable.
- **The game is live** at `game.homelab.living` / `api.homelab.living`. Migration must have a
  clear cutover plan and rollback path.

### Acceptance Criteria

1. Multi-stage Dockerfiles produce minimal, reproducible images for both backend and frontend.
2. A Helm chart defines all Kubernetes resources: Deployments, Services, ConfigMap, Secret,
   PVC, Ingress, and a database migration Job.
3. A GitHub Actions workflow builds both images and pushes to `ghcr.io/<owner>/homelab-game-*`.
4. PostgreSQL + TimescaleDB runs as a StatefulSet with persistent volume claims.
5. Database migrations run as a Kubernetes Job before the backend starts.
6. WebSocket connections work through the ingress controller.
7. All secrets (DB password, JWT secret) are managed via Kubernetes Secrets, not baked into images.
8. Each phase is independently verifiable -- partial completion does not break the existing deployment.

---

## 2. Context and Prior Art

### 2.1 Current Architecture (Source of Truth)

The system runs on a single VM with the following topology:

```
Internet
  |
  v
nginx (external, NOT on this VM)
  |---> api.homelab.living --> :8080 (Go backend, started via `go run`)
  |---> game.homelab.living --> :3000 (Vite dev server or static build)
  |
[This VM]
  |---> PostgreSQL 16 + TimescaleDB on localhost:5432
        DB: homelab_game, User: homelab_game
        14 migration files (001 through 014, plus wipe script)
```

**Backend configuration** (`internal/config/config.go`): Reads `PORT`, `DB_HOST`, `DB_PORT`,
`DB_USER`, `DB_PASSWORD`, `DB_NAME`, `JWT_SECRET` from environment variables. Has a custom
`.env` file loader in `main.go`. All defaults assume localhost deployment.

**Frontend configuration** (`apps/desktop/src/api.ts`, `wsClient.ts`): API URL from
`VITE_API_URL` env var (build-time), defaults to `https://api.homelab.living`. WebSocket
URL derived by protocol swap (`https://` to `wss://`).

**Database**: pgx/v5 connection pool (max 50 conns, min 5 -- note: `db.go` has 50/5,
architecture spec says 20/2; code is authoritative). Connection string built as
`postgres://user:pass@host:port/db?sslmode=disable`.

**In-memory state**: Rate limiter, per-user action mutexes, WebSocket hub. All lost on restart.
This is an existing constraint documented in the architecture spec. Kubernetes does not change
this -- it remains a single-replica backend.

**Hardcoded origins**: Both `middleware/cors.go` and `ws/hub.go` have origin allowlists.
CORS middleware reads `CORS_ORIGINS` env var and `ENV` var. WebSocket upgrader is fully
hardcoded and does NOT read from env. This must be addressed during containerization.

### 2.2 Files and Modules Requiring Changes

| File/Area | Change Required | Risk |
|-----------|----------------|------|
| `apps/backend/cmd/server/main.go` | Remove `.env` file loader for container deploy (env injected by K8s). Or: keep it as fallback, no change needed if env vars are set. | Low -- no code change needed |
| `apps/backend/internal/config/config.go` | No changes. Already reads all config from env vars with sensible defaults. | None |
| `apps/backend/internal/database/db.go` | No changes. Accepts a connection URL string. | None |
| `apps/backend/internal/api/ws/hub.go` | **Should be changed**: WebSocket origin allowlist is hardcoded. Should read from `CORS_ORIGINS` or `WS_ORIGINS` env var, or at minimum add the new cluster-internal origins. | Medium -- functional correctness |
| `apps/backend/internal/api/middleware/cors.go` | May need additional origins if the ingress presents different `Origin` headers. Existing `CORS_ORIGINS` env var mechanism is sufficient. | Low |
| `apps/backend/.env` | Not deployed to containers. Secrets provided via K8s Secret. | None (file stays for local dev) |
| `apps/desktop/vite.config.ts` | No changes for the Docker build. `VITE_API_URL` set at build time. | None |
| `apps/desktop/src/api.ts` | No changes. Already uses `VITE_API_URL`. | None |
| `apps/desktop/src/wsClient.ts` | No changes. Derives WS URL from `VITE_API_URL`. | None |
| (new) `apps/backend/Dockerfile` | Create multi-stage Go build. | New file |
| (new) `apps/desktop/Dockerfile` | Create multi-stage Node build + nginx static serve. | New file |
| (new) `helm/homelab-game/` | Full Helm chart. | New directory tree |
| (new) `.github/workflows/build.yml` | CI/CD pipeline. | New file |
| (new) `.dockerignore` (x2) | Exclude unnecessary files from build context. | New files |
| `apps/backend/internal/database/migrations/*.sql` | No changes to SQL. Migration Job runs them in order. | None |

**Key finding: The backend requires zero code changes for containerization.** The config layer
already reads from env vars, the database layer accepts a URL string, and the `.env` loader is
a no-op when env vars are already set. The only recommended (not required) code change is making
the WebSocket origin allowlist configurable.

### 2.3 How Others Solve This

- **k3s default ingress**: Traefik is the default k3s ingress controller. However, nginx
  ingress controller is more widely documented for WebSocket support and aligns with the
  existing external nginx proxy. We use nginx ingress.
- **TimescaleDB on Kubernetes**: The TimescaleDB Helm chart and the `timescale/timescaledb-ha`
  image are the standard approach. For a homelab, a simple PostgreSQL StatefulSet with the
  TimescaleDB extension is sufficient and avoids operator complexity.
- **Database migrations in K8s**: Kubernetes Jobs with `initContainers` or standalone Jobs
  with `helm.sh/hook` annotations are the standard pattern. We use a pre-install/pre-upgrade
  Helm hook Job.
- **Monorepo CI/CD**: GitHub Actions with path filters (`paths: apps/backend/**`) to build
  only changed components. Matrix builds for parallel image creation.

---

## 3. Alternatives Considered

### Alternative A: Docker Compose (No Kubernetes)

**Approach**: Write `docker-compose.yml` with services for backend, frontend, and PostgreSQL.
Deploy with `docker compose up -d`.

**Strengths**:
- Simpler than Kubernetes for a single-node deployment
- Lower learning curve
- Faster to implement (1-2 days vs. 3-5)

**Weaknesses**:
- No built-in health checks with auto-restart (requires `restart: always` which is blunt)
- No rolling updates -- `docker compose up -d` recreates containers
- No declarative secret management
- No Ingress abstraction -- still need manual nginx configuration
- Dead end for scaling: does not grow into multi-node without a rewrite

**Verdict**: Reasonable for a project that will never leave a single node. But k3s is
lightweight enough for homelab use and provides a growth path. The marginal complexity of
Helm over Compose is offset by the capabilities gained.

### Alternative B: Full Kubernetes with Operator-managed PostgreSQL

**Approach**: Use CloudNativePG or Zalando Postgres Operator for the database, with automated
failover, backup, and point-in-time recovery.

**Strengths**:
- Automated backups and WAL archiving
- Failover without manual intervention
- Connection pooling via PgBouncer sidecar

**Weaknesses**:
- Operator installation adds significant complexity to a homelab k3s cluster
- Resource overhead (operator pod, additional replicas) on limited hardware
- Over-engineered for a single-node deployment with one database
- Debugging operator behavior requires deep Kubernetes knowledge

**Verdict**: Excellent for production at scale. Over-engineered for homelab. We can migrate
to an operator later without changing the application -- the backend just needs a connection
string. The StatefulSet approach is the right starting point.

### Alternative C (Recommended): Kubernetes with Simple StatefulSet + Helm

**Approach**: Helm chart with manual PostgreSQL StatefulSet, nginx ingress, Kubernetes Secrets.
Operators and service mesh deferred to when they are needed.

**Strengths**:
- Right-sized for homelab k3s
- Helm chart is portable -- works on k3s, k8s, EKS, GKE
- StatefulSet with PVC protects data without operator complexity
- Growth path: add operators, HPA, service mesh incrementally
- Migration Job handles schema versioning

**Weaknesses**:
- No automated database failover (acceptable for single-node)
- No automated backups (should add a CronJob for pg_dump, documented as follow-up)
- Manual effort for TimescaleDB extension setup in container

**Verdict**: Best balance of capability vs. complexity for the current project stage.

---

## 4. Architecture and System Design

### 4.1 Target Kubernetes Topology

```
Internet
  |
  v
nginx Ingress Controller (in-cluster, replaces external nginx)
  |
  |---> Ingress: api.homelab.living
  |       |
  |       v
  |     Service: homelab-backend (ClusterIP)
  |       |
  |       v
  |     Deployment: homelab-backend (1 replica)
  |       - Container: backend (Go binary)
  |       - Env from: ConfigMap + Secret
  |       - Probes: /health
  |
  |---> Ingress: game.homelab.living
          |
          v
        Service: homelab-frontend (ClusterIP)
          |
          v
        Deployment: homelab-frontend (1 replica)
          - Container: frontend (nginx serving static files)

StatefulSet: homelab-postgresql (1 replica)
  - Container: timescale/timescaledb-ha:pg16
  - PVC: 10Gi (adjustable via values.yaml)
  - Service: homelab-postgresql (ClusterIP, headless)

Job (Helm hook: pre-install, pre-upgrade): homelab-migrate
  - Container: backend image
  - Command: run migrations in order against DB
  - Runs before backend Deployment rolls out
```

### 4.2 Networking

| Source | Destination | Protocol | Path |
|--------|------------|----------|------|
| Client browser | Ingress | HTTPS (TLS terminated at ingress) | `api.homelab.living/*` |
| Client browser | Ingress | WSS (TLS terminated at ingress) | `api.homelab.living/ws` |
| Client browser | Ingress | HTTPS | `game.homelab.living/*` |
| Ingress | Backend Service | HTTP :8080 | Proxied |
| Ingress | Frontend Service | HTTP :80 | Proxied |
| Backend pod | PostgreSQL Service | TCP :5432 | Cluster-internal |

**WebSocket through Ingress**: The nginx ingress controller requires specific annotations
for WebSocket support:

```yaml
nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
nginx.org/websocket-services: "homelab-backend"
```

The backend's existing gorilla/websocket ping/pong (30s/45s) keeps connections alive through
the proxy, which is already designed for this (noted in the code comments).

### 4.3 Configuration Flow

```
values.yaml (user-configurable)
  |
  +--> ConfigMap: homelab-config
  |      PORT=8080
  |      DB_HOST=homelab-postgresql
  |      DB_PORT=5432
  |      DB_NAME=homelab_game
  |      DB_USER=homelab_game
  |      ENV=production
  |      CORS_ORIGINS=https://game.homelab.living
  |
  +--> Secret: homelab-secrets
         DB_PASSWORD=<base64>
         JWT_SECRET=<base64>
```

The backend's existing `config.Load()` reads these env vars directly -- no code changes needed.

### 4.4 Component Boundaries

```
+-------------------+     +-------------------+     +-------------------+
|  Frontend Image   |     |  Backend Image    |     |  PostgreSQL Image |
|  (nginx + static) |     |  (Go binary)      |     |  (timescaledb-ha) |
|                   |     |                   |     |                   |
|  Build-time:      |     |  Runtime:         |     |  Runtime:         |
|  VITE_API_URL     |     |  PORT             |     |  POSTGRES_DB      |
|                   |     |  DB_HOST/PORT/... |     |  POSTGRES_USER    |
|  Runtime:         |     |  JWT_SECRET       |     |  POSTGRES_PASSWORD|
|  (none - static)  |     |  ENV              |     |                   |
+-------------------+     |  CORS_ORIGINS     |     |  PVC: /var/lib/   |
                          +-------------------+     |  postgresql/data  |
                                                    +-------------------+
```

---

## 5. Data Models and Storage

### 5.1 Persistent Volume Requirements

| Component | PVC Size | Access Mode | Storage Class | Data |
|-----------|----------|-------------|--------------|------|
| PostgreSQL | 10Gi (default, configurable) | ReadWriteOnce | local-path (k3s default) | All game data |

**Data lifecycle**: Player data is the only stateful component. The backend and frontend are
stateless (in-memory rate limiter and mutexes are ephemeral by design). PostgreSQL data must
survive pod restarts, node reboots, and upgrades.

### 5.2 Migration Strategy (Database Schema)

The project has 14 numbered SQL migration files plus a wipe script. There is no migration
framework or version tracking. The migration Job must:

1. Track which migrations have been applied (introduce a `schema_migrations` table).
2. Run pending migrations in numeric order.
3. Be idempotent (safe to re-run).

**Migration Job approach** -- a shell script baked into the backend image:

```bash
#!/bin/sh
# migrate.sh -- runs pending SQL migrations against PostgreSQL
set -e

MIGRATIONS_DIR="/app/migrations"
DB_URL="postgres://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

# Create tracking table if not exists
psql "$DB_URL" -c "
  CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
"

# Apply pending migrations in order
for f in $(ls "$MIGRATIONS_DIR"/[0-9]*.sql | sort); do
  version=$(basename "$f")
  already=$(psql "$DB_URL" -tAc "SELECT 1 FROM schema_migrations WHERE version='$version'")
  if [ "$already" != "1" ]; then
    echo "Applying $version..."
    psql "$DB_URL" -f "$f"
    psql "$DB_URL" -c "INSERT INTO schema_migrations (version) VALUES ('$version')"
    echo "Applied $version"
  else
    echo "Skipping $version (already applied)"
  fi
done

echo "All migrations applied."
```

**Note**: The `wipe_player_progress.sql` file is excluded from the migration Job (it does not
match the `[0-9]*.sql` glob pattern by design).

### 5.3 Initial Data Migration (VM to Kubernetes)

When the cluster is ready and the migration is performed for real, the database must be moved:

1. `pg_dump` from the current VM's PostgreSQL.
2. Restore into the Kubernetes PostgreSQL StatefulSet pod.
3. Run the migration Job to create `schema_migrations` and backfill records for all 14 existing
   migrations (since they were already applied on the VM).

This is a cutover operation documented in Phase 4, not automated by the Helm chart.

---

## 6. API Contracts

No API changes. The backend API surface, request/response schemas, and WebSocket protocol
remain identical. The only client-visible change is that TLS termination moves from the
external nginx proxy to the Kubernetes ingress controller.

The frontend build uses `VITE_API_URL=https://api.homelab.living` at build time, and the
client connects to the same domains. DNS records remain the same; they just point to the
k3s node's IP instead of the VM's IP (or the same IP if k3s runs on the same machine).

---

## 7. Migration and Rollout

### 7.1 Phased Rollout Plan

Each phase is independently verifiable. No phase depends on the cluster existing.

#### Phase 1: Dockerfiles and .dockerignore

**Goal**: Produce container images that can be built and run locally.

**Backend Dockerfile** (`apps/backend/Dockerfile`):

```dockerfile
# Stage 1: Build
FROM golang:1.25-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server/

# Stage 2: Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates postgresql-client
WORKDIR /app
COPY --from=builder /server .
COPY internal/database/migrations/ ./migrations/
COPY migrate.sh .
RUN chmod +x migrate.sh
EXPOSE 8080
CMD ["./server"]
```

Key decisions:
- `CGO_ENABLED=0` for fully static binary (no glibc dependency).
- `alpine` runtime for small image (~15MB base). `ca-certificates` for potential HTTPS
  outbound calls. `postgresql-client` for the migration script's `psql` usage.
- Migrations are copied into the image so the migration Job can use the same image.
- Multi-stage build keeps the final image small (no Go toolchain).

**Frontend Dockerfile** (`apps/desktop/Dockerfile`):

```dockerfile
# Stage 1: Build
FROM node:22-alpine AS builder
RUN corepack enable
WORKDIR /build

# Install dependencies (monorepo-aware)
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/shared/package.json packages/shared/
COPY apps/desktop/package.json apps/desktop/
RUN pnpm install --frozen-lockfile

# Copy source and build
COPY packages/shared/ packages/shared/
COPY apps/desktop/ apps/desktop/
ARG VITE_API_URL=https://api.homelab.living
ENV VITE_API_URL=$VITE_API_URL
RUN pnpm --filter @homelab-game/desktop build

# Stage 2: Serve
FROM nginx:1.27-alpine
COPY --from=builder /build/apps/desktop/dist /usr/share/nginx/html
COPY apps/desktop/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
```

Key decisions:
- pnpm workspace requires the root `package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`,
  and the `packages/shared` directory to resolve the `workspace:*` dependency.
- `VITE_API_URL` is a build-time `ARG` because Vite inlines `import.meta.env` at build time.
  The image is baked to a specific API URL. To change it, rebuild the image.
- nginx serves the static SPA with `try_files $uri $uri/ /index.html` for client-side routing.

**Frontend nginx config** (`apps/desktop/nginx.conf`):

```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }

    # Cache static assets aggressively
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }
}
```

**.dockerignore files** (one per app directory):

`apps/backend/.dockerignore`:
```
.env
*.test.go
*_test.go
stress-tests/
docs/
.git/
```

`apps/desktop/.dockerignore`:
```
node_modules/
dist/
src-tauri/
.git/
```

**Verification**: Build both images locally with `docker build`, run them with
`docker run -p 8080:8080` / `docker run -p 3000:80`, confirm the backend responds
to `/health` and the frontend loads.

**Complexity**: Small

---

#### Phase 2: Helm Chart

**Goal**: A Helm chart that defines all Kubernetes resources.

**Chart structure**:

```
helm/homelab-game/
  Chart.yaml
  values.yaml
  templates/
    _helpers.tpl
    backend-deployment.yaml
    backend-service.yaml
    frontend-deployment.yaml
    frontend-service.yaml
    postgresql-statefulset.yaml
    postgresql-service.yaml
    postgresql-pvc.yaml
    configmap.yaml
    secret.yaml
    ingress.yaml
    migration-job.yaml
    NOTES.txt
```

**`values.yaml`** (key sections):

```yaml
# -- Global
nameOverride: ""
fullnameOverride: ""

# -- Backend
backend:
  image:
    repository: ghcr.io/<owner>/homelab-game-backend
    tag: latest
    pullPolicy: IfNotPresent
  replicas: 1
  port: 8080
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi
  env:
    ENV: production
    CORS_ORIGINS: "https://game.homelab.living"

# -- Frontend
frontend:
  image:
    repository: ghcr.io/<owner>/homelab-game-frontend
    tag: latest
    pullPolicy: IfNotPresent
  replicas: 1
  port: 80
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi

# -- PostgreSQL
postgresql:
  image:
    repository: timescale/timescaledb-ha
    tag: pg16-ts2.17
    pullPolicy: IfNotPresent
  storage:
    size: 10Gi
    storageClass: ""  # empty = cluster default (local-path on k3s)
  port: 5432
  database: homelab_game
  user: homelab_game
  resources:
    requests:
      cpu: 200m
      memory: 256Mi
    limits:
      cpu: "1"
      memory: 1Gi

# -- Secrets (override at install time, NEVER commit real values)
secrets:
  dbPassword: "CHANGEME"
  jwtSecret: "CHANGEME"

# -- Ingress
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
  hosts:
    api:
      host: api.homelab.living
      paths:
        - path: /
          pathType: Prefix
    frontend:
      host: game.homelab.living
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: homelab-game-tls
      hosts:
        - api.homelab.living
        - game.homelab.living

# -- Migration Job
migration:
  enabled: true
```

**Key templates (design notes)**:

*Backend Deployment*:
- Single replica (in-memory state is not shareable).
- Liveness probe: `httpGet /health` every 15s, failure threshold 3.
- Readiness probe: `httpGet /health` every 5s, failure threshold 3.
- `envFrom` references both ConfigMap and Secret.
- `DB_HOST` set to the PostgreSQL Service name (`{{ .Release.Name }}-postgresql`).

*PostgreSQL StatefulSet*:
- Single replica with `ReadWriteOnce` PVC.
- `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` from Secret.
- TimescaleDB is included in the `timescale/timescaledb-ha` image by default.
- Liveness probe: `exec pg_isready -U $POSTGRES_USER` every 10s.
- Data mount at `/var/lib/postgresql/data`.

*Migration Job*:
- Helm hook: `pre-install,pre-upgrade`
- Hook weight: `-1` (runs before other hooks)
- Hook delete policy: `before-hook-creation` (cleans up previous Job on re-run)
- Uses the backend image with command override: `["./migrate.sh"]`
- Same env vars as the backend (ConfigMap + Secret)
- `restartPolicy: Never`, `backoffLimit: 3`

*Ingress*:
- Two host rules: `api.homelab.living` -> backend Service, `game.homelab.living` -> frontend Service
- TLS via cert-manager (optional, can be disabled)
- WebSocket annotations for the `/ws` path

**Verification**: `helm template ./helm/homelab-game` produces valid YAML. `helm lint` passes.
Can be dry-run installed: `helm install --dry-run homelab ./helm/homelab-game`.

**Complexity**: Medium

---

#### Phase 3: GitHub Actions CI/CD

**Goal**: Automated image builds on push, images pushed to GHCR.

**Workflow file** (`.github/workflows/build.yml`):

```yaml
name: Build and Push Images

on:
  push:
    branches: [main]
    paths:
      - 'apps/backend/**'
      - 'apps/desktop/**'
      - 'packages/shared/**'
      - 'helm/**'
      - '.github/workflows/build.yml'
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  BACKEND_IMAGE: ghcr.io/${{ github.repository_owner }}/homelab-game-backend
  FRONTEND_IMAGE: ghcr.io/${{ github.repository_owner }}/homelab-game-frontend

jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      backend: ${{ steps.filter.outputs.backend }}
      frontend: ${{ steps.filter.outputs.frontend }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            backend:
              - 'apps/backend/**'
            frontend:
              - 'apps/desktop/**'
              - 'packages/shared/**'

  build-backend:
    needs: changes
    if: needs.changes.outputs.backend == 'true' || github.event_name == 'push'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ${{ env.BACKEND_IMAGE }}
          tags: |
            type=sha,prefix=
            type=raw,value=latest,enable={{is_default_branch}}
      - uses: docker/build-push-action@v6
        with:
          context: ./apps/backend
          push: ${{ github.event_name == 'push' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}

  build-frontend:
    needs: changes
    if: needs.changes.outputs.frontend == 'true' || github.event_name == 'push'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/metadata-action@v5
        id: meta
        with:
          images: ${{ env.FRONTEND_IMAGE }}
          tags: |
            type=sha,prefix=
            type=raw,value=latest,enable={{is_default_branch}}
      - uses: docker/build-push-action@v6
        with:
          context: .
          file: ./apps/desktop/Dockerfile
          push: ${{ github.event_name == 'push' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VITE_API_URL=https://api.homelab.living

  helm-lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: azure/setup-helm@v4
      - run: helm lint ./helm/homelab-game
```

Key decisions:
- Path-based filtering: only build what changed. Frontend rebuild includes `packages/shared/**`
  because the desktop app depends on the shared workspace package.
- Image tags: Git SHA for traceability + `latest` on main branch.
- Pull requests build but do not push (validates the Dockerfile).
- Frontend Dockerfile `context` is the repo root (needs `packages/shared/` and workspace files).
  Backend Dockerfile context is `apps/backend/` only.
- No deploy step. Deployment is deferred until the cluster exists.
- Helm lint as a separate job validates chart changes.

**Verification**: Push to a branch, confirm Actions run. Check GHCR for images.

**Complexity**: Small

---

#### Phase 4: Database Strategy

**Goal**: PostgreSQL + TimescaleDB running in Kubernetes with data preserved from the VM.

**StatefulSet details**:

The `timescale/timescaledb-ha:pg16-ts2.17` image includes:
- PostgreSQL 16
- TimescaleDB extension (auto-loaded)
- `pg_isready` for health checks
- Standard PostgreSQL initialization (`/docker-entrypoint-initdb.d/`)

**Initialization**: On first boot, PostgreSQL creates the database and user from the
`POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD` env vars. The migration Job then
runs all 14 migration files to create the schema.

**Data migration from VM** (one-time cutover procedure):

```bash
# 1. On the current VM: dump the database
pg_dump -U homelab_game -d homelab_game -Fc -f /tmp/homelab_game.dump

# 2. Copy dump to k3s node
scp /tmp/homelab_game.dump k3s-node:/tmp/

# 3. Copy dump into the PostgreSQL pod
kubectl cp /tmp/homelab_game.dump homelab-postgresql-0:/tmp/homelab_game.dump

# 4. Restore into the pod's database
kubectl exec -it homelab-postgresql-0 -- pg_restore \
  -U homelab_game -d homelab_game --no-owner --clean --if-exists \
  /tmp/homelab_game.dump

# 5. Backfill schema_migrations so the migration Job knows all 14 are applied
kubectl exec -it homelab-postgresql-0 -- psql -U homelab_game -d homelab_game -c "
  CREATE TABLE IF NOT EXISTS schema_migrations (
    version TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  INSERT INTO schema_migrations (version) VALUES
    ('001_initial_schema.sql'),
    ('002_progression_system.sql'),
    ('003_saas_system.sql'),
    ('004_throttle_and_fixes.sql'),
    ('005_colo_prestige.sql'),
    ('006_endgame.sql'),
    ('008_add_customer_growth_timestamp.sql'),
    ('009_global_cu_store.sql'),
    ('010_bitcoin_trading.sql'),
    ('011_overclock_mode.sql'),
    ('012_research_tree.sql'),
    ('013_rack_optimization.sql'),
    ('014_add_game_state_indexes.sql')
  ON CONFLICT DO NOTHING;
"

# 6. Verify
kubectl exec -it homelab-postgresql-0 -- psql -U homelab_game -d homelab_game \
  -c "SELECT count(*) FROM users; SELECT count(*) FROM game_states;"
```

**TimescaleDB extension**: The `timescaledb-ha` image pre-loads the extension. The migration
`001_initial_schema.sql` calls `SELECT create_hypertable(...)` which requires the extension.
If restoring via `pg_dump`, the extension and hypertables are included in the dump. For a
fresh install (empty DB + migration Job), the extension must be created first:

```sql
-- Added to migrate.sh before running numbered migrations:
CREATE EXTENSION IF NOT EXISTS timescaledb;
```

**Complexity**: Medium (the cutover procedure requires care)

---

### 7.2 Rollback Strategy Per Phase

| Phase | Rollback Procedure | Impact |
|-------|-------------------|--------|
| Phase 1 (Dockerfiles) | Delete the Dockerfiles. The existing VM deployment continues unchanged. | Zero -- Dockerfiles are additive, not destructive. |
| Phase 2 (Helm chart) | Delete the `helm/` directory. No cluster exists to clean up. | Zero -- chart is artifacts only. |
| Phase 3 (GitHub Actions) | Delete `.github/workflows/build.yml`. Images in GHCR can be deleted via the GitHub UI. | Zero -- workflow is additive. |
| Phase 4 (Database migration) | **Before cutover**: The VM database is untouched. Simply do not cut over. **After cutover**: The `pg_dump` from step 1 is the rollback artifact. Restore it to the VM's PostgreSQL and point DNS back. | Low if the dump was taken correctly. The only risk window is new data written to K8s DB after cutover but before a rollback decision. Minimize by cutting over during a maintenance window. |
| **Full rollback** | Point DNS (`api.homelab.living`, `game.homelab.living`) back to the VM. Start the backend process manually as before. Restore the `pg_dump` if the database was migrated. | The game was running on the VM for months; returning to that state is trivial. |

### 7.3 Cutover Sequence

1. Maintenance window announced to users (in-game banner or brief downtime).
2. Stop the backend on the VM (`lsof -ti:8080 | xargs kill -9`).
3. Take `pg_dump` of the live database (this is the rollback safety net).
4. Deploy the Helm chart to k3s (images already built and pushed).
5. Restore the `pg_dump` into the Kubernetes PostgreSQL pod (see 5.3 above).
6. Run the migration Job (backfills `schema_migrations`).
7. Smoke test: hit `api.homelab.living/health` via the k3s ingress.
8. Update DNS to point to the k3s node.
9. Verify frontend loads, login works, game state is intact, WebSocket connects.
10. If anything fails: restore DNS to VM, restart the backend process, restore the dump if needed.

---

## 8. Risks and Open Questions

### 8.1 Known Risks

| Risk | Severity | Mitigation |
|------|----------|------------|
| **Data loss during cutover** | Critical | `pg_dump` taken before any migration. Cutover during maintenance window. Verify row counts after restore. |
| **WebSocket breaking through ingress** | High | nginx ingress annotations for timeout and upgrade. Test with a real WebSocket client before DNS cutover. The backend's existing ping/pong mechanism (30s interval) is designed to survive proxies. |
| **TimescaleDB extension not available in container** | Medium | Use `timescale/timescaledb-ha` image which bundles the extension. Verify `CREATE EXTENSION IF NOT EXISTS timescaledb` succeeds during migration. |
| **Frontend baked API URL** | Medium | `VITE_API_URL` is a build-time arg. If the domain changes, the image must be rebuilt. This is intentional (static assets are immutable). Document clearly. |
| **Single-replica backend loses in-memory state on restart** | Low | This is the existing behavior. Kubernetes restart is equivalent to the current `kill -9` deploy. Rate limiter resets, mutexes reset, WebSocket connections drop and auto-reconnect. No regression. |
| **PVC data loss on node failure** | Medium | `local-path` storage class on k3s stores data on the node's disk. Node disk failure = data loss (same as current VM). Mitigation: add a CronJob for `pg_dump` backups (follow-up work, not in this TDD). |

### 8.2 Open Questions

1. **Will k3s run on the same VM or a different machine?** If same VM, the migration is
   simpler (no data transfer, PostgreSQL can stay external to the cluster initially). If
   different machine, the full cutover procedure applies.

2. **TLS certificate management**: The Helm chart includes cert-manager annotations. Is
   cert-manager already planned for the cluster, or should we use a different TLS strategy
   (e.g., Traefik's built-in ACME, or manual certs)?

3. **Container registry namespace**: The workflow uses `ghcr.io/${{ github.repository_owner }}`.
   What is the GitHub org/user that owns the repository? This determines the GHCR path.

4. **Resource limits**: The `values.yaml` includes initial guesses for CPU/memory requests and
   limits. These should be tuned after running the stress test tool against the containerized
   backend to observe actual resource usage.

5. **WebSocket origin allowlist in hub.go**: The hardcoded list in `ws/hub.go` should ideally
   be made configurable via env var to match the CORS middleware pattern. Is this change in
   scope, or should it be a separate issue?

### 8.3 Assumptions

- The k3s cluster will have at least 2 CPU cores and 4GB RAM available for workloads.
- DNS for `api.homelab.living` and `game.homelab.living` can be updated to point to the
  k3s node's IP.
- The GitHub repository has GitHub Actions enabled and the `packages: write` permission
  is available for GHCR pushes.
- The existing external nginx proxy will be decommissioned after the Kubernetes ingress
  controller takes over its role.

---

## 9. Testing Strategy

### 9.1 Per-Phase Verification

| Phase | Test | Pass Criteria |
|-------|------|---------------|
| Phase 1 | `docker build -t backend ./apps/backend` | Image builds successfully, < 50MB |
| Phase 1 | `docker run -p 8080:8080 -e DB_HOST=host.docker.internal ... backend` + `curl localhost:8080/health` | Returns `{"status":"ok"}` |
| Phase 1 | `docker build -t frontend -f apps/desktop/Dockerfile .` | Image builds successfully, < 30MB |
| Phase 1 | `docker run -p 3000:80 frontend` + open browser | Game UI loads, shows login screen |
| Phase 2 | `helm lint ./helm/homelab-game` | No errors |
| Phase 2 | `helm template homelab ./helm/homelab-game` | Valid YAML, all resources present |
| Phase 3 | Push to PR branch, check GitHub Actions | Both build jobs succeed, no push (PR) |
| Phase 3 | Merge to main, check GHCR | Images appear with SHA tag and `latest` |
| Phase 4 | `kubectl exec postgresql-0 -- psql -c "SELECT count(*) FROM users"` | Matches pre-migration count |
| Phase 4 | `kubectl exec postgresql-0 -- psql -c "SELECT * FROM schema_migrations"` | All 14 migrations listed |

### 9.2 Integration Testing (Post-Cutover)

- Register a new user, verify game state initializes.
- Perform game actions (buy hardware, run job, upgrade tier).
- Verify WebSocket connects and receives state pushes.
- Verify existing users can log in and their state is preserved.
- Run the stress test tool against the Kubernetes endpoint.
- Verify the migration Job is idempotent (run `helm upgrade` twice, no errors).

### 9.3 Performance Baseline

Before and after cutover, run the stress test tool with identical parameters:

```bash
./stresstest -url https://api.homelab.living -players 100 -duration 2m -rate 500ms
```

Compare P50/P90/P99 latencies. Containerization should not introduce significant latency
(expected: < 5ms increase at P99 from network hop through ingress).

---

## 10. Observability and Operational Readiness

### 10.1 Health Signals

| Signal | Source | How to Check |
|--------|--------|-------------|
| Backend up | Liveness probe (`/health`) | `kubectl get pods` -- READY 1/1 |
| Backend ready | Readiness probe (`/health`) | Service receives traffic only when ready |
| PostgreSQL up | `pg_isready` exec probe | `kubectl get pods` -- READY 1/1 |
| Frontend serving | HTTP 200 on `/` | Ingress routes traffic when pod is ready |
| Migration Job status | Job completion | `kubectl get jobs` -- COMPLETIONS 1/1 |

### 10.2 Logging

The backend writes unstructured logs to stdout. Kubernetes captures these automatically:

```bash
kubectl logs -f deployment/homelab-backend
kubectl logs -f statefulset/homelab-postgresql
kubectl logs job/homelab-migrate
```

**Follow-up work** (not in this TDD): Add structured JSON logging to the backend, deploy a
log aggregator (Loki + Grafana is lightweight for homelab).

### 10.3 3am Diagnosability

If the game is down at 3am, the diagnostic path is:

1. `kubectl get pods -n homelab` -- which pods are not Ready?
2. `kubectl describe pod <name>` -- check Events for crash loops, OOM kills, probe failures.
3. `kubectl logs <pod>` -- check application logs.
4. `kubectl get events --sort-by=.lastTimestamp` -- cluster-level events.
5. `kubectl exec -it homelab-postgresql-0 -- pg_isready` -- database reachable?
6. `helm history homelab` -- what changed last?

**Rollback**: `helm rollback homelab <revision>` restores the previous deployment. This is a
massive improvement over the current "kill the process and hope" approach.

### 10.4 Production Readiness Criteria

Before declaring the migration complete:

- [ ] All pods running and passing probes for 24 hours
- [ ] At least one user has played through a full session on the new infrastructure
- [ ] Stress test results within acceptable range of pre-migration baseline
- [ ] `pg_dump` backup CronJob running (even if manual initially)
- [ ] DNS cutover complete and old VM's backend stopped
- [ ] Helm chart values committed to the repository (secrets excluded)

---

## 11. Implementation Phases

| Phase | Description | Dependencies | Complexity | Parallelizable |
|-------|------------|-------------|------------|----------------|
| 1 | Dockerfiles + .dockerignore + migrate.sh + nginx.conf | None | S | Yes (backend + frontend in parallel) |
| 2 | Helm chart (all templates + values.yaml) | Phase 1 (image references) | M | After Phase 1 |
| 3 | GitHub Actions workflow | Phase 1 (Dockerfiles must exist) | S | Parallel with Phase 2 |
| 4 | Database strategy (StatefulSet config, cutover runbook) | Phase 2 (part of Helm chart) | M | Partially parallel with Phase 2 |

**Total estimated effort**: 3-5 days of implementation work.

**Dependency graph**:

```
Phase 1 ──┬──> Phase 2 ──> Phase 4
           |
           └──> Phase 3
```

Phases 2 and 3 can proceed in parallel once Phase 1 is complete. Phase 4's StatefulSet
template is part of Phase 2, but the cutover runbook and data migration procedure are
documented separately and can be finalized after Phase 2.

---

## 12. Blast Radius Analysis

### What Could Go Wrong and How Bad Is It

| Failure Scenario | Blast Radius | Recovery Time | Data Loss |
|-----------------|--------------|---------------|-----------|
| Dockerfile build fails | None (existing deployment unaffected) | Fix and rebuild | None |
| Helm chart has a bug | None (no cluster to deploy to yet) | Fix chart | None |
| GitHub Actions workflow fails | No images pushed; old images still valid | Fix workflow | None |
| Backend container crashes in K8s | Game unavailable until restart (Kubernetes auto-restarts in ~30s) | 30s automatic | None (stateless) |
| PostgreSQL pod crashes | Game unavailable until PG restarts. PVC preserves data. | 1-2 min automatic | None (PVC) |
| PVC corruption / node disk failure | Game unavailable. Database must be restored from backup. | Manual restore (~30 min) | Data since last backup |
| Migration Job applies bad SQL | Database schema broken. Backend cannot start. | Rollback: restore pg_dump, fix migration. | None if pg_dump taken pre-migration |
| WebSocket fails through ingress | Game playable (falls back to HTTP polling) but no real-time updates | Fix ingress annotations | None |
| DNS cutover points to wrong IP | Game unreachable | Fix DNS record (~5 min propagation) | None |
| Full migration failure | Revert DNS to VM, restart old backend | 10-15 min manual | None (VM database is the last-known-good state) |

**Key insight**: The blast radius during Phases 1-3 is zero. The existing VM deployment is
completely untouched. Risk only materializes during Phase 4 cutover, and the rollback path
(DNS revert + VM restart) has been proven every time the developer deploys today.

---

## 13. Follow-Up Work (Out of Scope)

The following items are natural next steps after this migration but are explicitly not part of
this TDD:

1. **Database backup CronJob** -- `pg_dump` on a schedule, stored to a PVC or object storage.
2. **Structured logging** -- JSON logging in the backend, Loki/Grafana for aggregation.
3. **Metrics and monitoring** -- Prometheus scraping, Grafana dashboards for request latency,
   error rates, connection pool health, game action throughput.
4. **Sealed Secrets or external secrets operator** -- Replace base64-encoded K8s Secrets with
   encrypted-at-rest secrets management.
5. **Horizontal Pod Autoscaler** -- Requires refactoring in-memory state (rate limiter, mutexes)
   to distributed storage (Redis). Not needed at current scale.
6. **Automated deploy step in CI/CD** -- `helm upgrade` triggered by image push. Deferred until
   the cluster is provisioned and the manual deploy workflow is validated.
7. **Make WebSocket origin allowlist configurable** -- The hardcoded list in `ws/hub.go` should
   read from an env var. Small change, big operational benefit.
