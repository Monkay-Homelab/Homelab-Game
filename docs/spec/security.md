---
project: "project"
maturity: "proof-of-concept"
last_updated: "2026-03-20"
updated_by: "@staff-engineer"
scope: "Security posture, authentication, authorization, secret management, and trust boundaries for Homelab the Game"
owner: "@staff-engineer"
dependencies:
  - architecture.md
---

# Security Specification

This document describes the actual security state of the Homelab the Game codebase as of
2026-03-20. It is based on a thorough reading of the backend (Go), desktop client
(Tauri + React), database schema, and configuration. It documents what exists, identifies
gaps honestly, and provides context for future hardening work.

---

## 1. Authentication

### 1.1 Supported Auth Methods

**Email/Password** is the only implemented authentication method. The codebase has schema
columns for OAuth (oauth_provider, oauth_id on the users table) and CLAUDE.md references
Google, Apple, and Discord OAuth, but no OAuth flow is implemented in code.

### 1.2 Password Handling

- **Hashing:** bcrypt via `golang.org/x/crypto/bcrypt` at `bcrypt.DefaultCost` (cost 10).
  Implementation in `apps/backend/internal/auth/password.go`.
- **Validation:** Passwords must be 8-128 characters. No complexity requirements (no
  uppercase, digit, or special character enforcement).
- **Storage:** Hashed password stored in `users.password_hash` (VARCHAR 255). The raw
  password is never persisted.
- **Password hash in model:** The `User` struct correctly uses `json:"-"` on `PasswordHash`
  to prevent serialization in API responses.

### 1.3 Registration

- **Input validation:**
  - Email: trimmed, lowercased, validated via `net/mail.ParseAddress`, checked for "." presence,
    max 255 characters.
  - Display name: 2-20 characters, alphanumeric only (`^[a-zA-Z0-9]+$`), profanity filter
    (blocklist of ~20 words), URL pattern filter.
  - Password: 8-128 characters, no further complexity rules.
- **Uniqueness:** Email uniqueness enforced at the database level (UNIQUE constraint on
  `users.email`). Display name uniqueness is NOT enforced.
- **Flow:** Register creates the user, creates initial game state, and returns a JWT
  immediately. There is no email verification step.

### 1.4 Login

- Login accepts email + password. On invalid credentials, returns a generic
  `"invalid credentials"` message (does not distinguish between wrong email and wrong
  password). This is correct for preventing user enumeration.

### 1.5 JWT Tokens

- **Library:** `github.com/golang-jwt/jwt/v5`
- **Algorithm:** HMAC-SHA256 (`HS256`)
- **Signing key:** A shared secret loaded from the `JWT_SECRET` environment variable.
- **Claims:** Custom `Claims` struct containing `UserID` (string) plus standard registered
  claims (`ExpiresAt`, `IssuedAt`).
- **Token lifetime:** 24 hours. There is no refresh token mechanism.
- **Validation:** `ValidateToken` correctly verifies the signing method is HMAC before
  accepting the key, preventing algorithm confusion attacks.
- **No token revocation:** There is no blocklist, token versioning, or server-side session
  store. A compromised token remains valid until expiry.
- **No issuer/audience claims:** The JWT does not set or validate `iss` or `aud` claims.

### 1.6 Session Management

Tokens are stored in `localStorage` on the client. There is no `httpOnly` cookie-based
session mechanism. The client clears the token on 401 responses and on explicit logout.

**Gap:** `localStorage` is accessible to any JavaScript running in the page context. In a
Tauri application this is less critical than in a browser (no third-party scripts), but it
remains a deviation from cookie-based best practice.

---

## 2. Authorization

### 2.1 Middleware-Based Auth

The `Auth` middleware (`apps/backend/internal/api/middleware/auth.go`) extracts the
`Authorization: Bearer <token>` header, validates the JWT, and injects the `user_id` into
the request context.

### 2.2 Route Protection

Routes are divided into three categories:

| Category | Routes | Auth Required | Rate Limited |
|----------|--------|---------------|--------------|
| Public | `GET /health`, `GET /api/game/config` | No | No |
| Auth | `POST /api/auth/register`, `POST /api/auth/login` | No | Yes (10/min/IP) |
| Game | `GET /api/game/state`, `POST /api/game/action` | Yes | Yes (7200/min/user) |
| Social | All `/api/social/*` endpoints | Yes | Mixed (read: no limit; write: 180/min/user) |
| WebSocket | `GET /ws` | Yes (token in query param) | No |

### 2.3 Game State Authorization

Game state access is scoped by user ID. The `GetByUserID` query and the `Update` query both
include `WHERE user_id = $1` / `WHERE id = $1 AND user_id = $31`, which prevents a user from
modifying another user's game state.

**Gap:** Some child entity queries (hardware, services, upgrades, customers, expenses,
component upgrades) filter by `game_state_id` rather than `user_id` directly. The game
state ID is looked up from the authenticated user ID first, so the chain is:
`authenticated user_id -> game_state.id -> child entities`. This is functionally correct
but relies on the handler always performing the initial user-scoped lookup.

### 2.4 Social Feature Authorization

- **Group operations:** Promote and kick require the caller's role to be "founder" or "admin".
  Target membership in the same group is verified before action. Founders cannot be kicked.
- **Leaderboard update:** The `POST /api/social/leaderboard/update` endpoint is authenticated
  but any authenticated user can trigger it for their own scores. There is no admin-only
  restriction.

**Gap:** The `UpdateLeaderboards` endpoint lets any user push their current game state scores
to the leaderboard. If game state values could be manipulated (they cannot currently due to
server-authoritative design), this would be an escalation path. Currently benign but worth
noting as an unnecessary public surface.

### 2.5 Hardware Sell Authorization

When selling hardware, the engine looks up the hardware item by ID from the list already
fetched for the authenticated user's game state. This means a user cannot sell hardware they
do not own, since the hardware list is pre-scoped. However, the `DeleteByID` query in
`HardwareQueries` deletes by hardware ID alone without a user/game_state scoping clause.
The handler's pre-check makes this safe in practice, but a defense-in-depth approach would
add a game_state_id condition to the DELETE query.

---

## 3. Server-Authoritative Game Logic (Anti-Cheat)

The game is designed as server-authoritative. All game state mutations flow through the
backend `Engine.ProcessAction` method, which validates every action before applying it:

- **Resource checks:** Sufficient compute units, money, reputation before purchases.
- **Tier gating:** Hardware, services, and upgrades are gated by minimum tier.
- **Capacity checks:** Power limits, hardware slots, rack units validated before additions.
- **Catalog validation:** All purchasable items are looked up from server-side catalogs,
  not derived from client input.
- **Duplicate prevention:** Upgrade purchases check for existing ownership.
- **Per-user locking:** `userMutexMap` prevents race conditions from concurrent requests
  for the same user. This prevents double-spend exploits.

**Optimistic client rendering:** The frontend applies a local click reward before the server
response arrives (`runJob` in `gameStore.ts`). This is purely cosmetic; the server response
overwrites the local state.

**Gap:** The `userMutexMap` grows unboundedly. There is no eviction of mutex entries for
users who have disconnected. This is a memory concern, not a security concern.

---

## 4. Input Validation

### 4.1 Request Body

- **Body size limit:** Global `MaxBodySize` middleware caps all request bodies at 64KB.
- **JSON decoding:** All handlers decode into typed structs with `json.NewDecoder`.
  Unknown fields are silently ignored (Go's default `json.Decoder` behavior).
- **Action type whitelist:** `ProcessAction` uses a `switch` statement on action type strings.
  Unknown actions return an error. This prevents arbitrary action injection.

### 4.2 SQL Injection

All database queries use parameterized queries via `pgx`. No string interpolation is used
in query construction. The one case where SQL is dynamically constructed -- the leaderboard
`GetTopByCategory` query -- uses a hardcoded whitelist map (`allowed`) to select column
names. User input never reaches the SQL string directly.

### 4.3 Display Name Filtering

Registration applies:
- Alphanumeric-only regex: `^[a-zA-Z0-9]+$`
- Profanity blocklist (case-insensitive substring match)
- URL pattern filter (blocks http/https URLs, common TLDs)

**Gap:** Group names (`CreateGroup`) validate length (3-50 characters) but do not apply the
same profanity filter or character restrictions as display names. Group names could contain
special characters, profanity, or URLs.

---

## 5. Transport Security

### 5.1 Backend Server

The Go server runs plain HTTP via `http.ListenAndServe`. There is no TLS termination in the
application itself.

### 5.2 Production TLS

Based on the CORS allowed origins (`https://game.homelab.living`, `https://homelab.living`)
and the WebSocket hub comment ("keeps connection alive through nginx proxy"), TLS is
terminated at an external reverse proxy (nginx), not by the Go application. The Go
application listens on a local port and nginx handles HTTPS.

### 5.3 Database Connection

The database connection string uses `sslmode=disable`
(`apps/backend/internal/config/config.go:43`). This means the connection between the Go
application and PostgreSQL is unencrypted. Since CLAUDE.md states "infrastructure is
self-hosted on a homelab VM" and everything runs on the same server, this is a reasonable
tradeoff for a single-host deployment but would need to change for any multi-host setup.

### 5.4 WebSocket Authentication

WebSocket connections authenticate via JWT passed as a query parameter (`?token=...`).
This means the token appears in URL strings and potentially in server access logs. This is
a common pattern for WebSocket auth (the WebSocket protocol does not support custom headers
during the upgrade handshake from browsers), but the token exposure in logs is a concern.

---

## 6. CORS Configuration

### 6.1 HTTP CORS

Implemented in `apps/backend/internal/api/middleware/cors.go`:

- **Production origins:** `https://game.homelab.living`, `http://game.homelab.living`,
  `https://homelab.living`, `http://homelab.living`
- **Dev origins (when `ENV != "production"`):** `http://localhost:3000`,
  `http://127.0.0.1:3000`, `http://192.168.3.107:3000`
- **Additional origins:** Configurable via `CORS_ORIGINS` environment variable
  (comma-separated)
- **Allowed methods:** GET, POST, OPTIONS
- **Allowed headers:** Content-Type, Authorization
- **Credentials:** `Access-Control-Allow-Credentials` is NOT set (credentials mode is not
  used since auth is via Bearer token, not cookies)

**Observation:** The `allowedOrigins` map is built once at init time. The `ENV` environment
variable check is not strictly gated, so if `ENV` is unset in production, dev origins
(localhost) will be permitted.

### 6.2 WebSocket CORS

The WebSocket upgrader (`apps/backend/internal/api/ws/hub.go`) has its own origin check
via `CheckOrigin`, with a hardcoded list including both production and dev origins
(localhost, LAN IP). Unlike the HTTP CORS middleware, this list is not environment-aware --
dev origins are always allowed regardless of the `ENV` variable.

**Gap:** The WebSocket origin allowlist includes `http://192.168.3.107:3000` unconditionally,
which is a specific LAN IP. This is harmless but couples the code to a specific network
configuration. More importantly, the list does not respect the `ENV` variable, so
localhost/LAN origins are accepted even in production.

---

## 7. Rate Limiting

### 7.1 Implementation

In-memory rate limiter using a `sync.Mutex`-protected map of visitor counts
(`apps/backend/internal/api/middleware/ratelimit.go`).

- **Auth endpoints:** 10 requests per minute per client IP.
- **Game action endpoint:** 7200 requests per minute per authenticated user (120/sec effective).
- **Social write endpoints:** 180 requests per minute per authenticated user.
- **Social read endpoints and game state GET:** No rate limiting.

### 7.2 Client IP Extraction

The `getClientIP` function checks `X-Forwarded-For` (first IP), then `X-Real-IP`, then
falls back to `RemoteAddr`.

**Gap:** `X-Forwarded-For` is trusted unconditionally. If the Go server is directly
internet-facing (without a trusted proxy), an attacker can spoof their IP by setting this
header. Since the architecture uses nginx as a reverse proxy, this is likely safe as long as
nginx overwrites the header -- but the code does not validate that the header comes from a
trusted source.

### 7.3 Cleanup

A background goroutine cleans up expired visitor entries every minute (entries older than 1
minute are removed). This prevents unbounded memory growth.

### 7.4 Limitations

- The rate limiter is in-process. If multiple server instances were deployed (they are not
  currently), rate limits would not be shared.
- There is no account lockout after repeated failed login attempts. The 10/min IP-based
  rate limit provides some brute-force protection, but a distributed attack would bypass it.
- Game state GET (`/api/game/state`) has no rate limit. The client polls this every 5 seconds
  for idle progress. A malicious client could poll much faster.

---

## 8. Secret Management

### 8.1 Environment Variables

Secrets are loaded from environment variables, with fallback to a `.env` file read by a
custom `loadEnvFile()` function in `cmd/server/main.go`.

| Secret | Env Var | Current State |
|--------|---------|---------------|
| JWT signing key | `JWT_SECRET` | 64-char hex string in `.env` |
| DB password | `DB_PASSWORD` | Plaintext in `.env` |
| DB host | `DB_HOST` | `localhost` in `.env` |

### 8.2 .env File

The `.env` file at `apps/backend/.env` contains plaintext secrets:
- Database password
- JWT secret

The `.gitignore` correctly excludes `.env` and `apps/backend/.env`. The file is NOT tracked
in git (verified: not present in any commit on the current branch).

### 8.3 JWT Secret Fallback

If `JWT_SECRET` is not set, the config loader generates a random 32-byte secret and logs
a warning. This means the server is functional without configuration, but all sessions are
invalidated on restart. This is acceptable for development but could cause confusion if
accidentally deployed without the environment variable.

### 8.4 Tauri Configuration

The Tauri desktop app (`apps/desktop/src-tauri/tauri.conf.json`) has:
- **CSP:** Set to `null` (Content Security Policy disabled). This allows the webview to
  load any content.
- **Capabilities:** Only `core:default` permissions are granted. No filesystem, shell, or
  other dangerous Tauri APIs are exposed.
- **Identifier:** Uses placeholder `com.tauri.dev` (not a production identifier).

---

## 9. Trust Boundaries

### 9.1 Boundary Map

```
                          Internet
                             |
                         [nginx/TLS]
                             |
                      [Go HTTP Server]  <-- Trust boundary 1: All input untrusted
                             |
                       [Auth Middleware]  <-- Trust boundary 2: JWT validation
                             |
                      [Game Engine]      <-- Trust boundary 3: Action validation
                             |
                        [PostgreSQL]     <-- Trust boundary 4: Parameterized queries
```

### 9.2 Client Trust

The client is untrusted. All game logic runs server-side. The client's optimistic UI updates
are cosmetic and overwritten by server responses. The server does not trust any client-provided
game state values.

### 9.3 Database Trust

The database is trusted (same host, `sslmode=disable`). All queries use parameterized
statements. There is no ORM -- raw SQL with `pgx` parameterized queries throughout.

---

## 10. Dependency Security

### 10.1 Go Dependencies

| Dependency | Version | Purpose | Risk Notes |
|------------|---------|---------|------------|
| `golang-jwt/jwt/v5` | v5.3.1 | JWT signing/validation | Maintained, widely used |
| `jackc/pgx/v5` | v5.8.0 | PostgreSQL driver | Maintained, de facto standard |
| `golang.org/x/crypto` | v0.49.0 | bcrypt password hashing | Official Go extended library |
| `gorilla/websocket` | v1.5.3 | WebSocket connections | Widely used, community-maintained since Gorilla archive |

The dependency set is small and consists of well-established libraries. No known
vulnerabilities at the time of this review.

### 10.2 Frontend Dependencies

The frontend uses standard React, Zustand, Vite, and Tailwind. The Tauri shell is Rust-based.
No unusual or high-risk dependencies observed.

---

## 11. Known Gaps and Risks

### 11.1 Critical Gaps

None. For a proof-of-concept/early-stage game self-hosted on a single server, the security
posture is reasonable. The server-authoritative design is the most important security
property and it is well-implemented.

### 11.2 High-Priority Gaps (address before any public launch)

1. **No email verification on registration.** Anyone can register with any email address.
   This enables account squatting and makes account recovery impossible.

2. **No token refresh or revocation mechanism.** Compromised tokens are valid for 24 hours
   with no way to invalidate them. At minimum, a token version or generation counter in the
   database would allow server-side revocation.

3. **WebSocket origin allowlist includes dev origins unconditionally.** The WebSocket
   `CheckOrigin` function allows localhost and a specific LAN IP in all environments, not
   just when `ENV != "production"`.

4. **CSP disabled in Tauri.** The `"csp": null` in `tauri.conf.json` disables Content
   Security Policy entirely. While Tauri's webview is sandboxed from the OS, a CSP would
   provide defense-in-depth against XSS.

5. **Database connection unencrypted (`sslmode=disable`).** Acceptable for same-host
   deployment but must be addressed before any network-separated deployment.

### 11.3 Medium-Priority Gaps

6. **`X-Forwarded-For` trusted unconditionally.** If the Go server were ever exposed
   directly to the internet, rate limiting could be bypassed via header spoofing.

7. **Group names lack input validation parity with display names.** No profanity filter,
   no character restrictions beyond length.

8. **No password complexity requirements beyond length.** The 8-character minimum is a
   reasonable floor but many best practices recommend checking against common password lists.

9. **JWT token in WebSocket query parameter.** Token appears in URLs, potentially logged
   by nginx or other access log consumers.

10. **No account lockout or progressive delay after failed logins.** The 10/min IP rate
    limit provides basic protection, but credential stuffing from distributed IPs would not
    be mitigated.

11. **Hardware `DeleteByID` query lacks game_state_id scoping.** Safe in practice due to
    handler-level validation, but missing defense-in-depth.

12. **`leaderboard/update` is a public authenticated endpoint.** Any user can trigger a
    leaderboard score update. This is benign given server-authoritative scores but is
    unnecessary attack surface.

### 11.4 Low-Priority / Future Considerations

13. **No audit logging.** No record of authentication events, administrative actions, or
    security-relevant operations.

14. **No HTTPS enforcement headers.** No `Strict-Transport-Security`, `X-Frame-Options`, or
    `X-Content-Type-Options` headers set by the Go application. These may be set by nginx
    but are not verified in this codebase.

15. **No automated dependency vulnerability scanning.** No Dependabot, Snyk, or equivalent
    configured.

16. **No backend test suite.** Zero test files exist in `apps/backend/`. Security-relevant
    behavior (auth, authorization, input validation) is not tested.

17. **OAuth providers planned but not implemented.** The schema supports it; the code does not.

18. **`ENV` variable not set defaults to non-production behavior.** CORS and other
    environment-conditional logic defaults to permissive (dev) mode if `ENV` is not
    explicitly set to `"production"`.

---

## 12. Security Architecture Summary

The codebase implements a straightforward and sound security model for its maturity level:

- **Server-authoritative game logic** is the core security property and is correctly
  implemented. The client cannot fabricate game state.
- **Authentication** uses industry-standard JWT + bcrypt. No novel crypto.
- **Authorization** is user-scoped through the middleware -> handler -> query chain.
- **SQL injection** risk is negligible due to consistent use of parameterized queries.
- **Rate limiting** provides basic abuse protection for auth and game action endpoints.
- **Secret management** uses environment variables with `.env` files excluded from version
  control.
- **CORS** is origin-whitelisted with both HTTP and WebSocket checks.

The primary areas for hardening before a public launch are: email verification, token
revocation, environment-aware WebSocket origin checks, and CSP configuration in Tauri.
