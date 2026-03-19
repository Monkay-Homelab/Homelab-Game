package middleware

import (
	"net/http"
	"os"
	"strings"
)

var allowedOrigins = func() map[string]bool {
	origins := map[string]bool{
		"https://game.homelab.living": true,
		"http://game.homelab.living":  true,
		"https://homelab.living":      true,
		"http://homelab.living":       true,
	}
	// Allow extra origins from env (comma-separated)
	if extra := os.Getenv("CORS_ORIGINS"); extra != "" {
		for _, o := range strings.Split(extra, ",") {
			origins[strings.TrimSpace(o)] = true
		}
	}
	// Dev mode: allow localhost
	if os.Getenv("ENV") != "production" {
		origins["http://localhost:3000"] = true
		origins["http://127.0.0.1:3000"] = true
		origins["http://192.168.3.107:3000"] = true
	}
	return origins
}()

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
