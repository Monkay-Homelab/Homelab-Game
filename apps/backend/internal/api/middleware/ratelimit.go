package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimitStore abstracts the storage backend for rate limiting.
// Implementations must be safe for concurrent use.
type RateLimitStore interface {
	CheckRate(ctx context.Context, key string, maxPerMinute int) bool
}

// store is the active rate limit backend. Defaults to in-memory.
// Call SetRateLimitStore during initialization (before serving traffic) to swap.
var store RateLimitStore = NewInMemoryRateLimitStore()

// SetRateLimitStore replaces the active rate limit backend.
// Must be called during initialization, before serving traffic.
func SetRateLimitStore(s RateLimitStore) {
	store = s
}

// InMemoryRateLimitStore implements RateLimitStore with a local map.
type InMemoryRateLimitStore struct {
	mu       sync.Mutex
	visitors map[string]*visitor
}

type visitor struct {
	count    int
	lastSeen time.Time
}

func NewInMemoryRateLimitStore() *InMemoryRateLimitStore {
	s := &InMemoryRateLimitStore{visitors: make(map[string]*visitor)}
	go s.cleanup()
	return s
}

func (s *InMemoryRateLimitStore) cleanup() {
	for {
		time.Sleep(time.Minute)
		s.mu.Lock()
		for key, v := range s.visitors {
			if time.Since(v.lastSeen) > time.Minute {
				delete(s.visitors, key)
			}
		}
		s.mu.Unlock()
	}
}

func (s *InMemoryRateLimitStore) CheckRate(_ context.Context, key string, maxPerMinute int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, exists := s.visitors[key]
	if !exists || time.Since(v.lastSeen) > time.Minute {
		s.visitors[key] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}
	v.count++
	v.lastSeen = time.Now()
	return v.count <= maxPerMinute
}

// getClientIP extracts the real client IP. Prefers X-Real-IP (set by trusted
// reverse proxy) over X-Forwarded-For. For XFF, uses the rightmost IP which
// is the one appended by the trusted proxy and cannot be spoofed by the client.
func getClientIP(r *http.Request) string {
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return r.RemoteAddr
}

func checkRate(key string, maxPerMinute int) bool {
	return store.CheckRate(context.Background(), key, maxPerMinute)
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
