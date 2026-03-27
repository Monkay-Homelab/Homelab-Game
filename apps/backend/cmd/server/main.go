package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/homelab-game/backend/internal/api/handlers"
	"github.com/homelab-game/backend/internal/api/routes"
	"github.com/homelab-game/backend/internal/api/ws"
	"github.com/homelab-game/backend/internal/config"
	"github.com/homelab-game/backend/internal/database"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/game/bitcoin"
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
	researchLevelQueries := queries.NewResearchLevelQueries(pool)
	groupQueries := queries.NewGroupQueries(pool)
	leaderboardQueries := queries.NewLeaderboardQueries(pool)
	gameEngine := engine.New()
	wsHub := ws.NewHub()

	bitcoinQueries := queries.NewBitcoinQueries(pool)
	bitcoinStore := &bitcoinStoreAdapter{q: bitcoinQueries}
	bitcoinService := bitcoin.NewPriceService(bitcoinStore, bitcoin.DefaultPriceConfig())

	// Cache the global donated CU sum to eliminate per-request full table scans.
	// Blocking initial load ensures the cache is populated before accepting connections.
	globalCUCache := handlers.NewGlobalDonatedCUCache(pool, 30*time.Second)
	log.Println("Global donated CU cache initialized")

	authHandler := handlers.NewAuthHandler(userQueries, gameStateQueries, cfg.JWTSecret)
	gameHandler := handlers.NewGameHandler(pool, gameStateQueries, hardwareQueries, serviceQueries, upgradeQueries, componentQueries, customerQueries, expenseQueries, coloRackQueries, groupQueries, researchLevelQueries, gameEngine, wsHub, bitcoinService, globalCUCache)
	socialHandler := handlers.NewSocialHandler(groupQueries, leaderboardQueries, gameStateQueries)

	// Wire hub lifecycle callbacks to GameHandler so that WebSocket
	// connect/disconnect events start and stop per-user tick goroutines.
	// This must happen before routes.Setup, which registers the /ws
	// endpoint that begins accepting connections.
	wsHub.OnConnect = gameHandler.OnConnect
	wsHub.OnDisconnect = gameHandler.OnDisconnect
	wsHub.OnMessage = func(userID string, data []byte) {
		go gameHandler.HandleWSAction(userID, data)
	}

	handler := routes.Setup(authHandler, gameHandler, socialHandler, wsHub, cfg.JWTSecret)

	addr := ":" + cfg.Port
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	// Graceful shutdown: listen for SIGTERM/SIGINT, drain connections.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-quit
		log.Println("Shutting down server (10s grace period)...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("Homelab Game API starting on %s", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Println("Server stopped")
}

// bitcoinStoreAdapter adapts queries.BitcoinQueries to the bitcoin.PriceStore interface.
// BitcoinQueries was built with a different method signature than PriceStore expects,
// so this adapter bridges the two without modifying either package.
type bitcoinStoreAdapter struct {
	q *queries.BitcoinQueries
}

func (a *bitcoinStoreAdapter) GetPrice(ctx context.Context) (*bitcoin.PriceState, error) {
	bp, err := a.q.GetPrice(ctx)
	if err != nil {
		return nil, err
	}
	return &bitcoin.PriceState{
		CurrentPrice: bp.CurrentPrice,
		Seed:         bp.Seed,
		LastStepAt:   bp.LastStepAt,
		UpdatedAt:    bp.UpdatedAt,
	}, nil
}

func (a *bitcoinStoreAdapter) UpdatePrice(ctx context.Context, state *bitcoin.PriceState) error {
	return a.q.UpdatePrice(ctx, state.CurrentPrice, state.Seed, state.LastStepAt)
}

func (a *bitcoinStoreAdapter) InsertPriceHistory(ctx context.Context, point bitcoin.PricePoint) error {
	return a.q.InsertPriceHistory(ctx, point.Time, point.Price)
}

func (a *bitcoinStoreAdapter) GetPriceHistory(ctx context.Context, limit int) ([]bitcoin.PricePoint, error) {
	history, err := a.q.GetPriceHistory(ctx, limit)
	if err != nil {
		return nil, err
	}
	points := make([]bitcoin.PricePoint, len(history))
	for i, h := range history {
		points[i] = bitcoin.PricePoint{Time: h.Time, Price: h.Price}
	}
	return points, nil
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
