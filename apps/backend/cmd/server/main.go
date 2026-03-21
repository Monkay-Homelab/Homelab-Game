package main

import (
	"context"
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
	groupQueries := queries.NewGroupQueries(pool)
	leaderboardQueries := queries.NewLeaderboardQueries(pool)
	gameEngine := engine.New()
	wsHub := ws.NewHub()

	bitcoinQueries := queries.NewBitcoinQueries(pool)
	bitcoinStore := &bitcoinStoreAdapter{q: bitcoinQueries}
	bitcoinService := bitcoin.NewPriceService(bitcoinStore, bitcoin.DefaultPriceConfig())

	authHandler := handlers.NewAuthHandler(userQueries, gameStateQueries, cfg.JWTSecret)
	gameHandler := handlers.NewGameHandler(gameStateQueries, hardwareQueries, serviceQueries, upgradeQueries, componentQueries, customerQueries, expenseQueries, coloRackQueries, groupQueries, gameEngine, wsHub, bitcoinService)
	socialHandler := handlers.NewSocialHandler(groupQueries, leaderboardQueries, gameStateQueries)

	handler := routes.Setup(authHandler, gameHandler, socialHandler, wsHub, cfg.JWTSecret)

	addr := ":" + cfg.Port
	log.Printf("Homelab Game API starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatal(err)
	}
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
