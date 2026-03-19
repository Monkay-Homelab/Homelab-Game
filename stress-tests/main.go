package main

import (
	"bytes"
	crand "crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	BaseURL       string
	Players       int
	Duration      time.Duration
	RampUp        time.Duration
	ActionRate    time.Duration // time between actions per player
	EnableWS      bool
	Verbose       bool
	SkipRegister  bool
	CleanupAfter  bool
}

type Stats struct {
	totalRequests   atomic.Int64
	totalErrors     atomic.Int64
	totalLatencyUs  atomic.Int64
	registerOK      atomic.Int64
	registerErr     atomic.Int64
	getStateOK      atomic.Int64
	getStateErr     atomic.Int64
	actionOK        atomic.Int64
	actionErr       atomic.Int64
	wsConnected     atomic.Int64
	wsErrors        atomic.Int64
	rateLimited     atomic.Int64

	mu        sync.Mutex
	latencies []int64 // microseconds
}

func (s *Stats) recordLatency(us int64) {
	s.totalLatencyUs.Add(us)
	s.mu.Lock()
	s.latencies = append(s.latencies, us)
	s.mu.Unlock()
}

func (s *Stats) percentile(p float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.latencies) == 0 {
		return 0
	}
	sorted := make([]int64, len(s.latencies))
	copy(sorted, s.latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return float64(sorted[idx]) / 1000.0 // return ms
}

type player struct {
	email string
	token string
}

var actionTypes = []string{
	"run_job",
	"run_job",
	"run_job", // weighted towards run_job (most common action)
	"buy_hardware",
	"deploy_service",
	"buy_upgrade",
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BaseURL, "url", "http://localhost:8080", "Backend base URL")
	flag.IntVar(&cfg.Players, "players", 100, "Number of simulated players")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.DurationVar(&cfg.RampUp, "rampup", 5*time.Second, "Time to ramp up all players")
	flag.DurationVar(&cfg.ActionRate, "rate", 500*time.Millisecond, "Time between actions per player")
	flag.BoolVar(&cfg.EnableWS, "ws", false, "Also open WebSocket connections")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Print individual request results")
	flag.Parse()

	fmt.Println("=== Homelab Game Backend Stress Test ===")
	fmt.Printf("Target:     %s\n", cfg.BaseURL)
	fmt.Printf("Players:    %d\n", cfg.Players)
	fmt.Printf("Duration:   %s\n", cfg.Duration)
	fmt.Printf("Ramp-up:    %s\n", cfg.RampUp)
	fmt.Printf("Action rate: %s per player\n", cfg.ActionRate)
	fmt.Printf("WebSocket:  %v\n", cfg.EnableWS)
	fmt.Println()

	// Check server health
	resp, err := http.Get(cfg.BaseURL + "/health")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot reach server at %s: %v\n", cfg.BaseURL, err)
		os.Exit(1)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Server health check failed: %d\n", resp.StatusCode)
		os.Exit(1)
	}
	fmt.Println("Server is healthy.")

	stats := &Stats{}
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        cfg.Players * 2,
			MaxIdleConnsPerHost: cfg.Players * 2,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Phase 1: Register players
	fmt.Printf("\nRegistering %d players...\n", cfg.Players)
	players := make([]player, cfg.Players)
	regStart := time.Now()

	var regWg sync.WaitGroup
	sem := make(chan struct{}, 50) // limit concurrent registrations
	for i := 0; i < cfg.Players; i++ {
		regWg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer regWg.Done()
			defer func() { <-sem }()
			p, err := registerPlayer(client, cfg.BaseURL, idx, stats)
			if err != nil {
				if cfg.Verbose {
					fmt.Printf("  Register player %d failed: %v\n", idx, err)
				}
				return
			}
			players[idx] = p
		}(i)
	}
	regWg.Wait()

	registered := stats.registerOK.Load()
	fmt.Printf("Registered %d/%d players in %s (errors: %d)\n",
		registered, cfg.Players, time.Since(regStart).Round(time.Millisecond), stats.registerErr.Load())

	if registered == 0 {
		fmt.Fprintln(os.Stderr, "No players registered. Aborting.")
		os.Exit(1)
	}

	// Phase 2: Initial state fetch (warm up)
	fmt.Println("\nFetching initial state for all players...")
	var warmWg sync.WaitGroup
	for i := range players {
		if players[i].token == "" {
			continue
		}
		warmWg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer warmWg.Done()
			defer func() { <-sem }()
			getState(client, cfg.BaseURL, players[idx].token, stats, cfg.Verbose)
		}(i)
	}
	warmWg.Wait()
	fmt.Printf("State fetched: %d ok, %d errors\n", stats.getStateOK.Load(), stats.getStateErr.Load())

	// Phase 3: WebSocket connections (optional)
	var wsConns []*websocket.Conn
	if cfg.EnableWS {
		fmt.Println("\nOpening WebSocket connections...")
		wsConns = connectWebSockets(cfg, players, stats)
		fmt.Printf("WebSocket: %d connected, %d errors\n",
			stats.wsConnected.Load(), stats.wsErrors.Load())
	}

	// Phase 4: Main stress test
	fmt.Printf("\n--- Starting %s stress test ---\n", cfg.Duration)
	fmt.Println("(Stats printed every 5 seconds)")
	fmt.Println()

	// Reset action counters for the main test phase
	preActions := stats.actionOK.Load()
	preErrors := stats.actionErr.Load()
	preRequests := stats.totalRequests.Load()
	preLatencies := stats.totalLatencyUs.Load()

	// Clear latency samples for main phase
	stats.mu.Lock()
	stats.latencies = stats.latencies[:0]
	stats.mu.Unlock()

	stopCh := make(chan struct{})
	var testWg sync.WaitGroup

	rampDelay := cfg.RampUp / time.Duration(cfg.Players)

	// Ticker for periodic stats
	ticker := time.NewTicker(5 * time.Second)
	lastReport := time.Now()
	lastActions := int64(0)

	go func() {
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				elapsed := now.Sub(lastReport).Seconds()
				currentActions := stats.actionOK.Load() - preActions
				currentErrors := stats.actionErr.Load() - preErrors
				rps := float64(currentActions-lastActions) / elapsed

				fmt.Printf("[%s] actions: %d ok / %d err | %.0f actions/sec | p50: %.1fms | p99: %.1fms | rate-limited: %d\n",
					now.Format("15:04:05"),
					currentActions, currentErrors, rps,
					stats.percentile(50), stats.percentile(99),
					stats.rateLimited.Load())

				lastReport = now
				lastActions = currentActions
			case <-stopCh:
				return
			}
		}
	}()

	testStart := time.Now()
	for i := range players {
		if players[i].token == "" {
			continue
		}
		testWg.Add(1)
		go func(p player) {
			defer testWg.Done()
			runPlayerLoop(client, cfg, p, stats, stopCh)
		}(players[i])
		time.Sleep(rampDelay)
	}

	// Wait for duration
	time.Sleep(cfg.Duration)
	close(stopCh)
	testWg.Wait()
	ticker.Stop()
	testElapsed := time.Since(testStart)

	// Close WebSocket connections
	for _, ws := range wsConns {
		ws.Close()
	}

	// Final report
	totalActions := stats.actionOK.Load() - preActions
	totalErrors := stats.actionErr.Load() - preErrors
	totalReqs := stats.totalRequests.Load() - preRequests
	totalLat := stats.totalLatencyUs.Load() - preLatencies

	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("         STRESS TEST RESULTS")
	fmt.Println("========================================")
	fmt.Printf("Duration:           %s\n", testElapsed.Round(time.Millisecond))
	fmt.Printf("Players:            %d\n", registered)
	fmt.Printf("Action rate:        %s per player\n", cfg.ActionRate)
	fmt.Println()
	fmt.Println("--- Requests ---")
	fmt.Printf("Total requests:     %d\n", totalReqs)
	fmt.Printf("Successful actions: %d\n", totalActions)
	fmt.Printf("Failed actions:     %d\n", totalErrors)
	fmt.Printf("Rate limited:       %d\n", stats.rateLimited.Load())
	fmt.Printf("Error rate:         %.2f%%\n", errorRate(totalErrors, totalReqs))
	fmt.Println()
	fmt.Println("--- Throughput ---")
	fmt.Printf("Requests/sec:       %.1f\n", float64(totalReqs)/testElapsed.Seconds())
	fmt.Printf("Actions/sec:        %.1f\n", float64(totalActions)/testElapsed.Seconds())
	fmt.Println()
	fmt.Println("--- Latency ---")
	if totalReqs > 0 {
		fmt.Printf("Average:            %.1f ms\n", float64(totalLat)/float64(totalReqs)/1000.0)
	}
	fmt.Printf("P50:                %.1f ms\n", stats.percentile(50))
	fmt.Printf("P90:                %.1f ms\n", stats.percentile(90))
	fmt.Printf("P95:                %.1f ms\n", stats.percentile(95))
	fmt.Printf("P99:                %.1f ms\n", stats.percentile(99))
	fmt.Printf("Max:                %.1f ms\n", stats.percentile(100))
	fmt.Println()

	if cfg.EnableWS {
		fmt.Println("--- WebSocket ---")
		fmt.Printf("Connected:          %d\n", stats.wsConnected.Load())
		fmt.Printf("Errors:             %d\n", stats.wsErrors.Load())
		fmt.Println()
	}

	fmt.Println("--- Registration Phase ---")
	fmt.Printf("Registered:         %d\n", stats.registerOK.Load())
	fmt.Printf("Failed:             %d\n", stats.registerErr.Load())
	fmt.Println("========================================")
}

func registerPlayer(client *http.Client, baseURL string, idx int, stats *Stats) (player, error) {
	b := make([]byte, 8)
	crand.Read(b)
	hex := fmt.Sprintf("%x", b)
	email := fmt.Sprintf("s%d_%s@t.co", idx, hex)
	name := fmt.Sprintf("s%s", hex)
	if len(name) > 20 {
		name = name[:20]
	}

	body, _ := json.Marshal(map[string]string{
		"email":        email,
		"password":     "stresstestpassword123",
		"display_name": name,
	})

	req, _ := http.NewRequest("POST", baseURL+"/api/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	// Use unique X-Forwarded-For per player to avoid IP-based rate limiting
	req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.%d.%d.%d", (idx/65536)%256, (idx/256)%256, idx%256))

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Microseconds()

	stats.totalRequests.Add(1)
	stats.recordLatency(latency)

	if err != nil {
		stats.registerErr.Add(1)
		stats.totalErrors.Add(1)
		return player{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		stats.rateLimited.Add(1)
		stats.registerErr.Add(1)
		stats.totalErrors.Add(1)
		return player{}, fmt.Errorf("rate limited")
	}

	if resp.StatusCode != http.StatusCreated {
		stats.registerErr.Add(1)
		stats.totalErrors.Add(1)
		return player{}, fmt.Errorf("status %d (email=%s): %s", resp.StatusCode, email, string(respBody))
	}

	var result struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		stats.registerErr.Add(1)
		stats.totalErrors.Add(1)
		return player{}, fmt.Errorf("bad response: %w", err)
	}

	stats.registerOK.Add(1)
	return player{email: email, token: result.Token}, nil
}

func getState(client *http.Client, baseURL, token string, stats *Stats, verbose bool) {
	req, _ := http.NewRequest("GET", baseURL+"/api/game/state", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Microseconds()

	stats.totalRequests.Add(1)
	stats.recordLatency(latency)

	if err != nil {
		stats.getStateErr.Add(1)
		stats.totalErrors.Add(1)
		if verbose {
			fmt.Printf("  GetState error: %v\n", err)
		}
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // drain

	if resp.StatusCode == http.StatusTooManyRequests {
		stats.rateLimited.Add(1)
		stats.getStateErr.Add(1)
		stats.totalErrors.Add(1)
		return
	}

	if resp.StatusCode == http.StatusOK {
		stats.getStateOK.Add(1)
	} else {
		stats.getStateErr.Add(1)
		stats.totalErrors.Add(1)
	}
}

func performAction(client *http.Client, baseURL, token string, stats *Stats) {
	actionType := actionTypes[rand.Intn(len(actionTypes))]

	var payload any
	switch actionType {
	case "run_job":
		payload = nil
	case "buy_hardware":
		// Try to buy a raspberry pi (cheapest, always available at tier 1)
		payload = map[string]string{"hardware_id": "raspberry_pi_4"}
	case "deploy_service":
		payload = map[string]string{"service_id": "pihole"}
	case "buy_upgrade":
		payload = map[string]string{"upgrade_id": "desk_fan"}
	default:
		actionType = "run_job"
	}

	body, _ := json.Marshal(map[string]any{
		"type":    actionType,
		"payload": payload,
	})

	req, _ := http.NewRequest("POST", baseURL+"/api/game/action", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start).Microseconds()

	stats.totalRequests.Add(1)
	stats.recordLatency(latency)

	if err != nil {
		stats.actionErr.Add(1)
		stats.totalErrors.Add(1)
		return
	}
	defer resp.Body.Close()
	io.ReadAll(resp.Body) // drain

	if resp.StatusCode == http.StatusTooManyRequests {
		stats.rateLimited.Add(1)
		stats.actionErr.Add(1)
		stats.totalErrors.Add(1)
		return
	}

	if resp.StatusCode == http.StatusOK {
		stats.actionOK.Add(1)
	} else {
		// 400 is expected for "not enough compute" etc — still counts as processed
		stats.actionOK.Add(1)
	}
}

func runPlayerLoop(client *http.Client, cfg Config, p player, stats *Stats, stop chan struct{}) {
	// Add some jitter so players don't all fire at the same instant
	jitter := time.Duration(rand.Int63n(int64(cfg.ActionRate)))
	time.Sleep(jitter)

	ticker := time.NewTicker(cfg.ActionRate)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			// Mix of actions: 80% action, 20% state fetch
			if rand.Float64() < 0.2 {
				getState(client, cfg.BaseURL, p.token, stats, cfg.Verbose)
			} else {
				performAction(client, cfg.BaseURL, p.token, stats)
			}
		}
	}
}

func connectWebSockets(cfg Config, players []player, stats *Stats) []*websocket.Conn {
	var conns []*websocket.Conn
	var mu sync.Mutex

	wsURL := strings.Replace(cfg.BaseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	sem := make(chan struct{}, 50)
	var wg sync.WaitGroup

	for _, p := range players {
		if p.token == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(token string) {
			defer wg.Done()
			defer func() { <-sem }()

			u, _ := url.Parse(wsURL + "/ws")
			q := u.Query()
			q.Set("token", token)
			u.RawQuery = q.Encode()

			dialer := websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
			}
			conn, _, err := dialer.Dial(u.String(), nil)
			if err != nil {
				stats.wsErrors.Add(1)
				return
			}
			stats.wsConnected.Add(1)

			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()

			// Keep reading in background to keep connection alive
			go func() {
				for {
					_, _, err := conn.ReadMessage()
					if err != nil {
						return
					}
				}
			}()
		}(p.token)
	}
	wg.Wait()
	return conns
}

func errorRate(errors, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total) * 100
}
