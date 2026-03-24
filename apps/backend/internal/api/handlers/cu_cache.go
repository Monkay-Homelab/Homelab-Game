package handlers

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GlobalDonatedCUCache caches the SUM(total_donated_cu) from game_states,
// refreshing periodically instead of querying on every request. At 2K
// connected users with 5-second ticks, this eliminates ~400 full table
// scans per second.
type GlobalDonatedCUCache struct {
	mu    sync.RWMutex
	value int64
	pool  *pgxpool.Pool
}

// NewGlobalDonatedCUCache creates a new cache with a blocking initial load
// and starts a background goroutine that refreshes at refreshInterval.
// The initial query must succeed or the server starts with value 0 (logged).
func NewGlobalDonatedCUCache(pool *pgxpool.Pool, refreshInterval time.Duration) *GlobalDonatedCUCache {
	c := &GlobalDonatedCUCache{pool: pool}

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
func (c *GlobalDonatedCUCache) Get() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.value
}

// refresh queries the database for the current SUM and updates the cached
// value. On error, the stale value is retained and the error is logged.
func (c *GlobalDonatedCUCache) refresh(ctx context.Context) {
	start := time.Now()
	var val int64
	err := c.pool.QueryRow(ctx, "SELECT COALESCE(SUM(total_donated_cu), 0) FROM game_states").Scan(&val)
	if err != nil {
		log.Printf("[cu-cache] refresh error: %v", err)
		return // keep stale value
	}
	elapsed := time.Since(start)
	if elapsed > time.Second {
		log.Printf("[cu-cache] slow refresh: %v", elapsed)
	}
	c.mu.Lock()
	c.value = val
	c.mu.Unlock()
}

// Add atomically adds the given amount to the cached value. Called after a
// successful donate_cu action so the next tick/response reflects the donation
// without waiting for the periodic refresh.
func (c *GlobalDonatedCUCache) Add(amount int64) {
	c.mu.Lock()
	c.value += amount
	c.mu.Unlock()
}
