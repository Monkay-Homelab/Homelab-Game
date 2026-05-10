package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/homelab-game/backend/internal/api/handlers"
	"github.com/homelab-game/backend/internal/api/middleware"
	"github.com/homelab-game/backend/internal/api/routes"
	"github.com/homelab-game/backend/internal/api/ws"
	"github.com/homelab-game/backend/internal/config"
	"github.com/homelab-game/backend/internal/database"
	"github.com/homelab-game/backend/internal/database/queries"
	"github.com/homelab-game/backend/internal/game/bitcoin"
	"github.com/homelab-game/backend/internal/game/engine"
	"github.com/homelab-game/backend/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	loadEnvFile()
	logging.Init()

	cfg := config.Load()

	pool, err := database.Connect(cfg.DatabaseURL())
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer pool.Close()
	slog.Info("connected to database")

	// Connect to Redis (optional — graceful degradation if unavailable)
	var rdb *redis.Client
	rdb = redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		slog.Warn("redis unavailable, running without redis", "addr", cfg.RedisAddr, "error", err)
		rdb = nil
	} else {
		slog.Info("connected to redis", "addr", cfg.RedisAddr)
		defer func() { _ = rdb.Close() }()
	}
	// Wire Redis-backed services when available
	if rdb != nil {
		middleware.SetRateLimitStore(middleware.NewRedisRateLimitStore(rdb))
		slog.Info("rate limiting backed by redis")
	}

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

	// Bitcoin price leader election (only one replica advances the price)
	if rdb != nil {
		hostname, _ := os.Hostname()
		replicaID := fmt.Sprintf("backend-%s-%d", hostname, time.Now().UnixNano())
		priceLeader := bitcoin.NewPriceLeader(rdb, replicaID)
		priceLeader.Start(context.Background())
		defer priceLeader.Stop()
		bitcoinService.SetLeader(priceLeader)
		slog.Info("bitcoin price leader election started", "replica_id", replicaID)
	}

	// Create message broadcaster (Redis-backed if available, local otherwise)
	var broadcaster ws.MessageBroadcaster
	if rdb != nil {
		rb := ws.NewRedisBroadcaster(wsHub, rdb)
		_ = rb.Start(context.Background())
		defer rb.Stop()
		broadcaster = rb
		slog.Info("websocket broadcasting backed by redis pub/sub")
	} else {
		broadcaster = ws.NewLocalBroadcaster(wsHub)
	}

	// Cache the global donated CU sum to eliminate per-request full table scans.
	// Blocking initial load ensures the cache is populated before accepting connections.
	globalCUCache := handlers.NewGlobalDonatedCUCache(pool, rdb, 30*time.Second)
	slog.Info("global donated CU cache initialized")

	authHandler := handlers.NewAuthHandler(userQueries, gameStateQueries, cfg.JWTSecret, cfg.RegistrationEnabled)
	gameHandler := handlers.NewGameHandler(pool, gameStateQueries, hardwareQueries, serviceQueries, upgradeQueries, componentQueries, customerQueries, expenseQueries, coloRackQueries, groupQueries, researchLevelQueries, gameEngine, wsHub, broadcaster, bitcoinService, globalCUCache)
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
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown: listen for SIGTERM/SIGINT, drain connections.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-quit
		slog.Info("shutting down server", "grace_period", "10s")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	slog.Info("homelab game API starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server listen error: %w", err)
	}
	slog.Info("server stopped")
	return nil
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
		_ = os.Setenv(strings.TrimSpace(key), strings.TrimSpace(val))
	}
}
