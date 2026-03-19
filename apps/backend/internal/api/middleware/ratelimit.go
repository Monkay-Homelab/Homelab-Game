package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
}

type visitor struct {
	count    int
	lastSeen time.Time
}

var limiter = &rateLimiter{
	visitors: make(map[string]*visitor),
}

func init() {
	go func() {
		for {
			time.Sleep(time.Minute)
			limiter.mu.Lock()
			for key, v := range limiter.visitors {
				if time.Since(v.lastSeen) > time.Minute {
					delete(limiter.visitors, key)
				}
			}
			limiter.mu.Unlock()
		}
	}()
}

// getClientIP extracts the real client IP, checking X-Forwarded-For first.
func getClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First IP in the chain is the original client
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	return r.RemoteAddr
}

func checkRate(key string, maxPerMinute int) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	v, exists := limiter.visitors[key]
	if !exists || time.Since(v.lastSeen) > time.Minute {
		limiter.visitors[key] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}
	v.count++
	v.lastSeen = time.Now()
	return v.count <= maxPerMinute
}

// RateLimit limits requests per client IP.
func RateLimit(maxPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			if !checkRate("ip:"+ip, maxPerMinute) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitByUser limits requests per authenticated user ID.
// Falls back to IP if user ID not in context.
func RateLimitByUser(maxPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "ip:" + getClientIP(r)
			if uid, ok := r.Context().Value(UserIDKey).(string); ok && uid != "" {
				key = "user:" + uid
			}
			if !checkRate(key, maxPerMinute) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
