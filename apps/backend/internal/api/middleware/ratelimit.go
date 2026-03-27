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

// getClientIP extracts the real client IP. Prefers X-Real-IP (set by trusted
// reverse proxy) over X-Forwarded-For. For XFF, uses the rightmost IP which
// is the one appended by the trusted proxy and cannot be spoofed by the client.
func getClientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Rightmost IP is the one appended by the trusted reverse proxy
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
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
	return RateLimitNamed("auth", maxPerMinute)
}

// RateLimitNamed limits requests per client IP with a named bucket.
func RateLimitNamed(name string, maxPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			if !checkRate(name+":ip:"+ip, maxPerMinute) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CheckGameActionRate checks whether a game action from the given user should
// be allowed under the rate limit. It uses the same bucket name pattern and
// rate as the HTTP game action middleware (7200 req/min per user). Returns
// true if the request is within the limit, false if it should be rejected.
func CheckGameActionRate(userID string) bool {
	return checkRate("game:user:"+userID, 7200)
}

// RateLimitByUser limits requests per authenticated user ID with a named bucket.
// Falls back to IP if user ID not in context.
func RateLimitByUser(name string, maxPerMinute int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := name + ":ip:" + getClientIP(r)
			if uid, ok := r.Context().Value(UserIDKey).(string); ok && uid != "" {
				key = name + ":user:" + uid
			}
			if !checkRate(key, maxPerMinute) {
				http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
