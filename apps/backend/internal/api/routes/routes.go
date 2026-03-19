package routes

import (
	"net/http"

	"github.com/homelab-game/backend/internal/api/handlers"
	"github.com/homelab-game/backend/internal/api/middleware"
	"github.com/homelab-game/backend/internal/api/ws"
)

func Setup(authHandler *handlers.AuthHandler, gameHandler *handlers.GameHandler, socialHandler *handlers.SocialHandler, hub *ws.Hub, jwtSecret string) http.Handler {
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Auth routes (public, rate limited)
	authRateLimit := middleware.RateLimit(10) // 10 attempts per minute per IP
	mux.Handle("POST /api/auth/register", authRateLimit(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /api/auth/login", authRateLimit(http.HandlerFunc(authHandler.Login)))

	// WebSocket (auth via query param)
	mux.HandleFunc("GET /ws", hub.HandleConnect(jwtSecret))

	// Game routes (authenticated, rate limited per user)
	authMw := middleware.Auth(jwtSecret)
	gameRateLimit := middleware.RateLimitByUser(300) // 300 actions per minute per user (5/sec)
	mux.Handle("GET /api/game/state", authMw(http.HandlerFunc(gameHandler.GetState)))
	mux.Handle("POST /api/game/action", authMw(gameRateLimit(http.HandlerFunc(gameHandler.PerformAction))))

	// Social routes (authenticated, rate limited)
	socialRateLimit := middleware.RateLimitByUser(30) // 30 per minute per user
	mux.Handle("GET /api/social/group", authMw(http.HandlerFunc(socialHandler.GetMyGroup)))
	mux.Handle("GET /api/social/groups", authMw(http.HandlerFunc(socialHandler.ListGroups)))
	mux.Handle("POST /api/social/group/create", authMw(socialRateLimit(http.HandlerFunc(socialHandler.CreateGroup))))
	mux.Handle("POST /api/social/group/join", authMw(socialRateLimit(http.HandlerFunc(socialHandler.JoinGroup))))
	mux.Handle("POST /api/social/group/leave", authMw(socialRateLimit(http.HandlerFunc(socialHandler.LeaveGroup))))
	mux.Handle("POST /api/social/group/promote", authMw(socialRateLimit(http.HandlerFunc(socialHandler.PromoteMember))))
	mux.Handle("POST /api/social/group/kick", authMw(socialRateLimit(http.HandlerFunc(socialHandler.KickMember))))
	mux.Handle("GET /api/social/leaderboard", authMw(http.HandlerFunc(socialHandler.GetLeaderboard)))
	mux.Handle("POST /api/social/leaderboard/update", authMw(http.HandlerFunc(socialHandler.UpdateLeaderboards)))

	// Apply global middleware
	var handler http.Handler = mux
	handler = middleware.MaxBodySize(handler)
	handler = middleware.JSON(handler)
	handler = middleware.CORS(handler)

	return handler
}
