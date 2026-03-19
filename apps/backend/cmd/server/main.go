package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/homelab-game/backend/internal/api/handlers"
	"github.com/homelab-game/backend/internal/api/routes"
	"github.com/homelab-game/backend/internal/api/ws"
	"github.com/homelab-game/backend/internal/config"
	"github.com/homelab-game/backend/internal/database"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/game/engine"
)

func main() {
	loadEnvFile()

	cfg := config.Load()

	pool, err := database.Connect(cfg.DatabaseURL())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()
	log.Println("Connected to database")

	userQueries := queries.NewUserQueries(pool)
	gameStateQueries := queries.NewGameStateQueries(pool)
	hardwareQueries := queries.NewHardwareQueries(pool)
	serviceQueries := queries.NewServiceQueries(pool)
	upgradeQueries := queries.NewUpgradeQueries(pool)
	componentQueries := queries.NewComponentUpgradeQueries(pool)
	customerQueries := queries.NewCustomerQueries(pool)
	expenseQueries := queries.NewExpenseQueries(pool)
	coloRackQueries := queries.NewColoRackQueries(pool)
	groupQueries := queries.NewGroupQueries(pool)
	leaderboardQueries := queries.NewLeaderboardQueries(pool)
	gameEngine := engine.New()
	wsHub := ws.NewHub()

	authHandler := handlers.NewAuthHandler(userQueries, gameStateQueries, cfg.JWTSecret)
	gameHandler := handlers.NewGameHandler(gameStateQueries, hardwareQueries, serviceQueries, upgradeQueries, componentQueries, customerQueries, expenseQueries, coloRackQueries, groupQueries, gameEngine, wsHub)
	socialHandler := handlers.NewSocialHandler(groupQueries, leaderboardQueries, gameStateQueries)

	handler := routes.Setup(authHandler, gameHandler, socialHandler, wsHub, cfg.JWTSecret)

	addr := ":" + cfg.Port
	log.Printf("Homelab Game API starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
}

// loadEnvFile reads a .env file from the working directory if present.
func loadEnvFile() {
	data, err := os.ReadFile(filepath.Join(".", ".env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		os.Setenv(strings.TrimSpace(key), strings.TrimSpace(val))
	}
}
