package middleware

import "net/http"

// MaxBodySize limits request body to 64KB.
func MaxBodySize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024) // 64KB
		next.ServeHTTP(w, r)
	})
}
