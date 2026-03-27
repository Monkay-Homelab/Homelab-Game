---
project: "homelab-the-game"
maturity: "draft"
last_updated: "2026-03-27"
updated_by: "@staff-engineer"
scope: "Horizontal scaling of the Go backend across N Docker Swarm replicas via Redis externalization of in-memory state, DB-level user locking, and Traefik sticky sessions"
owner: "@staff-engineer"
dependencies:
  - ../spec/architecture.md
  - ../spec/operations.md
  - ../spec/performance.md
  - tick-performance-optimizations.md
  - websocket-state-push.md
  - websocket-actions.md
  - migrate-kubernetes.md
---

# TDD: Horizontal Scaling of the Backend

## 1. Problem Statement

### What

The Homelab the Game backend is a single-process Go server where all concurrent-access state
lives in-memory: per-user action mutexes (`userMutexMap`), WebSocket client connections
(`Hub.clients`), rate limiter visitor counts (`rateLimiter.visitors`), the global donated CU
cache (`GlobalDonatedCUCache`), and the bitcoin price update loop (`PriceService.mu`). Running
more than one backend replica produces data races, lost WebSocket messages, bypassed rate limits,
and duplicate bitcoin price updates.

This TDD designs the changes required to run N backend replicas behind Traefik on Docker Swarm
with full correctness, zero data loss, and zero downtime deploys.

### Why Now

The Kubernetes migration TDD (`docs/tdd/migrate-kubernetes.md`) and docker-stack.yml already
define the container orchestration layer. The backend image is built and deployed via
`docker-stack.yml` with `replicas: 1`. The game is live at `api.homelab.living`. Scaling to
multiple replicas is the next operational milestone -- it provides fault tolerance (a single
replica crash does not take the game offline), enables zero-downtime rolling deploys, and
removes the single-process ceiling identified in the performance spec (4,500 actions/sec
throughput cap at `docs/spec/performance.md` Section 9).

### Constraints

- Docker Swarm orchestration (not Kubernetes). The `docker-stack.yml` is the deployment artifact.
- Traefik is the ingress proxy (replacing the current direct port mapping `8080:8080`).
- PostgreSQL remains a single instance (`db` service in the stack). No read replicas.
- Redis is introduced as a new infrastructure dependency (sidecar service in the stack).
- All existing features must work identically. The client is unaware of multi-replica topology.
- No data races under concurrent multi-replica operation.
- No lost WebSocket messages when a user's connection is on replica A and a state mutation
  occurs on replica B.
- Zero downtime deploys via Swarm rolling updates with Traefik health checks.
- The bitcoin price update loop must run on exactly one replica at any time.

### Acceptance Criteria

1. `docker stack deploy` with `replicas: N` (N >= 2) produces a fully functional game with
   no behavioral differences from single-replica.
2. Two concurrent `PerformAction` requests for the same user, hitting different replicas,
   are correctly serialized (no lost updates, no double-spend).
3. A WebSocket message pushed by replica A is received by a client connected to replica B.
4. Rate limiting correctly tracks request counts across all replicas.
5. The global donated CU value is consistent across all replicas within the cache TTL.
6. Exactly one replica runs the bitcoin price update loop at any time, with automatic
   failover within 30 seconds if the leader dies.
7. Rolling deploys (`docker service update --image ...`) complete with zero client-visible
   errors (existing WebSocket connections drain gracefully, new connections route to new
   replicas).
8. The system degrades gracefully if Redis is temporarily unavailable (rate limiting falls
   back to permissive, CU cache serves stale values, WebSocket messages are delivered locally
   only).

## 2. Context and Prior Art

### Current Architecture (Single-Process)

```
                +------------------+
                |     Traefik      | (or direct port mapping today)
                +--------+---------+
                         |
                +--------v---------+
                |   Go Backend     |
                |  (1 replica)     |
                |                  |
                | userMutexMap     |  <-- in-memory per-user locks
                | Hub.clients      |  <-- in-memory WS connections
                | rateLimiter      |  <-- in-memory request counters
                | GlobalCUCache    |  <-- in-memory cached aggregate
                | PriceService.mu  |  <-- in-memory price update lock
                | tickStateMap     |  <-- in-memory per-user tick cache
                +--------+---------+
                         |
                +--------v---------+
                |   PostgreSQL     |
                |  + TimescaleDB   |
                +------------------+
```

**In-memory state inventory** (from codebase analysis):

| State | Location | Purpose | Multi-Replica Risk |
|-------|----------|---------|-------------------|
| `userMutexMap` | `handlers/game.go:26-75` | Serializes per-user game actions | Two replicas process the same user concurrently -- data race on game_states |
| `Hub.clients` | `ws/hub.go:72-74` | Maps userID to WebSocket connection | User connects to replica A; mutation on replica B cannot push state to the user |
| `rateLimiter.visitors` | `middleware/ratelimit.go:10-22` | Request count per IP/user | Each replica counts independently -- effective limit is N * configured limit |
| `GlobalDonatedCUCache` | `handlers/cu_cache.go:16-20` | Cached SUM(total_donated_cu) | Each replica has its own cache refreshing independently -- minor inconsistency |
| `PriceService.mu` | `bitcoin/price.go:71-74` | Serializes price advancement | Every replica runs `GetCurrentPrice` and advances the price -- duplicate steps, seed corruption |
| `tickStateMap` | `handlers/game.go:107-141` | Per-user dirty flag and cached tick data | Tick goroutine runs on the replica where the user's WS is connected -- safe IF WS is sticky |

### Target Architecture (Multi-Replica)

```
                +------------------+
                |     Traefik      |  <-- sticky sessions (cookie)
                +---+---------+----+
                    |         |
          +---------v---+ +---v---------+
          | Backend (A) | | Backend (B) |  ... (N replicas)
          |             | |             |
          | Hub.clients | | Hub.clients |  <-- local WS connections only
          | tickStateMap| | tickStateMap|  <-- local per connected user
          +------+------+ +------+------+
                 |    \    /    |
                 |     \  /     |
                 |   +--v---+   |
                 |   | Redis |  |  <-- pub/sub, rate limits, CU cache, leader election
                 |   +-------+  |
                 |              |
          +------v--------------v------+
          |        PostgreSQL           |
          | SELECT ... FOR UPDATE      |  <-- per-user row locking
          +----------------------------+
```

### How Solved Elsewhere

- **User action locking via database row locks**: Standard pattern in server-authoritative
  multiplayer games. `SELECT ... FOR UPDATE` on the user's game_states row within a
  transaction serializes all mutations for that user across any number of application servers.
  Used by EVE Online's cluster architecture and numerous MMORPG backends.
- **Redis pub/sub for WebSocket fan-out**: The standard approach for multi-node real-time
  systems. Socket.IO uses Redis adapter for exactly this. `go-redis` provides a clean pub/sub
  API.
- **Redis-based distributed rate limiting**: The `INCR` + `EXPIRE` pattern is the textbook
  approach. Used by Cloudflare, Kong, and most API gateway implementations.
- **Redis leader election via `SET NX EX`**: A lightweight alternative to Raft/consensus
  for single-leader tasks. Documented in the Redis official docs as the "Redlock" simplified
  pattern. Appropriate when brief dual-leader windows are acceptable (the bitcoin price model
  is idempotent given its database-stored seed).
- **Sticky sessions for WebSocket**: Traefik's built-in sticky cookie support routes all
  requests from a client to the same backend, ensuring the WebSocket connection and HTTP
  actions from the same user land on the same replica. This keeps the `tickStateMap` cache
  and `Hub.clients` map effective.

## 3. Alternatives Considered

### Alternative A: PostgreSQL-Only (No Redis)

Externalize all state to PostgreSQL: row locks for user actions, advisory locks for leader
election, a `rate_limits` table for request counting, pg_notify for WebSocket pub/sub.

**Strengths**: No new infrastructure dependency. PostgreSQL is already deployed and trusted.
Advisory locks (`pg_advisory_lock`) are lightweight and transactional.

**Weaknesses**: pg_notify is unreliable for WebSocket fan-out -- it drops messages if the
notify queue fills (8GB default, but no delivery guarantee or backpressure). Rate limiting
via database writes would add significant load to an already busy PostgreSQL instance
(the performance spec identifies DB query volume as the primary bottleneck). A `rate_limits`
table would produce thousands of writes per second. PostgreSQL advisory locks require a
dedicated connection per lock, which conflicts with connection pool sharing.

**Verdict**: Rejected. PostgreSQL is the wrong tool for high-frequency ephemeral state (rate
counters, pub/sub). It would shift the bottleneck from application memory to database I/O.

### Alternative B: Redis for Everything (Recommended)

Add Redis as a sidecar service. Use it for rate limiting (INCR+EXPIRE), CU cache (GET/SET
with TTL), WebSocket pub/sub (SUBSCRIBE/PUBLISH), and leader election (SET NX EX). Keep
user action locking in PostgreSQL (SELECT FOR UPDATE) since the database transaction is
already required for the read-modify-write cycle.

**Strengths**: Redis is purpose-built for all four use cases. Single new dependency handles
all cross-replica coordination except user locking. go-redis/v9 is the mature, well-maintained
Go client. Redis adds <1ms latency per operation. The pub/sub model maps cleanly to the
existing Hub architecture.

**Weaknesses**: New infrastructure dependency (Redis) that must be deployed, monitored, and
kept available. If Redis goes down, rate limiting and WebSocket cross-replica delivery degrade
(but the game continues to function in degraded mode).

**Verdict**: Recommended. The Redis operational cost is low (single instance, no clustering
needed at this scale), and it is the right tool for each of the four use cases.

### Alternative C: Embedded NATS or Similar Message Broker

Use NATS for pub/sub and rate limiting, with PostgreSQL for user locking.

**Strengths**: NATS has excellent Go support, built-in pub/sub, and JetStream for persistence.

**Weaknesses**: Overkill. NATS is a full message broker; we need simple pub/sub and key-value
operations. Redis covers all four use cases; NATS would leave rate limiting and CU caching
to another solution. Adding NATS means two new dependencies instead of one.

**Verdict**: Rejected. Redis handles all needs. NATS would be justified if we needed durable
message queuing, which we do not.

### Decision

**Alternative B (Redis for rate limiting, CU cache, pub/sub, leader election; PostgreSQL
SELECT FOR UPDATE for user action locking)**. This minimizes new infrastructure while using
each system for what it does best.

## 4. Architecture and System Design

### 4.1 New Dependency: Redis Client

**Go module**: `github.com/redis/go-redis/v9`

This is the official Redis Go client maintained by the Redis team. It provides connection
pooling, pub/sub, pipelining, and scripting. It replaced the older `go-redis/redis` path
and is the recommended client for Go 1.21+.

**Configuration additions to `config.Config`**:

```go
// In internal/config/config.go

type Config struct {
    // ... existing fields ...
    RedisAddr     string // Redis address (host:port)
    RedisPassword string // Redis password (optional)
    RedisDB       int    // Redis database number (default 0)
}

func Load() *Config {
    return &Config{
        // ... existing ...
        RedisAddr:     getEnv("REDIS_ADDR", "redis:6379"),
        RedisPassword: getSecret("REDIS_PASSWORD", "redis_password", ""),
        RedisDB:       0,
    }
}
```

**Redis client initialization in `main.go`**:

```go
import "github.com/redis/go-redis/v9"

rdb := redis.NewClient(&redis.Options{
    Addr:     cfg.RedisAddr,
    Password: cfg.RedisPassword,
    DB:       cfg.RedisDB,
})
// Verify connectivity
if err := rdb.Ping(context.Background()).Err(); err != nil {
    log.Fatalf("Failed to connect to Redis: %v", err)
}
defer rdb.Close()
```

### 4.2 Component 1: User Action Locking (SELECT ... FOR UPDATE)

**Current state**: `userMutexMap` (`handlers/game.go:26-75`) is an in-memory `map[string]*userLock`
protected by a global `sync.Mutex`. `Lock(userID)` acquires the per-user mutex before any
game state read/modify/write cycle. Used by `PerformAction` (line 743-744), `HandleWSAction`
(line 1122), and `runUserTick` (line 381).

**Problem**: Two replicas can simultaneously acquire the "lock" for the same user because
each replica has its own independent mutex map.

**Design**: Replace the in-memory mutex with PostgreSQL row-level locking using
`SELECT ... FOR UPDATE` within a database transaction. The `game_states` row for the user
serves as the lock target.

**Interface**:

```go
// In internal/database/queries/game_state.go (modified method)

// GetByUserIDForUpdate loads the game state within the given transaction,
// acquiring a row-level exclusive lock. Other transactions attempting to
// read the same row FOR UPDATE will block until this transaction commits
// or rolls back.
func (q *GameStateQueries) GetByUserIDForUpdate(ctx context.Context, tx pgx.Tx, userID string) (*models.GameState, error) {
    row := tx.QueryRow(ctx,
        `SELECT /* all columns */ FROM game_states WHERE user_id = $1 FOR UPDATE`,
        userID,
    )
    // ... scan into models.GameState ...
}
```

**Transaction lifecycle in handlers**:

Each action processing path (`PerformAction`, `HandleWSAction`, `runUserTick`) will:

1. `tx, err := pool.Begin(ctx)` -- start a transaction
2. `gs, err := gameState.GetByUserIDForUpdate(ctx, tx, userID)` -- lock the row
3. Load child tables (hardware, services, etc.) within the same transaction
4. Run engine processing (ProcessIdleProgress, ProcessAction)
5. Persist all changes within the transaction
6. `tx.Commit(ctx)` -- release the lock

The in-memory `userMutexMap` is retained as a **local fast-path optimization**: it prevents
multiple goroutines on the same replica from competing for the database lock, reducing DB
lock contention. The database lock is the correctness guarantee; the in-memory lock is a
performance optimization.

**Modified `LoadFullGameState` in `queries/batch.go`**:

```go
// LoadFullGameStateForUpdate loads all game data within a transaction,
// acquiring a FOR UPDATE lock on the game_states row.
func LoadFullGameStateForUpdate(ctx context.Context, tx pgx.Tx, userID string) (*FullGameData, error) {
    // Phase 1: Lock and load game_states
    gs, err := scanGameState(tx.QueryRow(ctx,
        `SELECT /* columns */ FROM game_states WHERE user_id = $1 FOR UPDATE`, userID))
    if err != nil {
        return nil, err
    }

    // Phase 2: Batch load child tables (within same transaction, so lock is held)
    batch := &pgx.Batch{}
    batch.Queue(`SELECT /* columns */ FROM hardware WHERE game_state_id = $1`, gs.ID)
    batch.Queue(`SELECT /* columns */ FROM services WHERE game_state_id = $1`, gs.ID)
    // ... remaining child tables ...

    br := tx.SendBatch(ctx, batch)
    defer br.Close()

    // ... scan results ...
    return &FullGameData{GameState: gs, ...}, nil
}
```

**Files modified**:
- `internal/database/queries/game_state.go` -- add `GetByUserIDForUpdate` method
- `internal/database/queries/batch.go` -- add `LoadFullGameStateForUpdate` that takes `pgx.Tx`
- `internal/api/handlers/game.go` -- modify `PerformAction`, `HandleWSAction`, `runUserTick` to use transactions
- `internal/api/handlers/game.go` -- retain `userMutexMap` as local optimization with a TODO to remove it if profiling shows it is unnecessary

**Transaction timeout**: All transactions must have a context timeout to prevent long-held
locks. Use `context.WithTimeout(ctx, 10*time.Second)` as the ceiling. The existing tick
timeout is already 10 seconds (line 623).

**Deadlock prevention**: Since we always lock only one row (the user's game_states row) and
always lock it first before any child table reads, there is no deadlock risk. PostgreSQL's
deadlock detector (default 1s timeout) provides a safety net.

**Tests needed**:
- Integration test: two goroutines performing concurrent actions for the same user verify
  serialized execution (no lost compute units).
- Test that transaction timeout correctly rolls back and releases the lock.
- Test that a failed transaction does not leave the game state in a partial state.

### 4.3 Component 2: WebSocket Presence and Cross-Replica Push (Redis Pub/Sub)

**Current state**: `Hub` in `ws/hub.go` maintains `clients map[string]*Client` (line 74).
`SendToUser(userID, msg)` (line 253) looks up the client in the local map and writes to
its send channel. If the user is not connected to this replica, the message is silently
dropped.

**Problem**: When a user's game state is mutated by replica B (e.g., a group member's action
triggers a bonus recalculation), replica B cannot push the updated state to the user connected
on replica A.

**Design**: Each replica subscribes to a Redis pub/sub channel. When any replica needs to push
a message to a user, it first checks the local Hub; if the user is connected locally, deliver
directly. If not, publish to the Redis channel. All replicas receive the published message and
check their local Hub.

**New interface and implementation**:

```go
// In internal/api/ws/pubsub.go (new file)

// MessageBroadcaster abstracts the mechanism for sending messages to users.
// In single-replica mode, this delegates directly to the Hub.
// In multi-replica mode, this publishes to Redis pub/sub.
type MessageBroadcaster interface {
    // SendToUser sends a message to the specified user, regardless of which
    // replica they are connected to.
    SendToUser(userID string, msg Message)

    // SendToUserBytes sends pre-serialized bytes to the specified user.
    SendToUserBytes(userID string, data []byte)

    // Start begins listening for messages from other replicas.
    // Must be called once during server startup.
    Start(ctx context.Context) error

    // Stop shuts down the broadcaster. Called during graceful shutdown.
    Stop()
}
```

**Redis-backed implementation**:

```go
// In internal/api/ws/redis_broadcaster.go (new file)

// RedisBroadcaster implements MessageBroadcaster using Redis pub/sub.
// Each replica subscribes to "ws:broadcast". When SendToUser is called:
//   1. Check local Hub -- if user is connected locally, deliver directly.
//   2. If not local, publish {userID, data} to "ws:broadcast".
// On receiving a published message, check local Hub and deliver if present.
type RedisBroadcaster struct {
    hub    *Hub
    rdb    *redis.Client
    pubsub *redis.PubSub
    done   chan struct{}
}

const wsBroadcastChannel = "ws:broadcast"

type broadcastMessage struct {
    UserID string `json:"u"`
    Data   []byte `json:"d"`
}

func NewRedisBroadcaster(hub *Hub, rdb *redis.Client) *RedisBroadcaster {
    return &RedisBroadcaster{
        hub:  hub,
        rdb:  rdb,
        done: make(chan struct{}),
    }
}

func (b *RedisBroadcaster) SendToUser(userID string, msg Message) {
    // Fast path: deliver locally if connected
    if b.hub.HasUser(userID) {
        b.hub.SendToUser(userID, msg)
        return
    }

    // Slow path: publish to Redis for other replicas
    data, err := json.Marshal(msg)
    if err != nil {
        return
    }
    bm := broadcastMessage{UserID: userID, Data: data}
    payload, _ := json.Marshal(bm)
    b.rdb.Publish(context.Background(), wsBroadcastChannel, payload)
}

func (b *RedisBroadcaster) Start(ctx context.Context) error {
    b.pubsub = b.rdb.Subscribe(ctx, wsBroadcastChannel)
    go b.listen()
    return nil
}

func (b *RedisBroadcaster) listen() {
    ch := b.pubsub.Channel()
    for {
        select {
        case msg, ok := <-ch:
            if !ok {
                return
            }
            var bm broadcastMessage
            if json.Unmarshal([]byte(msg.Payload), &bm) == nil {
                b.hub.SendToUserBytes(bm.UserID, bm.Data)
            }
        case <-b.done:
            return
        }
    }
}
```

**Hub modifications**:

Add a `HasUser(userID string) bool` method to `Hub` for local connection checking:

```go
// In ws/hub.go

func (h *Hub) HasUser(userID string) bool {
    h.mu.RLock()
    _, ok := h.clients[userID]
    h.mu.RUnlock()
    return ok
}
```

**Integration with GameHandler**:

The `GameHandler` currently calls `h.hub.SendToUser(...)` in multiple places:
- `pushEvents` (line 240-248)
- `runFullTick` (line 481-484)
- `runLightTick` (line 587-590)
- `PerformAction` (line 1016-1020)
- `HandleWSAction` (multiple locations)

All these call sites change from `h.hub.SendToUser` to `h.broadcaster.SendToUser`, where
`broadcaster` is a new field on `GameHandler` of type `MessageBroadcaster`. In single-replica
mode, the broadcaster can be a thin wrapper around the Hub that implements the same interface
without Redis.

**Graceful degradation**: If Redis is unavailable, `Publish` calls will fail silently (logged).
Messages will still be delivered locally. Users connected to the replica where the mutation
happens will see updates immediately; users on other replicas will see updates on their next
tick or HTTP request.

**Files modified**:
- `internal/api/ws/hub.go` -- add `HasUser` method
- `internal/api/ws/pubsub.go` -- new file, `MessageBroadcaster` interface
- `internal/api/ws/redis_broadcaster.go` -- new file, Redis pub/sub implementation
- `internal/api/handlers/game.go` -- replace `h.hub.SendToUser*` with `h.broadcaster.SendToUser*`
- `cmd/server/main.go` -- create `RedisBroadcaster`, pass to `GameHandler`

**Tests needed**:
- Unit test: `RedisBroadcaster` publishes to Redis when user is not local.
- Unit test: `RedisBroadcaster` delivers locally when user is connected.
- Integration test: message published by one broadcaster instance is received by another.
- Test: Redis unavailability does not crash the broadcaster (logs error, continues).

### 4.4 Component 3: Distributed Rate Limiting (Redis INCR + EXPIRE)

**Current state**: `rateLimiter` in `middleware/ratelimit.go:10-22` uses an in-memory
`map[string]*visitor` with a background cleanup goroutine. `checkRate` (line 54-66)
increments a counter and checks against the limit.

**Problem**: Each replica maintains its own counter. A user sending 10 requests gets rate
limited on a single replica but can send 10 * N requests across N replicas.

**Design**: Replace the in-memory map with Redis `INCR` + `EXPIRE`. Each rate limit check
becomes a single Redis command.

**New interface**:

```go
// In internal/api/middleware/ratelimit.go (modified)

// RateLimitStore abstracts the storage backend for rate limiting.
type RateLimitStore interface {
    // CheckRate returns true if the request should be allowed.
    // Implementations must atomically increment the counter and check the limit.
    CheckRate(ctx context.Context, key string, maxPerMinute int) bool
}
```

**Redis implementation**:

```go
// In internal/api/middleware/redis_ratelimit.go (new file)

type RedisRateLimitStore struct {
    rdb *redis.Client
}

func NewRedisRateLimitStore(rdb *redis.Client) *RedisRateLimitStore {
    return &RedisRateLimitStore{rdb: rdb}
}

// CheckRate uses Redis INCR + EXPIRE for distributed rate limiting.
// The key expires after 60 seconds (the rate limit window).
// Uses a Lua script for atomic increment-and-check to avoid race conditions.
func (s *RedisRateLimitStore) CheckRate(ctx context.Context, key string, maxPerMinute int) bool {
    // Lua script: increment, set expire on first increment, return count
    script := redis.NewScript(`
        local count = redis.call('INCR', KEYS[1])
        if count == 1 then
            redis.call('EXPIRE', KEYS[1], 60)
        end
        return count
    `)

    result, err := script.Run(ctx, s.rdb, []string{"rl:" + key}, maxPerMinute).Int()
    if err != nil {
        // Redis unavailable -- fail open (allow the request)
        log.Printf("[ratelimit] Redis error, failing open: %v", err)
        return true
    }
    return result <= maxPerMinute
}
```

**In-memory fallback implementation** (for single-replica or Redis-down scenarios):

```go
// In internal/api/middleware/ratelimit.go (modified existing code)

type InMemoryRateLimitStore struct {
    mu       sync.Mutex
    visitors map[string]*visitor
}

func NewInMemoryRateLimitStore() *InMemoryRateLimitStore {
    s := &InMemoryRateLimitStore{visitors: make(map[string]*visitor)}
    go s.cleanup()
    return s
}

func (s *InMemoryRateLimitStore) CheckRate(_ context.Context, key string, maxPerMinute int) bool {
    // ... existing checkRate logic, moved to this method ...
}
```

**Middleware integration**:

The `RateLimit`, `RateLimitNamed`, `RateLimitByUser`, and `CheckGameActionRate` functions
are modified to accept a `RateLimitStore` parameter (or use a package-level variable set
during initialization):

```go
// In middleware/ratelimit.go

var store RateLimitStore = NewInMemoryRateLimitStore() // default

func SetRateLimitStore(s RateLimitStore) {
    store = s
}

func checkRate(key string, maxPerMinute int) bool {
    return store.CheckRate(context.Background(), key, maxPerMinute)
}
```

This preserves the existing function signatures for `RateLimit`, `RateLimitNamed`, etc.
The store is swapped at startup in `main.go`.

**Files modified**:
- `internal/api/middleware/ratelimit.go` -- extract `RateLimitStore` interface, refactor
  existing code into `InMemoryRateLimitStore`
- `internal/api/middleware/redis_ratelimit.go` -- new file, Redis implementation
- `cmd/server/main.go` -- set the rate limit store based on Redis availability

**Tests needed**:
- Unit test: `RedisRateLimitStore` correctly limits after maxPerMinute requests.
- Unit test: `RedisRateLimitStore` resets after 60-second window.
- Unit test: Redis failure causes `CheckRate` to return true (fail open).
- Existing tests continue to work with `InMemoryRateLimitStore`.

### 4.5 Component 4: Global Donated CU Cache (Redis Key with TTL)

**Current state**: `GlobalDonatedCUCache` in `handlers/cu_cache.go:16-80` stores the cached
value in a `sync.RWMutex`-protected `int64`. A background goroutine refreshes it every 30
seconds from PostgreSQL. The `Add` method (line 76) bumps the cached value immediately after
a donation.

**Problem**: Each replica maintains its own cache, refreshing independently. The values are
eventually consistent (within 30 seconds) but diverge between replicas. The `Add` method
only updates the local cache, so other replicas do not see the immediate bump.

**Design**: Replace the local cache with a Redis key. The refresh goroutine writes to Redis;
`Get()` reads from Redis. The `Add` method uses `INCRBY` to atomically update the shared
value.

**Modified implementation**:

```go
// In handlers/cu_cache.go (modified)

type GlobalDonatedCUCache struct {
    pool         *pgxpool.Pool
    rdb          *redis.Client
    localValue   int64        // fallback when Redis is unavailable
    mu           sync.RWMutex // protects localValue only
}

const cuCacheKey = "global:donated_cu"

func NewGlobalDonatedCUCache(pool *pgxpool.Pool, rdb *redis.Client, refreshInterval time.Duration) *GlobalDonatedCUCache {
    c := &GlobalDonatedCUCache{pool: pool, rdb: rdb}

    // Blocking initial load
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    c.refresh(ctx)

    go func() {
        ticker := time.NewTicker(refreshInterval)
        defer ticker.Stop()
        for range ticker.C {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            c.refresh(ctx)
            cancel()
        }
    }()

    return c
}

func (c *GlobalDonatedCUCache) Get() int64 {
    if c.rdb != nil {
        val, err := c.rdb.Get(context.Background(), cuCacheKey).Int64()
        if err == nil {
            return val
        }
        // Redis miss or error -- fall back to local value
    }
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.localValue
}

func (c *GlobalDonatedCUCache) refresh(ctx context.Context) {
    var val int64
    err := c.pool.QueryRow(ctx,
        "SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states").Scan(&val)
    if err != nil {
        log.Printf("[cu-cache] refresh error: %v", err)
        return
    }

    // Update both Redis and local fallback
    if c.rdb != nil {
        c.rdb.Set(ctx, cuCacheKey, val, 60*time.Second) // TTL = 2x refresh interval
    }
    c.mu.Lock()
    c.localValue = val
    c.mu.Unlock()
}

func (c *GlobalDonatedCUCache) Add(amount int64) {
    if c.rdb != nil {
        c.rdb.IncrBy(context.Background(), cuCacheKey, amount)
    }
    c.mu.Lock()
    c.localValue += amount
    c.mu.Unlock()
}
```

**Files modified**:
- `internal/api/handlers/cu_cache.go` -- add Redis integration, keep local fallback
- `cmd/server/main.go` -- pass `redis.Client` to `NewGlobalDonatedCUCache`

**Tests needed**:
- Test: `Get()` returns Redis value when available.
- Test: `Get()` falls back to local value when Redis is unavailable.
- Test: `Add()` atomically increments the Redis value.
- Test: `refresh()` writes the correct value to both Redis and local cache.

### 4.6 Component 5: WebSocket Cross-Replica Push (Redis Pub/Sub)

Covered in Section 4.3 above. This is the same component -- listed separately in the
requirements table because the work description separated "WebSocket presence" from
"WebSocket push." The design in 4.3 addresses both: presence checking (local Hub lookup)
and cross-replica push (Redis pub/sub).

### 4.7 Component 6: Bitcoin Price Leader Election (Redis SET NX)

**Current state**: `PriceService` in `bitcoin/price.go:68-83` uses an in-memory `sync.Mutex`
to serialize price advancement. `GetCurrentPrice` (line 94-152) is called from `fetchBitcoinData`
in `game.go` during every tick, every `GetState`, and every `PerformAction`. The in-memory
mutex ensures only one goroutine advances the price at a time.

**Problem**: With multiple replicas, each replica independently calls `GetCurrentPrice`, which
acquires the in-memory mutex locally, reads the seed from DB, advances N steps, and writes
back. Two replicas calling this simultaneously will both read the same seed, compute the same
steps, and write back (last writer wins). While the deterministic seed means they compute the
same price, the race on `UpdatePrice` can corrupt the seed sequence if interleaved with
different `now` timestamps.

**Design**: Only one replica should actively advance the bitcoin price. Other replicas should
read the price from the database without advancing it. This is a leader election problem.

**Leader election via Redis**:

```go
// In internal/game/bitcoin/leader.go (new file)

// PriceLeader manages leader election for the bitcoin price update loop.
// Only the leader replica actively advances the price model.
type PriceLeader struct {
    rdb       *redis.Client
    replicaID string         // unique identifier for this replica
    isLeader  bool
    mu        sync.RWMutex
    done      chan struct{}
}

const (
    leaderKey      = "bitcoin:price_leader"
    leaderTTL      = 15 * time.Second // lease duration
    renewInterval  = 5 * time.Second  // renew before TTL expires
)

func NewPriceLeader(rdb *redis.Client, replicaID string) *PriceLeader {
    return &PriceLeader{
        rdb:       rdb,
        replicaID: replicaID,
        done:      make(chan struct{}),
    }
}

// Start begins the leader election loop. Call once at startup.
func (l *PriceLeader) Start(ctx context.Context) {
    go l.electionLoop(ctx)
}

func (l *PriceLeader) electionLoop(ctx context.Context) {
    ticker := time.NewTicker(renewInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            l.tryAcquireOrRenew(ctx)
        case <-l.done:
            // Release leadership on shutdown
            l.rdb.Del(ctx, leaderKey)
            return
        case <-ctx.Done():
            return
        }
    }
}

func (l *PriceLeader) tryAcquireOrRenew(ctx context.Context) {
    // Try to acquire (SET NX) or renew (SET XX if we are the current holder)
    ok, err := l.rdb.SetNX(ctx, leaderKey, l.replicaID, leaderTTL).Result()
    if err != nil {
        return
    }

    if ok {
        // Acquired leadership
        l.mu.Lock()
        l.isLeader = true
        l.mu.Unlock()
        return
    }

    // Did not acquire -- check if we already hold it and renew
    holder, err := l.rdb.Get(ctx, leaderKey).Result()
    if err != nil {
        return
    }
    if holder == l.replicaID {
        // We are the leader -- renew TTL
        l.rdb.Expire(ctx, leaderKey, leaderTTL)
        l.mu.Lock()
        l.isLeader = true
        l.mu.Unlock()
    } else {
        l.mu.Lock()
        l.isLeader = false
        l.mu.Unlock()
    }
}

func (l *PriceLeader) IsLeader() bool {
    l.mu.RLock()
    defer l.mu.RUnlock()
    return l.isLeader
}

func (l *PriceLeader) Stop() {
    close(l.done)
}
```

**PriceService modification**:

```go
// In bitcoin/price.go (modified)

type PriceService struct {
    store  PriceStore
    mu     sync.Mutex
    config PriceConfig
    leader *PriceLeader // nil in single-replica mode
}

func (s *PriceService) GetCurrentPrice(ctx context.Context, now time.Time) (int64, error) {
    // If leader election is configured and we are NOT the leader,
    // just read the current price without advancing.
    if s.leader != nil && !s.leader.IsLeader() {
        state, err := s.store.GetPrice(ctx)
        if err != nil {
            return 0, fmt.Errorf("get bitcoin price state: %w", err)
        }
        return state.CurrentPrice, nil
    }

    // Leader path (or single-replica mode): advance the price
    s.mu.Lock()
    defer s.mu.Unlock()
    // ... existing advancement logic ...
}
```

**Replica ID generation in `main.go`**:

```go
// Generate a unique replica ID from hostname (Docker Swarm task slot)
hostname, _ := os.Hostname()
replicaID := fmt.Sprintf("backend-%s-%d", hostname, time.Now().UnixNano())
```

**Failover behavior**: If the leader replica dies, its Redis key expires after 15 seconds.
Another replica acquires leadership on its next 5-second election tick. Maximum gap in
price advancement: ~20 seconds (15s TTL + 5s election interval). The price model handles
this gracefully via its catch-up mechanism (computes missed steps on next call).

**Files modified**:
- `internal/game/bitcoin/leader.go` -- new file, leader election
- `internal/game/bitcoin/price.go` -- add leader check to `GetCurrentPrice`
- `cmd/server/main.go` -- create `PriceLeader`, pass to `PriceService`

**Tests needed**:
- Test: only one of two PriceLeader instances becomes leader.
- Test: leader crash (Stop without Del) results in failover after TTL.
- Test: non-leader PriceService returns current price without advancing.
- Test: leader PriceService advances price normally.

### 4.8 Component 7: Sticky Sessions and Infrastructure (Traefik + Docker Swarm)

**Current state**: `docker-stack.yml` exposes the backend on port 8080 directly. There is no
reverse proxy in the stack (nginx sits outside, per CLAUDE.md).

**Problem**: Without session stickiness, a user's HTTP requests and WebSocket connection may
land on different replicas. The `tickStateMap` cache becomes useless if actions go to a
different replica than the one running the tick goroutine. While the database locking (4.2)
ensures correctness, sticky sessions ensure performance (local cache hits, local WebSocket
delivery).

**Design**: Add Traefik as the ingress proxy within the Docker stack. Configure sticky cookies
so that all requests from a client route to the same backend replica. WebSocket upgrades
inherit the sticky cookie.

**Modified `docker-stack.yml`**:

```yaml
version: "3.8"

services:
  # ---- Traefik Ingress ----
  traefik:
    image: traefik:v3.3
    command:
      - "--providers.swarm=true"
      - "--providers.swarm.exposedByDefault=false"
      - "--entrypoints.web.address=:80"
      - "--entrypoints.websecure.address=:443"
      - "--api.dashboard=false"
      - "--ping=true"
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    networks:
      - frontend
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role == manager

  # ---- Backend API ----
  backend:
    image: ghcr.io/monkay-homelab/homelab-game-backend:latest
    environment:
      PORT: "8080"
      ENV: production
      DB_HOST: db
      DB_PORT: "5432"
      DB_USER: homelab_game
      DB_PASSWORD: ${DB_PASSWORD}
      DB_NAME: homelab_game
      DB_SSLMODE: disable
      JWT_SECRET: ${JWT_SECRET}
      REDIS_ADDR: "redis:6379"
    networks:
      - backend
      - frontend
    deploy:
      replicas: 2  # <-- scaled to N
      update_config:
        parallelism: 1
        delay: 10s
        order: start-first  # new replica starts before old one stops
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 5
      labels:
        - "traefik.enable=true"
        # HTTP routing
        - "traefik.http.routers.backend.rule=Host(`api.homelab.living`)"
        - "traefik.http.routers.backend.entrypoints=websecure"
        - "traefik.http.services.backend.loadbalancer.server.port=8080"
        # Sticky sessions
        - "traefik.http.services.backend.loadbalancer.sticky.cookie=true"
        - "traefik.http.services.backend.loadbalancer.sticky.cookie.name=_backend_affinity"
        - "traefik.http.services.backend.loadbalancer.sticky.cookie.httponly=true"
        - "traefik.http.services.backend.loadbalancer.sticky.cookie.secure=true"
        - "traefik.http.services.backend.loadbalancer.sticky.cookie.samesite=strict"
        # Health check
        - "traefik.http.services.backend.loadbalancer.healthcheck.path=/health"
        - "traefik.http.services.backend.loadbalancer.healthcheck.interval=5s"
        - "traefik.http.services.backend.loadbalancer.healthcheck.timeout=3s"
    healthcheck:
      test: ["CMD", "/healthcheck"]
      interval: 10s
      timeout: 3s
      start_period: 10s
      retries: 3

  # ---- Redis ----
  redis:
    image: redis:7-alpine
    command: >
      redis-server
      --maxmemory 128mb
      --maxmemory-policy allkeys-lru
      --save ""
      --appendonly no
    networks:
      - backend
    deploy:
      replicas: 1
      placement:
        constraints:
          - node.role == manager
      restart_policy:
        condition: on-failure
        delay: 5s
        max_attempts: 5
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 10s
      timeout: 3s
      start_period: 5s
      retries: 3

  # ---- Database (unchanged) ----
  db:
    image: timescale/timescaledb:latest-pg16
    # ... existing configuration ...

  # ---- Frontend (unchanged) ----
  frontend:
    image: ghcr.io/monkay-homelab/homelab-game-frontend:latest
    # ... existing configuration ...

  # ---- Migrations (unchanged) ----
  migrations:
    image: ghcr.io/monkay-homelab/homelab-game-migrations:latest
    # ... existing configuration ...

networks:
  backend:
    driver: overlay
  frontend:
    driver: overlay

volumes:
  pgdata:
    driver: local
```

**Redis configuration rationale**:
- `maxmemory 128mb`: Sufficient for rate limit keys, pub/sub, CU cache, and leader election.
  All data is ephemeral.
- `maxmemory-policy allkeys-lru`: If memory is exhausted, evict least-recently-used keys.
  Rate limit keys are the most numerous and least critical.
- `save ""` and `appendonly no`: No persistence. All Redis data is ephemeral and reconstructible.
  On Redis restart, rate limits reset (acceptable), CU cache refreshes within 30 seconds,
  leader re-election happens within 20 seconds.
- Single replica, pinned to manager node (same as PostgreSQL).

**Sticky session behavior**:
- Traefik sets a `_backend_affinity` cookie on the first response.
- All subsequent requests from the same client (including WebSocket upgrade) include this cookie.
- Traefik routes to the same backend replica.
- If the target replica goes down, Traefik automatically re-routes to a healthy replica
  (the cookie is invalidated).

**Files modified**:
- `docker-stack.yml` -- add Traefik service, Redis service, backend labels, scale replicas

**Tests needed**:
- Deploy with 2 replicas, verify sticky cookie is set and requests are routed consistently.
- Kill one replica, verify Traefik re-routes to the surviving replica within the health check
  interval.
- Verify WebSocket upgrade follows the sticky cookie.

## 5. Data Models and Storage

### Redis Key Schema

| Key Pattern | Type | TTL | Purpose |
|-------------|------|-----|---------|
| `rl:{bucket}:{key}` | String (integer) | 60s | Rate limit counter per bucket/key |
| `global:donated_cu` | String (integer) | 60s | Cached SUM(total_donated_cu) |
| `bitcoin:price_leader` | String (replica ID) | 15s | Leader election for price updates |
| `ws:broadcast` | Pub/Sub channel | N/A | WebSocket message fan-out |

**Estimated Redis memory usage at 10,000 active users**:
- Rate limit keys: ~10,000 keys * ~100 bytes = ~1 MB
- CU cache: 1 key = negligible
- Leader election: 1 key = negligible
- Pub/sub: no stored state (messages are transient)
- Total: well under 10 MB. The 128 MB limit provides ample headroom.

### PostgreSQL Changes

No schema changes. The only new PostgreSQL behavior is `SELECT ... FOR UPDATE` on the
existing `game_states` table, which uses the existing `user_id` unique index.

## 6. API Contracts

No API changes. All endpoints, request/response formats, and WebSocket message types remain
identical. The horizontal scaling is transparent to clients.

The only client-visible change is the `_backend_affinity` cookie set by Traefik, which the
browser handles automatically.

## 7. Migration and Rollout

### Migration Strategy: Incremental, Backward-Compatible

Each phase can be deployed independently. The system works correctly at every intermediate
state (single replica, single replica with Redis, multiple replicas). No phase requires
downtime.

### Phase 1: Add Redis to Infrastructure (S)

**Changes**: Add Redis service to `docker-stack.yml`. Add `go-redis/v9` dependency to
`go.mod`. Add `REDIS_ADDR` config. Initialize Redis client in `main.go` with a connectivity
check (log warning if unavailable, do not fail startup).

**Deployment**: `docker stack deploy` with updated stack file. Backend is still 1 replica.
Redis starts but is not used yet.

**Acceptance criteria**:
- `docker service ls` shows Redis running and healthy.
- Backend logs `Connected to Redis` on startup.
- All existing functionality unchanged.

**Rollback**: Remove Redis service from stack file, redeploy.

### Phase 2: Database Row Locking (M)

**Changes**: Implement `GetByUserIDForUpdate` and `LoadFullGameStateForUpdate`. Modify
`PerformAction`, `HandleWSAction`, and `runUserTick` to use transactions with `FOR UPDATE`.
Retain `userMutexMap` as local optimization.

**Deployment**: Rebuild backend image, rolling update (still 1 replica). Verify under load
with stress tester.

**Acceptance criteria**:
- All game actions work correctly.
- Stress test with 1,000 concurrent users shows no regressions.
- `pg_stat_activity` shows `FOR UPDATE` in active queries during load.
- No lock wait timeouts during normal operation.

**Rollback**: Revert transaction changes, redeploy. The `FOR UPDATE` is purely additive.

### Phase 3: Redis Rate Limiter (S)

**Changes**: Extract `RateLimitStore` interface. Implement `RedisRateLimitStore`. Wire in
`main.go` when Redis is available. Fall back to in-memory when Redis is absent.

**Deployment**: Rebuild, rolling update.

**Acceptance criteria**:
- Rate limiting works identically from the client perspective.
- `redis-cli KEYS "rl:*"` shows rate limit keys being created.
- Sending 11 requests in 1 minute to `/api/auth/login` returns 429 on the 11th.

**Rollback**: Set `REDIS_ADDR` to empty or remove `SetRateLimitStore` call. Falls back to
in-memory.

### Phase 4: Redis CU Cache (S)

**Changes**: Modify `GlobalDonatedCUCache` to use Redis. Add `rdb *redis.Client` parameter.

**Deployment**: Rebuild, rolling update.

**Acceptance criteria**:
- `redis-cli GET "global:donated_cu"` returns a number matching the database.
- `donate_cu` action immediately increments the Redis value.
- Other replicas (when scaled) see the updated value.

**Rollback**: Remove `rdb` parameter, revert to local-only cache.

### Phase 5: Redis Pub/Sub for WebSocket (M)

**Changes**: Implement `MessageBroadcaster` interface, `RedisBroadcaster`. Replace all
`h.hub.SendToUser*` calls in `GameHandler` with `h.broadcaster.SendToUser*`.

**Deployment**: Rebuild, rolling update (still 1 replica). Verify messages still delivered.

**Acceptance criteria**:
- WebSocket state pushes work identically.
- `redis-cli SUBSCRIBE "ws:broadcast"` shows messages being published.
- Events and state updates are received by the client.

**Rollback**: Revert broadcaster to direct Hub calls.

### Phase 6: Bitcoin Price Leader Election (S)

**Changes**: Implement `PriceLeader`. Modify `PriceService` to check leadership. Generate
replica IDs in `main.go`.

**Deployment**: Rebuild, rolling update.

**Acceptance criteria**:
- `redis-cli GET "bitcoin:price_leader"` shows exactly one replica ID.
- Bitcoin price updates continue at the expected rate.
- Killing the leader replica results in failover within 20 seconds.

**Rollback**: Remove leader check from `GetCurrentPrice` (all replicas advance, same as before).

### Phase 7: Traefik and Multi-Replica Deployment (M)

**Changes**: Add Traefik to `docker-stack.yml`. Add sticky session labels. Scale backend to
`replicas: 2`. Remove direct port mapping (`8080:8080`).

**Deployment**: `docker stack deploy` with updated stack file. This is the "big bang" moment
where multiple replicas go live.

**Acceptance criteria**:
- All 8 acceptance criteria from Section 1 are verified.
- Stress test with 1,000+ users across 2 replicas shows no errors.
- WebSocket connections survive rolling deploys.
- `docker service scale homelab_backend=3` works without issues.
- Traefik dashboard (if enabled) shows healthy backends.

**Rollback**: Scale backend to `replicas: 1`. All components continue to work in single-replica
mode (this is a key design property).

## 8. Risks and Open Questions

### Known Risks

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| Redis single point of failure | Medium | Low | All components degrade gracefully without Redis (rate limiting fails open, CU cache uses local value, WebSocket delivers locally only, price leader falls back to all-advance). Redis is also more reliable than the application itself -- it is a mature, battle-tested service. |
| `SELECT FOR UPDATE` increases transaction duration | Medium | Medium | Transactions are short (10s timeout). The in-memory `userMutexMap` prevents multiple goroutines on the same replica from competing for the DB lock, keeping contention low. Monitor `pg_stat_activity` for lock waits. |
| Sticky session cookie lost (client clears cookies) | Low | Low | System remains correct without stickiness -- just less optimal (local caches miss, messages fan out via Redis). The next request creates a new sticky assignment. |
| Redis pub/sub message loss during Redis restart | Low | Low | Pub/sub is inherently lossy. Lost messages mean a user misses one state push; the next tick delivers current state. The game is designed for this (5-second tick interval means worst case is 5s stale display). |
| Transaction deadlocks | Low | Very Low | We lock exactly one row per transaction (the user's game_states row), always first. No cross-user transactions exist. PostgreSQL deadlock detection (1s default) provides a safety net. |
| Dual bitcoin price leaders during failover | Low | Low | The price model is deterministic from its stored seed. If two replicas briefly advance the price simultaneously, they compute identical steps (same seed from DB). The `SELECT ... FOR UPDATE` in `GetCurrentPrice` prevents actual seed corruption. |

### Open Questions

1. **Should we add Redis Sentinel or clustering?** Not at this scale. A single Redis instance
   handles the workload trivially (sub-millisecond operations, <10 MB memory). Sentinel adds
   operational complexity without meaningful benefit until Redis availability becomes a real
   concern. If Redis goes down, the system degrades gracefully.

2. **Should the `userMutexMap` be removed entirely?** It should be retained as a local
   optimization to reduce database lock contention. Two goroutines on the same replica
   competing for the same DB row lock waste a database connection. The in-memory lock
   prevents this. It can be removed in the future if profiling shows it is unnecessary.

3. **Should the tick goroutine run on all replicas or only the one with the WebSocket?**
   The tick goroutine is spawned by `OnConnect`, which fires when a WebSocket connection
   is established. With sticky sessions, the WebSocket connection is on a specific replica,
   so the tick goroutine runs there. If the sticky session fails and the user reconnects to
   a different replica, `OnConnect` fires on the new replica and starts a new tick goroutine.
   This is correct behavior -- no change needed.

4. **What is the maximum number of replicas?** PostgreSQL connection pool (50 connections per
   replica) is the limiting factor. At 5 replicas: 250 connections total against a
   `max_connections` of 150. Solution: reduce per-replica pool size (e.g., 25 per replica for
   5 replicas) or increase PostgreSQL `max_connections`. This should be addressed before
   scaling beyond 3 replicas. A configuration mechanism (`DB_MAX_CONNS` env var) should be
   added.

### Assumptions

- Docker Swarm's overlay network provides reliable inter-service communication (backend to
  Redis, backend to PostgreSQL).
- Traefik correctly handles WebSocket upgrade with sticky cookies (well-documented and tested
  feature of Traefik).
- Redis latency is <1ms for all operations (Redis is on the same overlay network as the
  backend, same physical host).
- The existing 5-second tick interval is acceptable as the maximum stale data window for
  cross-replica consistency.

## 9. Testing Strategy

### Per-Phase Unit Tests

Each new component (`RedisRateLimitStore`, `RedisBroadcaster`, `PriceLeader`,
`LoadFullGameStateForUpdate`) must have unit tests that cover the happy path, error paths,
and fallback behavior.

### Integration Tests

- **Multi-replica simulation**: Start two `GameHandler` instances in the same test process
  sharing a PostgreSQL and Redis connection. Verify that concurrent actions for the same user
  are serialized.
- **WebSocket cross-replica delivery**: Publish a message from one `RedisBroadcaster` instance,
  verify it is received by a Hub connected to a different `RedisBroadcaster` instance.
- **Rate limit distribution**: Send requests through two different `RedisRateLimitStore`
  instances, verify the combined count is enforced.

### Stress Testing

The existing stress tester (`stress-tests/main.go`) must be run after each phase:

```bash
cd /root/project/stress-tests
go run . -players 1000 -duration 60s -rampup 10s -ws
go run . -players 2000 -duration 60s -rampup 10s -ws
```

After Phase 7 (multi-replica), run the stress tester against the Traefik endpoint to verify
that requests are distributed across replicas and no errors occur.

### Manual Verification Checklist

- [ ] Register a new user on multi-replica deployment
- [ ] Log in, verify WebSocket connects and state pushes arrive
- [ ] Perform game actions (buy hardware, deploy service, run job)
- [ ] Verify bitcoin price updates on the price chart
- [ ] Perform a `donate_cu` action, verify the global CU updates
- [ ] Kill one replica (`docker service scale backend=1`), verify game continues
- [ ] Scale back up (`docker service scale backend=2`), verify no disruption
- [ ] Perform a rolling update (`docker service update --image ...`), verify zero downtime

## 10. Observability and Operational Readiness

### Key Health Signals

| Signal | Source | Alert Threshold |
|--------|--------|-----------------|
| Redis connectivity | `PING` every 10s | 3 consecutive failures |
| Redis memory usage | `INFO memory` | >80% of maxmemory (>102 MB) |
| DB lock wait time | `pg_stat_activity` wait_event = 'Lock' | >1s average |
| Transaction duration | Application logging | >5s for any transaction |
| Leader election state | `GET bitcoin:price_leader` | No leader for >30s |
| WebSocket pub/sub lag | Application logging | Messages queued >100 |
| Rate limit fallback | Application logging | Any `failing open` log entry |

### Logging Additions

- `[redis] connected to {addr}` at startup
- `[redis] connection lost: {err}` on disconnect
- `[ratelimit] Redis error, failing open: {err}` on Redis failure
- `[cu-cache] Redis GET failed, using local: {err}` on Redis failure
- `[ws-pubsub] published to {channel}: {userID}` at debug level
- `[ws-pubsub] received from {channel}: {userID}` at debug level
- `[bitcoin-leader] acquired leadership: {replicaID}` on election
- `[bitcoin-leader] lost leadership: {replicaID}` on loss
- `[tx] lock acquired for user {userID} in {duration}` when lock wait >100ms

### 3am Diagnosability

If the game is broken at 3am after a multi-replica deploy:

1. **Check replica count**: `docker service ls` -- are all replicas running?
2. **Check Redis**: `docker exec redis redis-cli PING` -- is Redis alive?
3. **Check leader**: `docker exec redis redis-cli GET bitcoin:price_leader` -- is there a
   bitcoin price leader?
4. **Check logs**: `docker service logs backend --tail 100` -- any `[redis]` error logs?
5. **Check DB locks**: `SELECT * FROM pg_stat_activity WHERE wait_event = 'Lock'` -- are
   transactions waiting?
6. **Nuclear option**: Scale to 1 replica (`docker service scale backend=1`). This immediately
   reverts to single-replica behavior with all in-memory state working as before. Diagnose
   the multi-replica issue at a reasonable hour.

## 11. Implementation Phases Summary

| Phase | Component | Files Modified | Size | Dependencies | Acceptance |
|-------|-----------|---------------|------|-------------|------------|
| 1 | Redis infrastructure | `docker-stack.yml`, `go.mod`, `config/config.go`, `cmd/server/main.go` | S | None | Redis running, backend connects |
| 2 | DB row locking | `queries/game_state.go`, `queries/batch.go`, `handlers/game.go` | M | Phase 1 (Redis up for later phases) | Concurrent actions serialized, stress test passes |
| 3 | Redis rate limiter | `middleware/ratelimit.go`, `middleware/redis_ratelimit.go` (new), `cmd/server/main.go` | S | Phase 1 | Rate limits enforced via Redis keys |
| 4 | Redis CU cache | `handlers/cu_cache.go`, `cmd/server/main.go` | S | Phase 1 | Redis key matches DB value |
| 5 | Redis pub/sub WS | `ws/pubsub.go` (new), `ws/redis_broadcaster.go` (new), `ws/hub.go`, `handlers/game.go`, `cmd/server/main.go` | M | Phase 1 | Messages delivered cross-replica |
| 6 | Bitcoin leader | `bitcoin/leader.go` (new), `bitcoin/price.go`, `cmd/server/main.go` | S | Phase 1 | Exactly 1 leader, failover works |
| 7 | Traefik + scale out | `docker-stack.yml` | M | Phases 1-6 | All 8 acceptance criteria pass |

**Total estimated effort**: L (large) -- 4 small phases + 3 medium phases, spanning 10+
files with integration testing between each phase.

**Critical path**: Phases 1-6 can be developed in parallel after Phase 1 (they all depend
only on Redis being available). Phase 7 depends on all prior phases being deployed and
verified. The recommended implementation order prioritizes correctness (Phase 2: DB locking)
over convenience (Phases 3-4) because database locking is the highest-risk change and should
be verified under load before introducing additional complexity.
