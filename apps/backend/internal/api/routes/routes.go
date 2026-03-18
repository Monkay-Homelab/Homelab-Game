package routes

import (
	"net/http"

	"github.com/homelab-game/backend/internal/api/handlers"
	"github.com/homelab-game/backend/internal/api/middleware"
)

func Setup(authHandler *handlers.AuthHandler, gameHandler *handlers.GameHandler, jwtSecret string) http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Auth routes (public)
	mux.HandleFunc("POST /api/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)

	// Game routes (authenticated)
	authMw := middleware.Auth(jwtSecret)
	mux.Handle("GET /api/game/state", authMw(http.HandlerFunc(gameHandler.GetState)))
	mux.Handle("POST /api/game/action", authMw(http.HandlerFunc(gameHandler.PerformAction)))

	// Apply global middleware
	var handler http.Handler = mux
	handler = middleware.JSON(handler)
	handler = middleware.CORS(handler)

	return handler
}
