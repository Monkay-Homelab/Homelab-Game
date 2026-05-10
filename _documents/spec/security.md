---
project: "homelab-the-game"
maturity: "experimental"
last_updated: "2026-04-04"
updated_by: "@staff-engineer"
scope: "Security architecture, auth patterns, trust boundaries, and known gaps in the Go backend and TypeScript frontend"
owner: "@staff-engineer"
dependencies:
  - code-quality.md
---

# Security Specification

This document describes the security architecture, authentication patterns, trust boundaries,
and known gaps that **actually exist** in the Homelab the Game codebase as of 2026-04-04. It is
intended as onboarding material for a developer contributing code. Aspirational improvements are
clearly marked as gaps.

---

## 1. Project Maturity Assessment

The project is in an **experimental** security posture. Authentication works (JWT + bcrypt),
authorization is enforced via user-scoped database queries, and the server-authoritative
game engine prevents most cheating. However, there are significant gaps in areas that would be
required for a production-grade service handling real user credentials: no token revocation,
no email verification, no security response headers from the Go app, no HTTP server timeouts,
and the environment defaults to permissive (non-production) mode.

### Honest Assessment

**What works well:**
- Server-authoritative game logic prevents client-side cheating
- bcrypt password hashing at default cost (10)
- Per-user mutex prevents race conditions on concurrent game actions
- Parameterized SQL queries throughout (no SQL injection vectors found)
- Sensitive model fields excluded from JSON serialization (`json:"-"`)
- Rate limiting on auth endpoints and game actions (IP-based and user-based)
- Origin validation on both CORS middleware and WebSocket upgrader
- Docker image runs as `nonroot` user (distroless base)
- `.env` files are gitignored

**What needs work:**
- JWT tokens cannot be revoked (no blacklist, no refresh token rotation)
- No email verification on registration
- No security response headers (HSTS, CSP, X-Frame-Options, etc.)
- ENV defaults to non-production (permissive CORS/WS origin checks)
- No HTTP server timeouts configured (ReadTimeout, WriteTimeout, IdleTimeout)
- WebSocket auth via query parameter exposes token in server logs and URL history
- Redis rate limiter fails open on Redis errors
- No password complexity requirements beyond length (8-128 chars)
- Database connection uses `sslmode=disable`

---

## 2. Authentication

### 2.1 Password Authentication

**File:** `apps/backend/internal/auth/password.go`

Passwords are hashed using bcrypt from `golang.org/x/crypto/bcrypt` at `bcrypt.DefaultCost`
(currently 10).

```
HashPassword(password string) (string, error)   -- bcrypt hash
CheckPassword(password, hash string) bool        -- bcrypt compare
```

**Validation (in `handlers/auth.go`):**
- Email: parsed via `net/mail.ParseAddress`, must contain a `.`, max 255 chars, lowercased
- Password: 8-128 characters (length only, no complexity rules)
- Display name: 2-20 chars, alphanumeric only (`^[a-zA-Z0-9]+$`), profanity-filtered, URL-filtered

**Registration toggle:** Controlled by `REGISTRATION_ENABLED` env var (defaults to `"true"`;
currently set to `"false"` in the local `.env`).

**Gap: No email verification.** Registration creates the account and issues a token immediately.
There is no email confirmation flow. A user can register with any email address they do not own.

**Gap: No password complexity.** Only length is checked (8-128 chars). No requirements for
uppercase, lowercase, digits, or special characters.

### 2.2 JWT Token Generation

**File:** `apps/backend/internal/auth/jwt.go`

| Property | Value |
|---|---|
| Algorithm | HS256 (HMAC-SHA256) |
| Library | `github.com/golang-jwt/jwt/v5` (v5.3.1) |
| Lifetime | 24 hours (hardcoded) |
| Claims | `user_id` (string), standard `exp` and `iat` |
| Secret source | `JWT_SECRET` env var or `/run/secrets/jwt_secret` Docker secret |
| Fallback | Random 32-byte hex secret generated at startup (warning logged) |

**Token generation:**
```go
ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour))
IssuedAt:  jwt.NewNumericDate(time.Now())
```

**Token validation** (in `auth.ValidateToken`):
- Checks signing method is `*jwt.SigningMethodHMAC` (prevents algorithm confusion attacks)
- Validates expiry via the `jwt/v5` library's standard claims checking
- Returns parsed `Claims` struct with `UserID`

**Gap: No token revocation.** There is no blacklist, no refresh token, and no mechanism to
invalidate a compromised token before its 24-hour expiry. Logout on the client simply removes
the token from `localStorage`; the token remains valid server-side.

**Gap: No refresh token rotation.** The single 24h token serves as both access and session
token. There is no short-lived access token + long-lived refresh token pattern.

**Gap: No `jti` (JWT ID) claim.** Tokens are not uniquely identified, which would be a
prerequisite for any future revocation mechanism.

### 2.3 Secret Management

**File:** `apps/backend/internal/config/config.go`

The `getSecret` function implements a three-tier lookup:
1. Environment variable (e.g., `JWT_SECRET`)
2. Docker secret file at `/run/secrets/<name>` (e.g., `/run/secrets/jwt_secret`)
3. Fallback value

Secrets using this pattern:
- `JWT_SECRET` / `jwt_secret` -- falls back to random generation with a warning
- `DB_PASSWORD` / `db_password` -- falls back to empty string
- `REDIS_PASSWORD` / `redis_password` -- falls back to empty string

**Production deployment** (`docker-stack.yml`): JWT_SECRET and DB_PASSWORD are injected via
environment variables referencing shell variables (`${JWT_SECRET}`, `${DB_PASSWORD}`), which
are expected to be set in the deployment environment. Docker secrets (file-based) are supported
by the code but not configured in the stack file.

**Dev `.env` file** (`apps/backend/.env`): Contains plaintext DB_PASSWORD and JWT_SECRET.
This file is listed in `.gitignore` and should not be committed.

**Gap: The dev `.env` file exists on the production server.** Since everything runs on a single
VM with no separate environments, the `.env` file with real credentials is present on disk.
The `loadEnvFile()` function in `main.go` reads `.env` from the working directory at startup.

### 2.4 User Model Serialization

**File:** `apps/backend/internal/models/user.go`

Sensitive fields are excluded from JSON responses:
- `PasswordHash *string` tagged `json:"-"`
- `OAuthID *string` tagged `json:"-"`

The `Email` field is included in responses (`json:"email,omitempty"`). This is intentional for
the auth response but means email addresses are included in any endpoint that returns user data.

---

## 3. Authorization

### 3.1 HTTP Middleware Chain

**File:** `apps/backend/internal/api/routes/routes.go`

The middleware chain is applied in this order (outermost first):
1. `CORS` -- origin validation + CORS headers
2. `JSON` -- sets `Content-Type: application/json`
3. `MaxBodySize` -- limits request body to 64KB
4. Per-route: `Auth` middleware for authenticated endpoints
5. Per-route: `RateLimit` or `RateLimitByUser` for rate-limited endpoints

**Route protection matrix:**

| Route | Auth | Rate Limit | Notes |
|---|---|---|---|
| `GET /health` | No | No | Health check |
| `POST /api/auth/register` | No | IP: 10/min | Public |
| `POST /api/auth/login` | No | IP: 10/min | Public |
| `GET /ws` | Query param | No (see note) | WebSocket upgrade |
| `GET /api/game/config` | No | No | Static config, cached 1h |
| `GET /api/game/state` | Bearer | No | User-scoped |
| `POST /api/game/action` | Bearer | User: 7200/min | 120 req/sec per user |
| `GET /api/social/group` | Bearer | No | User-scoped |
| `GET /api/social/groups` | Bearer | No | Public list (top 50) |
| `POST /api/social/group/*` | Bearer | User: 180/min | Role-checked |
| `GET /api/social/leaderboard` | Bearer | No | Public data |
| `POST /api/social/leaderboard/update` | Bearer | No | User-scoped |

**Gap: WebSocket endpoint has no HTTP-level rate limit.** The `/ws` endpoint accepts
connections without rate limiting. Rate limiting for WebSocket game actions is applied
inside `HandleWSAction` via `middleware.CheckGameActionRate(userID)` (7200/min per user),
but there is no limit on connection attempts themselves.

**Gap: `GET /api/game/state` and `GET /api/social/leaderboard` have no rate limit.**
An authenticated user could poll these endpoints without restriction.

### 3.2 Auth Middleware (HTTP)

**File:** `apps/backend/internal/api/middleware/auth.go`

Extracts `Authorization: Bearer <token>` header, validates the JWT, and injects `user_id`
into the request context via `context.WithValue`. Downstream handlers retrieve it via
`middleware.GetUserID(ctx)`.

The middleware returns generic error messages (`"missing authorization header"`,
`"invalid authorization format"`, `"invalid token"`) without leaking implementation details.

### 3.3 WebSocket Authentication

**File:** `apps/backend/internal/api/ws/hub.go`

WebSocket authentication uses a JWT token passed as a query parameter:
```
GET /ws?token=<jwt>
```

The `HandleConnect` function:
1. Extracts `token` from `r.URL.Query().Get("token")`
2. Validates via `auth.ValidateToken(token, jwtSecret)`
3. Upgrades to WebSocket on success
4. Enforces single connection per user (closes old connection if exists)

**Security concern: Token in URL.** Query parameters are logged by web servers, proxies,
and appear in browser history. The token is the same JWT used for HTTP Bearer auth (24h
lifetime). In the current architecture, TLS termination happens at the external nginx proxy,
and Traefik routes internally over HTTP -- so the token is in plaintext on the internal
overlay network.

**Mitigation: Origin check.** The WebSocket upgrader validates the `Origin` header against the
same allowlist as the CORS middleware (hardcoded origins + env-based additions + dev mode
localhost entries).

### 3.4 Data Access Authorization

All game state queries are scoped to the authenticated user's ID:
- `LoadFullGameState(ctx, pool, userID)` filters by `WHERE user_id = $1`
- Child table queries filter by `WHERE game_state_id = $1` (linked to the user's game state)
- Social actions (promote, kick) check the acting user's role (`founder` or `admin`)

There is no admin/superuser system. There are no endpoints that allow one user to access
another user's game state.

**Authorization model for groups:**
- `founder`: Can promote members, kick members, leave (which deletes the group)
- `admin`: Can promote members, kick members (except founder)
- `member`: Can only leave

**Gap: Admins can promote other members to admin, but there is no demotion mechanism.**
Once promoted, an admin can only be removed by being kicked.

**Gap: `POST /api/social/leaderboard/update` updates the calling user's leaderboard scores.**
It is authenticated but has no rate limit. A user could call it repeatedly, though the
effect is idempotent (it reads current game state and writes the score).

---

## 4. Trust Boundaries

```
                    Internet
                       |
                   [nginx] ---- TLS termination
                       |
                    [Traefik] ---- Layer 7 routing, sticky sessions
                       |
            +----------+----------+
            |                     |
    [Backend API x2]        [Frontend nginx]
     (Go, port 8085)        (static, port 80)
            |
     +------+------+
     |             |
  [PostgreSQL]  [Redis]
  (port 5432)  (port 6379)
```

### Trust Boundary 1: Internet to nginx (External)
- TLS terminated at external nginx (not on this VM)
- nginx proxies to Traefik on port 80 (plaintext HTTP)
- This boundary is outside the project's control

### Trust Boundary 2: Traefik to Backend
- Traefik routes based on `Host` header: `api.homelab.living` to backend
- Communication is plaintext HTTP over Docker overlay network
- Traefik adds its own CORS middleware (configured in `docker-stack.yml` labels)
- Sticky sessions via `_backend_affinity` cookie (HttpOnly, Secure, SameSite=Strict)

### Trust Boundary 3: Backend to Database
- Connection string uses `sslmode=disable`
- Communication is plaintext over Docker overlay network (`backend` network)
- Database is not exposed to the `frontend` network
- Connection pool: 5-50 connections

### Trust Boundary 4: Backend to Redis
- No password configured in the stack file (Redis runs without auth)
- Communication is plaintext over Docker overlay network
- Redis is ephemeral (no persistence: `--save ""` `--appendonly no`)

### Trust Boundary 5: Client to Server
- Tokens stored in `localStorage` (accessible to any JS on the page)
- HTTP API uses Bearer token in Authorization header
- WebSocket uses token in query parameter
- Client falls back from WebSocket to HTTP API on connection failure

**Gap: No network-level encryption between services.** All inter-service communication
(Traefik to backend, backend to PostgreSQL, backend to Redis) is plaintext. This is
acceptable for a single-host Docker Swarm deployment where all traffic stays on local overlay
networks, but would be a concern if services were distributed across hosts.

---

## 5. CORS Configuration

### 5.1 Application-Level CORS

**File:** `apps/backend/internal/api/middleware/cors.go`

Hardcoded allowed origins:
- `https://game.homelab.living`
- `http://game.homelab.living`
- `https://homelab.living`
- `http://homelab.living`
- `https://dev-game.homelab.living`
- `http://dev-game.homelab.living`

**Environment-aware expansion:**
- `CORS_ORIGINS` env var: comma-separated additional origins
- When `ENV != "production"`: adds `http://localhost:3000`, `http://127.0.0.1:3000`,
  `http://192.168.3.107:3000`

CORS response headers set on matching origins:
- `Access-Control-Allow-Origin: <matched origin>` (not `*`)
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type, Authorization`
- `Vary: Origin`

**Gap: ENV defaults to non-production.** The `ENV` variable is not set in the local `.env`
file. The check is `os.Getenv("ENV") != "production"`, which means any value other than
`"production"` (including empty/unset) enables localhost origins. In the `docker-stack.yml`,
`ENV: production` is set for the containerized deployment, but local dev runs without it.

**Gap: No `Access-Control-Allow-Credentials` header.** This means browsers will not send
cookies with cross-origin requests. Currently not needed (auth is via Bearer token in
headers), but would be required if cookie-based auth were added.

**Gap: `Access-Control-Max-Age` not set in Go CORS.** Preflight results are not cached by the
browser. (Traefik's CORS middleware does set `accesscontrolmaxage=600`.)

### 5.2 Traefik-Level CORS

**File:** `docker-stack.yml` (Traefik labels on backend service)

Traefik adds a second CORS layer:
- `accesscontrolallowmethods`: GET, POST, OPTIONS
- `accesscontrolallowheaders`: Content-Type, Authorization
- `accesscontrolalloworiginlist`: `https://game.homelab.living` (only production, HTTPS)
- `accesscontrolallowcredentials`: true
- `accesscontrolmaxage`: 600

**Note: CORS headers may be duplicated.** Both Traefik and the Go app set CORS headers.
If both fire, the browser may see duplicate `Access-Control-Allow-Origin` headers, which
some browsers reject. In practice, Traefik's origin list is a subset of the Go app's list,
so the Traefik CORS may be redundant or conflicting.

### 5.3 WebSocket Origin Check

**File:** `apps/backend/internal/api/ws/hub.go`

The WebSocket upgrader's `CheckOrigin` function uses a near-identical allowlist to the CORS
middleware:
- Same hardcoded origins (minus `http://192.168.3.107:3000` in dev mode)
- Same `ENV != "production"` check for localhost
- Same `CORS_ORIGINS` env var support

**Gap: Origin lists are duplicated.** The CORS middleware and WebSocket upgrader maintain
separate, hardcoded copies of the allowed origin list. Adding a new origin requires changes
in two files (`middleware/cors.go` and `ws/hub.go`). These can drift.

---

## 6. Rate Limiting

### 6.1 Architecture

**Files:**
- `apps/backend/internal/api/middleware/ratelimit.go` -- interface + in-memory store
- `apps/backend/internal/api/middleware/redis_ratelimit.go` -- Redis-backed store

The rate limiter uses a pluggable `RateLimitStore` interface:

```go
type RateLimitStore interface {
    CheckRate(ctx context.Context, key string, maxPerMinute int) bool
}
```

**Backends:**
- **In-memory (default):** Uses a `map[string]*visitor` with a mutex. Cleanup goroutine
  removes entries inactive for >1 minute. Per-process only; not shared across replicas.
- **Redis (optional):** Uses a Lua script with `INCR` + `EXPIRE` for atomic rate checking.
  Shared across all backend replicas.

Backend selection happens at startup in `main.go`:
```go
if rdb != nil {
    middleware.SetRateLimitStore(middleware.NewRedisRateLimitStore(rdb))
}
```

### 6.2 Rate Limit Buckets

| Bucket Name | Key Pattern | Limit | Applied To |
|---|---|---|---|
| `auth` | `auth:ip:<client_ip>` | 10/min | `/api/auth/register`, `/api/auth/login` |
| `game` | `game:user:<user_id>` (fallback: `game:ip:<ip>`) | 7200/min | `POST /api/game/action` |
| `game` (WS) | `game:user:<user_id>` | 7200/min | WebSocket action messages |
| `social` | `social:user:<user_id>` (fallback: `social:ip:<ip>`) | 180/min | Social write endpoints |

### 6.3 Client IP Extraction

**File:** `apps/backend/internal/api/middleware/ratelimit.go` (`getClientIP`)

Priority:
1. `X-Real-IP` header (set by trusted reverse proxy)
2. `X-Forwarded-For` header, rightmost entry (last hop appended by trusted proxy)
3. `r.RemoteAddr` (direct connection)

This is correct for a trusted-proxy architecture. The rightmost XFF entry cannot be spoofed
by the client when the proxy is the one appending it.

### 6.4 Known Gaps

**Gap: Redis rate limiter fails open.** When `RedisRateLimitStore.CheckRate` encounters a
Redis error, it returns `true` (allows the request) and logs the error. This means a Redis
outage disables rate limiting entirely for Redis-backed deployments.

**Gap: In-memory rate limiter is per-process.** With 2 backend replicas (as configured in
`docker-stack.yml`), the in-memory rate limiter allows effectively double the configured
limit when requests are distributed across replicas. Redis-backed rate limiting is shared.

**Gap: No rate limit on WebSocket connection attempts.** The `/ws` endpoint has no rate
limiting middleware. A client could repeatedly connect/disconnect to exhaust server resources.

**Gap: No rate limit on `GET /api/game/state`, `GET /api/social/group`,
`GET /api/social/groups`, `GET /api/social/leaderboard`.** Read endpoints are authenticated
but have no rate limit.

**Gap: Rate limit response does not include `Retry-After` header.** Clients receive a 429
response but have no indication of when they can retry.

---

## 7. Anti-Cheat / Server-Authoritative Model

### 7.1 Architecture

The game uses a server-authoritative model. Clients send action requests; the server validates
and executes them. The client never directly modifies game state.

**Flow (HTTP):**
1. Client sends `POST /api/game/action` with `{"type": "buy_hardware", "payload": {...}}`
2. `PerformAction` handler acquires per-user mutex
3. Loads full game state from database
4. Runs `engine.ProcessIdleProgress` to catch up idle earnings
5. Runs `engine.ProcessAction` which validates and executes the action
6. Persists results to database
7. Returns updated full state to client

**Flow (WebSocket):**
1. Client sends `{"type": "action", "id": "<uuid>", "action": "buy_hardware", "payload": {...}}`
2. `HandleWSAction` checks rate limit, acquires per-user mutex
3. Same validation/execution path as HTTP
4. Returns `action_result` message with success/failure + updated state

### 7.2 Per-User Mutex

**File:** `apps/backend/internal/api/handlers/game.go` (`userMutexMap`)

A per-user mutex prevents race conditions when a user sends concurrent actions (e.g., rapid
clicking or scripted requests). This ensures:
- Game state is loaded, mutated, and saved atomically per user
- No double-spend on purchases
- No parallel action processing for the same user

The mutex map has a cleanup goroutine that removes entries for users inactive >10 minutes.

The WebSocket handler includes panic recovery that releases the mutex to prevent deadlocks:
```go
defer func() {
    if r := recover(); r != nil {
        if locked {
            h.userLocks.Unlock(userID)
        }
        // ... log and send error response
    }
}()
```

### 7.3 Server-Side Validation

The engine (`internal/game/engine/engine.go`) validates all actions server-side:
- **Resource checks:** Can the user afford the purchase? (compute, money, reputation)
- **Tier gates:** Is the user at the required tier for this item?
- **Slot/capacity limits:** Does the user have available hardware slots or rack units?
- **Catalog validation:** Is the requested item in the server-side catalog?
- **State consistency:** Are prerequisites met (e.g., SaaS unlocked before deploying SaaS)?

The `ProcessAction` function uses a `switch` statement with ~25 known action types. Unknown
actions return an error (`"unknown action: %s"`).

**Gap: The unknown action error message includes the user-supplied action string.** This is
a minor information leak -- the error message reflects untrusted input back to the client,
though it is bounded by the 64KB body limit and JSON parsing.

### 7.4 Tick System

The server runs a 5-second tick for each connected WebSocket user. The tick:
1. Loads game state from database
2. Calculates idle progress since last tick
3. Saves updated state
4. Pushes state to client over WebSocket

This is configurable via `TICK_INTERVAL_SECONDS` env var.

**Optimization: Light ticks.** If the user has not performed any action since the last tick
(not "dirty"), the tick reuses cached data instead of querying child tables, reducing DB load.

---

## 8. Input Validation

### 8.1 Request Body Limits

**File:** `apps/backend/internal/api/middleware/bodylimit.go`

All HTTP requests have a 64KB body limit via `http.MaxBytesReader`. WebSocket messages
have a 64KB read limit via `conn.SetReadLimit(65536)`.

### 8.2 JSON Parsing

All request bodies are decoded via `json.NewDecoder(r.Body).Decode(&req)` with typed
structs. Unknown fields are silently ignored (Go's default JSON behavior). Malformed JSON
returns a 400 error.

### 8.3 Display Name Validation

- Length: 2-20 characters
- Character set: alphanumeric only (`^[a-zA-Z0-9]+$`)
- Profanity filter: ~20 blocked words (substring match, case-insensitive)
- URL filter: blocks patterns matching `(?i)(https?://|www\.|\.com|\.net|\.org|\.io)`

### 8.4 SQL Injection Prevention

All database queries use parameterized queries (`$1`, `$2`, etc.) via the `pgx` library.
No string concatenation or `fmt.Sprintf` is used in SQL query construction. Verified by
searching the entire `internal/database/queries/` directory.

### 8.5 Group Name Validation

Group names are validated for length (3-50 chars) but have no character set or profanity
filtering, unlike display names.

**Gap: Group names accept arbitrary Unicode.** A user could create a group with special
characters, control characters, or offensive content not caught by the display name filter.

---

## 9. Security-Relevant Dependencies

| Package | Version | Purpose | Security Notes |
|---|---|---|---|
| `golang-jwt/jwt/v5` | v5.3.1 | JWT generation/validation | Actively maintained; v5 requires explicit algorithm selection |
| `golang.org/x/crypto` | v0.49.0 | bcrypt password hashing | Official Go crypto extension |
| `jackc/pgx/v5` | v5.8.0 | PostgreSQL driver | Parameterized query support |
| `gorilla/websocket` | v1.5.3 | WebSocket connections | Maintained by the gorilla community |
| `redis/go-redis/v9` | v9.18.0 | Redis client | Used for rate limiting + pub/sub |

All dependencies are well-known, actively maintained Go packages. The `go.sum` file provides
integrity verification. No known CVEs at current versions (as of 2026-04-04 assessment).

**Frontend:**
- Tokens stored in `localStorage` -- accessible to any JavaScript running on the page
- API URL defaults to `https://api.homelab.living` (HTTPS, hardcoded)
- WebSocket URL derived from API URL (protocol swap to `wss://`)

---

## 10. HTTP Server Configuration

**File:** `apps/backend/cmd/server/main.go`

```go
srv := &http.Server{
    Addr:    addr,
    Handler: handler,
}
```

**Gap: No HTTP server timeouts.** The server does not set `ReadTimeout`, `WriteTimeout`,
`ReadHeaderTimeout`, or `IdleTimeout`. This makes the server vulnerable to slowloris-style
attacks where a client opens a connection and sends data very slowly to exhaust server
resources. The external nginx proxy may provide some protection, but the Go server itself
is unprotected.

**Gap: No `MaxHeaderBytes` configured.** Defaults to Go's `http.DefaultMaxHeaderBytes`
(1MB), which is generous.

**Graceful shutdown:** The server handles `SIGTERM`/`SIGINT` with a 10-second grace period
for draining connections.

---

## 11. Security Headers

**Gap: No security response headers from the Go application.**

The following headers are NOT set by the Go backend:
- `Strict-Transport-Security` (HSTS)
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy`
- `Referrer-Policy`
- `Permissions-Policy`
- `X-XSS-Protection` (deprecated but still useful for older browsers)

The JSON middleware sets `Content-Type: application/json` on all responses, which provides
some XSS protection for API responses. The frontend is served by a separate nginx container
which may add its own headers (not verified -- nginx config is in the container image).

Some of these headers may be set by the external nginx reverse proxy or by Traefik
middleware, but the Go application does not set them itself.

---

## 12. Network & Deployment Security

### 12.1 Docker Swarm Topology

**File:** `docker-stack.yml`

| Service | Network(s) | Exposed Ports |
|---|---|---|
| Traefik | `frontend` | `80` (host) |
| Backend (x2) | `backend`, `frontend` | None (accessed via Traefik) |
| Frontend | `frontend` | None (accessed via Traefik) |
| PostgreSQL | `backend` | None |
| Redis | `backend` | None |

Network isolation: PostgreSQL and Redis are only on the `backend` network. The frontend
container cannot reach them. Traefik is only on the `frontend` network and routes to the
backend via overlay.

### 12.2 Container Security

- **Backend image:** Built on `gcr.io/distroless/static-debian12`, runs as `nonroot:nonroot`
- **Multi-stage build:** Build artifacts only; no Go toolchain in runtime image
- **Health checks:** All services have Docker-level health checks

### 12.3 Sticky Sessions

Traefik configures sticky sessions for the backend:
- Cookie name: `_backend_affinity`
- `HttpOnly: true` -- not accessible via JavaScript
- `Secure: true` -- only sent over HTTPS
- `SameSite: strict` -- not sent on cross-site requests

This is important for WebSocket connections: the client must consistently reach the same
backend replica to maintain their WebSocket connection.

### 12.4 Database Security

- `sslmode=disable` for the PostgreSQL connection (acceptable for same-host overlay network)
- Redis runs without authentication (no `requirepass`)
- Redis has `maxmemory 128mb` with `allkeys-lru` eviction
- Redis has no persistence (`--save ""` `--appendonly no`) -- data is ephemeral

**Gap: Redis has no authentication.** Any container on the `backend` overlay network can
connect to Redis without credentials. This is acceptable for the current single-host setup
but would be a concern in a multi-host deployment.

---

## 13. Logging & Auditability

### What is Logged

- WebSocket connection/disconnection events (no user identification in log message)
- WebSocket send buffer full warnings (includes user ID)
- WebSocket panic recovery (includes user ID, action, request ID)
- Redis errors (in rate limiter and broadcaster)
- Server startup configuration (port, Redis address)
- JWT_SECRET fallback warning

### What is NOT Logged

- Authentication attempts (successful or failed)
- Registration events
- Game action requests
- Rate limit violations (429s are returned but not logged)
- User IP addresses on requests

**Gap: No authentication audit log.** Failed login attempts, successful logins, and
registrations are not logged. This makes it impossible to detect brute-force attacks,
credential stuffing, or unauthorized access after the fact.

**Gap: No request logging/access log.** The Go server does not produce access logs.
Traefik may produce access logs depending on its configuration, but the Go application
itself has no request-level logging.

---

## 14. Known Vulnerability Summary

Ordered by estimated risk (highest first):

| # | Finding | Severity | Current Mitigation |
|---|---|---|---|
| 1 | No token revocation | Medium | 24h expiry limits window |
| 2 | No HTTP server timeouts | Medium | External nginx may provide protection |
| 3 | ENV defaults to permissive | Medium | `docker-stack.yml` sets `ENV: production` |
| 4 | No security response headers | Medium | External proxies may add some |
| 5 | WS token in query parameter | Low-Medium | Origin validation; TLS at edge |
| 6 | Redis rate limiter fails open | Low-Medium | Logged; Redis is local and stable |
| 7 | No authentication audit log | Low-Medium | None |
| 8 | No email verification | Low | Registration currently disabled |
| 9 | Redis without authentication | Low | Only accessible on backend network |
| 10 | DB connection without SSL | Low | Same-host overlay network |
| 11 | Group name allows arbitrary content | Low | Group visibility is limited |
| 12 | Duplicate CORS origin lists | Low | Manual sync (drift risk) |

**Note on severity:** These are assessed relative to the project's current maturity
(experimental, single-host homelab deployment, no real money transactions yet). Severity
would increase significantly if the project moved to a multi-host deployment, handled real
payments (IAP), or grew its user base.

---

## 15. Recommendations for Contributors

### Before Writing Security-Sensitive Code

1. **All game logic goes in the engine.** Never trust client input -- validate in
   `engine.ProcessAction`. The handler layer is for I/O; the engine is for business rules.
2. **Use parameterized queries.** pgx handles this naturally with `$1` placeholders. Never
   concatenate user input into SQL strings.
3. **Scope data access to the authenticated user.** Always filter by `user_id` or
   `game_state_id`. There is no admin role -- every query should be user-scoped.
4. **Rate limit new endpoints.** Use `RateLimitByUser` for authenticated endpoints or
   `RateLimitNamed` for public ones. See `routes.go` for examples.

### Environment-Aware Development

- The codebase checks `os.Getenv("ENV") != "production"` to enable dev-mode behavior
  (localhost CORS origins, localhost WebSocket origins)
- When developing locally, do NOT set `ENV=production` or you will lose CORS access from
  `localhost:3000`
- The `docker-stack.yml` sets `ENV: production` for deployed replicas

### Token Handling

- The JWT token is the sole authentication credential. There is no session server-side.
- Client stores it in `localStorage` and sends it as `Authorization: Bearer <token>`
- For WebSocket, it is sent as `?token=<jwt>` query parameter
- Token lifetime is 24 hours, non-revocable
