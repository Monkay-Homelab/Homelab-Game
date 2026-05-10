package handlers

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const redisCUKey = "global:donated_cu"

// GlobalDonatedCUCache caches the SUM(total_donated_cu) from game_states,
// refreshing periodically instead of querying on every request. At 2K
// connected users with 5-second ticks, this eliminates ~400 full table
// scans per second.
//
// When Redis is available, Get reads from Redis first (shared across replicas)
// and Add uses atomic INCRBY for cross-replica consistency. All Redis
// operations are nil-safe: if rdb is nil or a Redis call fails, the cache
// falls back to its local in-memory value.
type GlobalDonatedCUCache struct {
	mu    sync.RWMutex
	value int64
	pool  *pgxpool.Pool
	rdb   *redis.Client
}

// NewGlobalDonatedCUCache creates a new cache with a blocking initial load
// and starts a background goroutine that refreshes at refreshInterval.
// The initial query must succeed or the server starts with value 0 (logged).
// rdb may be nil for no-Redis mode (local-only caching).
func NewGlobalDonatedCUCache(pool *pgxpool.Pool, rdb *redis.Client, refreshInterval time.Duration) *GlobalDonatedCUCache {
	c := &GlobalDonatedCUCache{pool: pool, rdb: rdb}

	// Blocking initial load before the server accepts connections.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c.refresh(ctx)

	// Background refresh goroutine — runs for the lifetime of the process.
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

// Get returns the cached global donated CU value. Safe for concurrent use.
// When Redis is available, it reads from Redis first for cross-replica
// consistency. On Redis error or nil client, falls back to the local value.
func (c *GlobalDonatedCUCache) Get() int64 {
	if c.rdb != nil {
		val, err := c.rdb.Get(context.Background(), redisCUKey).Int64()
		if err == nil {
			return val
		}
		slog.Warn("cu-cache redis get error, falling back to local", "error", err)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// refresh queries the database for the current SUM and updates the cached
// value. On error, the stale value is retained and the error is logged.
// After a successful PostgreSQL query, the value is also written to Redis
// (if available) with a 60-second TTL.
func (c *GlobalDonatedCUCache) refresh(ctx context.Context) {
	start := time.Now()
	var val int64
	err := c.pool.QueryRow(ctx, "SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states").Scan(&val)
	if err != nil {
		slog.Error("cu-cache refresh error", "error", err)
		return // keep stale value
	}
	elapsed := time.Since(start)
	if elapsed > time.Second {
		slog.Warn("cu-cache slow refresh", "duration", elapsed)
	}
	c.mu.Lock()
	c.value = val
	c.mu.Unlock()

	if c.rdb != nil {
		if err := c.rdb.Set(ctx, redisCUKey, val, 60*time.Second).Err(); err != nil {
			slog.Warn("cu-cache redis set error", "error", err)
		}
	}
}

// Add atomically adds the given amount to the cached value. Called after a
// successful donate_cu action so the next tick/response reflects the donation
// without waiting for the periodic refresh.
//
// When Redis is available, uses INCRBY for atomic cross-replica increment
// and also updates the local value. On Redis error, falls back to local-only.
func (c *GlobalDonatedCUCache) Add(amount int64) {
	if c.rdb != nil {
		if err := c.rdb.IncrBy(context.Background(), redisCUKey, amount).Err(); err != nil {
			slog.Warn("cu-cache redis incrby error, local-only update", "error", err)
		}
	}
	c.mu.Lock()
	c.value += amount
	c.mu.Unlock()
}
