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

	// Auth routes (public)
	mux.HandleFunc("POST /api/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)

	// WebSocket (auth via query param)
	mux.HandleFunc("GET /ws", hub.HandleConnect(jwtSecret))

	// Game routes (authenticated)
	authMw := middleware.Auth(jwtSecret)
	mux.Handle("GET /api/game/state", authMw(http.HandlerFunc(gameHandler.GetState)))
	mux.Handle("POST /api/game/action", authMw(http.HandlerFunc(gameHandler.PerformAction)))

	// Social routes (authenticated)
	mux.Handle("GET /api/social/group", authMw(http.HandlerFunc(socialHandler.GetMyGroup)))
	mux.Handle("GET /api/social/groups", authMw(http.HandlerFunc(socialHandler.ListGroups)))
	mux.Handle("POST /api/social/group/create", authMw(http.HandlerFunc(socialHandler.CreateGroup)))
	mux.Handle("POST /api/social/group/join", authMw(http.HandlerFunc(socialHandler.JoinGroup)))
	mux.Handle("POST /api/social/group/leave", authMw(http.HandlerFunc(socialHandler.LeaveGroup)))
	mux.Handle("POST /api/social/group/promote", authMw(http.HandlerFunc(socialHandler.PromoteMember)))
	mux.Handle("POST /api/social/group/kick", authMw(http.HandlerFunc(socialHandler.KickMember)))
	mux.Handle("GET /api/social/leaderboard", authMw(http.HandlerFunc(socialHandler.GetLeaderboard)))
	mux.Handle("POST /api/social/leaderboard/update", authMw(http.HandlerFunc(socialHandler.UpdateLeaderboards)))

	// Apply global middleware
	var handler http.Handler = mux
	handler = middleware.JSON(handler)
	handler = middleware.CORS(handler)

	return handler
}
