package middleware

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimitStore implements RateLimitStore using Redis INCR + EXPIRE.
type RedisRateLimitStore struct {
	rdb *redis.Client
}

func NewRedisRateLimitStore(rdb *redis.Client) *RedisRateLimitStore {
	return &RedisRateLimitStore{rdb: rdb}
}

// CheckRate uses a Lua script for atomic increment-and-check.
// On Redis error, fails open (allows the request).
func (s *RedisRateLimitStore) CheckRate(ctx context.Context, key string, maxPerMinute int) bool {
	script := redis.NewScript(`
		local count = redis.call('INCR', KEYS[1])
		if count == 1 then
			redis.call('EXPIRE', KEYS[1], 60)
		end
		return count
	`)

	result, err := script.Run(ctx, s.rdb, []string{"rl:" + key}).Int()
	if err != nil {
		slog.Warn("ratelimit redis error, failing open", "key", key, "error", err)
		return true
	}
	return result <= maxPerMinute
}
