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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Config struct {
	BaseURL    string
	Players    int
	Duration   time.Duration
	RampUp     time.Duration
	ActionRate time.Duration // time between actions per player
	EnableWS   bool
	WSOnly     bool // when true + EnableWS, send ALL actions over WS (no HTTP)
	Verbose    bool
	ResultsDir string // directory to write results markdown file
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
	wsActionOK      atomic.Int64
	wsActionErr     atomic.Int64
	wsActionLatUs   atomic.Int64
	rateLimited     atomic.Int64

	mu           sync.Mutex
	latencies    []int64 // microseconds (HTTP)
	wsLatencies  []int64 // microseconds (WS actions)
}

func (s *Stats) recordLatency(us int64) {
	s.totalLatencyUs.Add(us)
	s.mu.Lock()
	s.latencies = append(s.latencies, us)
	s.mu.Unlock()
}

func (s *Stats) recordWSLatency(us int64) {
	s.wsActionLatUs.Add(us)
	s.mu.Lock()
	s.wsLatencies = append(s.wsLatencies, us)
	s.mu.Unlock()
}

func percentileOf(data []int64, p float64) float64 {
	if len(data) == 0 {
		return 0
	}
	sorted := make([]int64, len(data))
	copy(sorted, data)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	return float64(sorted[idx]) / 1000.0 // return ms
}

func (s *Stats) percentile(p float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return percentileOf(s.latencies, p)
}

func (s *Stats) wsPercentile(p float64) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return percentileOf(s.wsLatencies, p)
}

type player struct {
	email string
	token string
}

// weightedAction pairs an action type with its payload generator and selection weight.
type weightedAction struct {
	actionType string
	weight     int
	payload    func() any
}

var actions = []weightedAction{
	// run_job is the dominant action (most common player action)
	{"run_job", 10, func() any { return nil }},
	{"buy_hardware", 3, func() any { return map[string]string{"hardware_id": "raspberry_pi_4"} }},
	{"deploy_service", 2, func() any { return map[string]string{"service_id": "pihole"} }},
	{"buy_upgrade", 2, func() any { return map[string]string{"upgrade_id": "desk_fan"} }},
	{"activate_overclock", 1, func() any { return map[string]int{"tier": 1} }},
	{"buy_research", 1, func() any { return map[string]string{"node_id": "read_the_docs"} }},
	{"optimize_rack", 1, func() any { return nil }},
	{"buy_max_bitcoin", 1, func() any { return nil }},
	{"sell_all_bitcoin", 1, func() any { return nil }},
}

// totalWeight is the sum of all action weights, computed at init time.
var totalWeight int

func init() {
	for _, a := range actions {
		totalWeight += a.weight
	}
}

// pickAction selects a random action weighted by the weight field.
func pickAction() weightedAction {
	r := rand.Intn(totalWeight)
	for _, a := range actions {
		r -= a.weight
		if r < 0 {
			return a
		}
	}
	return actions[0] // fallback
}

// wsConn wraps a websocket connection with a map for correlating action request/response by ID.
type wsConn struct {
	conn    *websocket.Conn
	mu      sync.Mutex
	pending map[string]chan wsActionResult
}

type wsActionResult struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Success bool            `json:"success"`
	State   json.RawMessage `json:"state,omitempty"`
	Error   string          `json:"error,omitempty"`
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{
		conn:    conn,
		pending: make(map[string]chan wsActionResult),
	}
}

// readLoop reads messages from the WebSocket, dispatching action_result to pending callers
// and discarding other message types (state pushes, events, etc.).
func (w *wsConn) readLoop() {
	for {
		_, message, err := w.conn.ReadMessage()
		if err != nil {
			// Connection closed — wake up all pending callers.
			w.mu.Lock()
			for id, ch := range w.pending {
				close(ch)
				delete(w.pending, id)
			}
			w.mu.Unlock()
			return
		}

		var msg struct {
			Type string `json:"type"`
			ID   string `json:"id"`
		}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		if msg.Type == "action_result" {
			var result wsActionResult
			if err := json.Unmarshal(message, &result); err != nil {
				continue
			}
			w.mu.Lock()
			ch, ok := w.pending[result.ID]
			if ok {
				delete(w.pending, result.ID)
			}
			w.mu.Unlock()
			if ok {
				ch <- result
			}
		}
		// Ignore state pushes, events, etc. — just keep the connection alive.
	}
}

// sendAction sends a game action over WebSocket and waits for the correlated response.
// Returns (success, latencyUs, error).
func (w *wsConn) sendAction(actionType string, payload any) (bool, int64, error) {
	b := make([]byte, 16)
	crand.Read(b)
	id := fmt.Sprintf("%x", b)

	msg := map[string]any{
		"type":   "action",
		"id":     id,
		"action": actionType,
	}
	if payload != nil {
		msg["payload"] = payload
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return false, 0, fmt.Errorf("marshal: %w", err)
	}

	ch := make(chan wsActionResult, 1)
	w.mu.Lock()
	w.pending[id] = ch
	w.mu.Unlock()

	start := time.Now()
	if err := w.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		w.mu.Lock()
		delete(w.pending, id)
		w.mu.Unlock()
		return false, 0, fmt.Errorf("write: %w", err)
	}

	select {
	case result, ok := <-ch:
		latency := time.Since(start).Microseconds()
		if !ok {
			return false, latency, fmt.Errorf("connection closed")
		}
		return result.Success, latency, nil
	case <-time.After(10 * time.Second):
		w.mu.Lock()
		delete(w.pending, id)
		w.mu.Unlock()
		latency := time.Since(start).Microseconds()
		return false, latency, fmt.Errorf("timeout")
	}
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.BaseURL, "url", "http://localhost:8080", "Backend base URL")
	flag.IntVar(&cfg.Players, "players", 100, "Number of simulated players")
	flag.DurationVar(&cfg.Duration, "duration", 60*time.Second, "Test duration")
	flag.DurationVar(&cfg.RampUp, "rampup", 5*time.Second, "Time to ramp up all players")
	flag.DurationVar(&cfg.ActionRate, "rate", 500*time.Millisecond, "Time between actions per player")
	flag.BoolVar(&cfg.EnableWS, "ws", false, "Open WebSocket connections and send actions over WS")
	flag.BoolVar(&cfg.WSOnly, "ws-only", false, "When combined with -ws, send ALL actions over WS (no HTTP)")
	flag.BoolVar(&cfg.Verbose, "verbose", false, "Print individual request results")
	flag.StringVar(&cfg.ResultsDir, "results-dir", "/root/project/docs/stress-tests", "Directory to write results markdown file")
	flag.Parse()

	// -ws-only requires -ws
	if cfg.WSOnly && !cfg.EnableWS {
		fmt.Fprintln(os.Stderr, "-ws-only requires -ws to be enabled")
		os.Exit(1)
	}

	fmt.Println("=== Homelab Game Backend Stress Test ===")
	fmt.Printf("Target:     %s\n", cfg.BaseURL)
	fmt.Printf("Players:    %d\n", cfg.Players)
	fmt.Printf("Duration:   %s\n", cfg.Duration)
	fmt.Printf("Ramp-up:    %s\n", cfg.RampUp)
	fmt.Printf("Action rate: %s per player\n", cfg.ActionRate)
	fmt.Printf("WebSocket:  %v\n", cfg.EnableWS)
	if cfg.EnableWS {
		fmt.Printf("WS-only:    %v\n", cfg.WSOnly)
	}
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
	// playerWS maps player index -> *wsConn for WS action sending
	playerWS := make(map[int]*wsConn)
	if cfg.EnableWS {
		fmt.Println("\nOpening WebSocket connections...")
		playerWS = connectWebSockets(cfg, players, stats)
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
	preWSActionOK := stats.wsActionOK.Load()
	preWSActionErr := stats.wsActionErr.Load()
	preWSLatency := stats.wsActionLatUs.Load()

	// Clear latency samples for main phase
	stats.mu.Lock()
	stats.latencies = stats.latencies[:0]
	stats.wsLatencies = stats.wsLatencies[:0]
	stats.mu.Unlock()

	stopCh := make(chan struct{})
	var testWg sync.WaitGroup

	rampDelay := cfg.RampUp / time.Duration(cfg.Players)

	// Ticker for periodic stats
	ticker := time.NewTicker(5 * time.Second)
	lastReport := time.Now()
	lastActions := int64(0)
	lastWSActions := int64(0)

	go func() {
		for {
			select {
			case <-ticker.C:
				now := time.Now()
				elapsed := now.Sub(lastReport).Seconds()
				currentActions := stats.actionOK.Load() - preActions
				currentErrors := stats.actionErr.Load() - preErrors
				rps := float64(currentActions-lastActions) / elapsed

				line := fmt.Sprintf("[%s] http: %d ok / %d err | %.0f/sec | p50: %.1fms | p99: %.1fms | rate-limited: %d",
					now.Format("15:04:05"),
					currentActions, currentErrors, rps,
					stats.percentile(50), stats.percentile(99),
					stats.rateLimited.Load())

				if cfg.EnableWS {
					currentWSActions := stats.wsActionOK.Load() - preWSActionOK
					currentWSErrors := stats.wsActionErr.Load() - preWSActionErr
					wsRPS := float64(currentWSActions-lastWSActions) / elapsed
					line += fmt.Sprintf(" | ws: %d ok / %d err | %.0f/sec | p50: %.1fms | p99: %.1fms",
						currentWSActions, currentWSErrors, wsRPS,
						stats.wsPercentile(50), stats.wsPercentile(99))
					lastWSActions = currentWSActions
				}

				fmt.Println(line)

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
		ws := playerWS[i] // nil if WS not enabled or connection failed
		go func(p player, ws *wsConn) {
			defer testWg.Done()
			runPlayerLoop(client, cfg, p, ws, stats, stopCh)
		}(players[i], ws)
		time.Sleep(rampDelay)
	}

	// Wait for duration
	time.Sleep(cfg.Duration)
	close(stopCh)
	testWg.Wait()
	ticker.Stop()
	testElapsed := time.Since(testStart)

	// Close WebSocket connections
	for _, ws := range playerWS {
		ws.conn.Close()
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
	fmt.Println("--- HTTP Requests ---")
	fmt.Printf("Total requests:     %d\n", totalReqs)
	fmt.Printf("Successful actions: %d\n", totalActions)
	fmt.Printf("Failed actions:     %d\n", totalErrors)
	fmt.Printf("Rate limited:       %d\n", stats.rateLimited.Load())
	fmt.Printf("Error rate:         %.2f%%\n", errorRate(totalErrors, totalReqs))
	fmt.Println()
	fmt.Println("--- HTTP Throughput ---")
	fmt.Printf("Requests/sec:       %.1f\n", float64(totalReqs)/testElapsed.Seconds())
	fmt.Printf("Actions/sec:        %.1f\n", float64(totalActions)/testElapsed.Seconds())
	fmt.Println()
	fmt.Println("--- HTTP Latency ---")
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
		totalWSOK := stats.wsActionOK.Load() - preWSActionOK
		totalWSErr := stats.wsActionErr.Load() - preWSActionErr
		totalWSLat := stats.wsActionLatUs.Load() - preWSLatency
		totalWS := totalWSOK + totalWSErr

		fmt.Println("--- WebSocket Actions ---")
		fmt.Printf("Connected:          %d\n", stats.wsConnected.Load())
		fmt.Printf("Connection errors:  %d\n", stats.wsErrors.Load())
		fmt.Printf("Successful actions: %d\n", totalWSOK)
		fmt.Printf("Failed actions:     %d\n", totalWSErr)
		fmt.Printf("Actions/sec:        %.1f\n", float64(totalWSOK)/testElapsed.Seconds())
		if totalWS > 0 {
			fmt.Printf("Error rate:         %.2f%%\n", errorRate(totalWSErr, totalWS))
		}
		fmt.Println()
		fmt.Println("--- WebSocket Latency ---")
		if totalWS > 0 {
			fmt.Printf("Average:            %.1f ms\n", float64(totalWSLat)/float64(totalWS)/1000.0)
		}
		fmt.Printf("P50:                %.1f ms\n", stats.wsPercentile(50))
		fmt.Printf("P90:                %.1f ms\n", stats.wsPercentile(90))
		fmt.Printf("P95:                %.1f ms\n", stats.wsPercentile(95))
		fmt.Printf("P99:                %.1f ms\n", stats.wsPercentile(99))
		fmt.Printf("Max:                %.1f ms\n", stats.wsPercentile(100))
		fmt.Println()
	}

	fmt.Println("--- Registration Phase ---")
	fmt.Printf("Registered:         %d\n", stats.registerOK.Load())
	fmt.Printf("Failed:             %d\n", stats.registerErr.Load())
	fmt.Println("========================================")

	// Write results to markdown file
	if cfg.ResultsDir != "" {
		writeResultsFile(cfg, stats, testElapsed, registered, totalActions, totalErrors, totalReqs, totalLat,
			preWSActionOK, preWSActionErr, preWSLatency)
	}
}

func writeResultsFile(cfg Config, stats *Stats, testElapsed time.Duration, registered int64,
	totalActions, totalErrors, totalReqs, totalLat int64,
	preWSActionOK, preWSActionErr, preWSLatency int64) {

	os.MkdirAll(cfg.ResultsDir, 0755)

	now := time.Now()
	durationLabel := strings.TrimSuffix(cfg.Duration.String(), "0s")
	if durationLabel == "" {
		durationLabel = cfg.Duration.String()
	}
	mode := "http"
	if cfg.WSOnly {
		mode = "ws-only"
	} else if cfg.EnableWS {
		mode = "ws"
	}
	filename := fmt.Sprintf("%s-%d_players_%s_%s.md",
		now.Format("2006-01-02_15-04-05"),
		cfg.Players,
		durationLabel,
		mode,
	)
	filepath := filepath.Join(cfg.ResultsDir, filename)

	var b strings.Builder
	b.WriteString("# Stress Test Results\n\n")
	b.WriteString(fmt.Sprintf("**Date:** %s\n\n", now.Format("2006-01-02 15:04:05")))
	b.WriteString("## Parameters\n\n")
	b.WriteString(fmt.Sprintf("| Parameter | Value |\n|---|---|\n"))
	b.WriteString(fmt.Sprintf("| Target | `%s` |\n", cfg.BaseURL))
	b.WriteString(fmt.Sprintf("| Players | %d |\n", cfg.Players))
	b.WriteString(fmt.Sprintf("| Duration | %s |\n", cfg.Duration))
	b.WriteString(fmt.Sprintf("| Ramp-up | %s |\n", cfg.RampUp))
	b.WriteString(fmt.Sprintf("| Action rate | %s/player |\n", cfg.ActionRate))
	b.WriteString(fmt.Sprintf("| WebSocket | %v |\n", cfg.EnableWS))
	if cfg.EnableWS {
		b.WriteString(fmt.Sprintf("| WS-only | %v |\n", cfg.WSOnly))
	}
	b.WriteString(fmt.Sprintf("| Registered | %d/%d |\n", registered, cfg.Players))
	b.WriteString("\n")

	b.WriteString("## HTTP Results\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Value |\n|---|---|\n"))
	b.WriteString(fmt.Sprintf("| Total requests | %d |\n", totalReqs))
	b.WriteString(fmt.Sprintf("| Successful actions | %d |\n", totalActions))
	b.WriteString(fmt.Sprintf("| Failed actions | %d |\n", totalErrors))
	b.WriteString(fmt.Sprintf("| Rate limited | %d |\n", stats.rateLimited.Load()))
	b.WriteString(fmt.Sprintf("| Error rate | %.2f%% |\n", errorRate(totalErrors, totalReqs)))
	b.WriteString(fmt.Sprintf("| Requests/sec | %.1f |\n", float64(totalReqs)/testElapsed.Seconds()))
	b.WriteString(fmt.Sprintf("| Actions/sec | %.1f |\n", float64(totalActions)/testElapsed.Seconds()))
	b.WriteString("\n")

	b.WriteString("## HTTP Latency\n\n")
	b.WriteString(fmt.Sprintf("| Percentile | Latency |\n|---|---|\n"))
	if totalReqs > 0 {
		b.WriteString(fmt.Sprintf("| Average | %.1f ms |\n", float64(totalLat)/float64(totalReqs)/1000.0))
	}
	b.WriteString(fmt.Sprintf("| P50 | %.1f ms |\n", stats.percentile(50)))
	b.WriteString(fmt.Sprintf("| P90 | %.1f ms |\n", stats.percentile(90)))
	b.WriteString(fmt.Sprintf("| P95 | %.1f ms |\n", stats.percentile(95)))
	b.WriteString(fmt.Sprintf("| P99 | %.1f ms |\n", stats.percentile(99)))
	b.WriteString(fmt.Sprintf("| Max | %.1f ms |\n", stats.percentile(100)))
	b.WriteString("\n")

	if cfg.EnableWS {
		totalWSOK := stats.wsActionOK.Load() - preWSActionOK
		totalWSErr := stats.wsActionErr.Load() - preWSActionErr
		totalWSLat := stats.wsActionLatUs.Load() - preWSLatency
		totalWS := totalWSOK + totalWSErr

		b.WriteString("## WebSocket Results\n\n")
		b.WriteString(fmt.Sprintf("| Metric | Value |\n|---|---|\n"))
		b.WriteString(fmt.Sprintf("| Connected | %d |\n", stats.wsConnected.Load()))
		b.WriteString(fmt.Sprintf("| Connection errors | %d |\n", stats.wsErrors.Load()))
		b.WriteString(fmt.Sprintf("| Successful actions | %d |\n", totalWSOK))
		b.WriteString(fmt.Sprintf("| Failed actions | %d |\n", totalWSErr))
		b.WriteString(fmt.Sprintf("| Actions/sec | %.1f |\n", float64(totalWSOK)/testElapsed.Seconds()))
		if totalWS > 0 {
			b.WriteString(fmt.Sprintf("| Error rate | %.2f%% |\n", errorRate(totalWSErr, totalWS)))
		}
		b.WriteString("\n")

		b.WriteString("## WebSocket Latency\n\n")
		b.WriteString(fmt.Sprintf("| Percentile | Latency |\n|---|---|\n"))
		if totalWS > 0 {
			b.WriteString(fmt.Sprintf("| Average | %.1f ms |\n", float64(totalWSLat)/float64(totalWS)/1000.0))
		}
		b.WriteString(fmt.Sprintf("| P50 | %.1f ms |\n", stats.wsPercentile(50)))
		b.WriteString(fmt.Sprintf("| P90 | %.1f ms |\n", stats.wsPercentile(90)))
		b.WriteString(fmt.Sprintf("| P95 | %.1f ms |\n", stats.wsPercentile(95)))
		b.WriteString(fmt.Sprintf("| P99 | %.1f ms |\n", stats.wsPercentile(99)))
		b.WriteString(fmt.Sprintf("| Max | %.1f ms |\n", stats.wsPercentile(100)))
		b.WriteString("\n")
	}

	if err := os.WriteFile(filepath, []byte(b.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write results file: %v\n", err)
	} else {
		fmt.Printf("\nResults written to: %s\n", filepath)
	}
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
	act := pickAction()

	payload := act.payload()

	body, _ := json.Marshal(map[string]any{
		"type":    act.actionType,
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

func performWSAction(ws *wsConn, stats *Stats) {
	act := pickAction()
	payload := act.payload()

	success, latencyUs, err := ws.sendAction(act.actionType, payload)
	stats.recordWSLatency(latencyUs)

	if err != nil {
		stats.wsActionErr.Add(1)
		return
	}

	if success {
		stats.wsActionOK.Add(1)
	} else {
		// Game-logic errors (not enough money, wrong tier, etc.) are still "processed OK"
		stats.wsActionOK.Add(1)
	}
}

func runPlayerLoop(client *http.Client, cfg Config, p player, ws *wsConn, stats *Stats, stop chan struct{}) {
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
			if cfg.WSOnly && ws != nil {
				// WS-only mode: all actions go over WebSocket
				performWSAction(ws, stats)
			} else if cfg.EnableWS && ws != nil {
				// Mixed mode: WS for actions, HTTP for state fetches
				if rand.Float64() < 0.2 {
					getState(client, cfg.BaseURL, p.token, stats, cfg.Verbose)
				} else {
					performWSAction(ws, stats)
				}
			} else {
				// HTTP-only mode (original behavior)
				if rand.Float64() < 0.2 {
					getState(client, cfg.BaseURL, p.token, stats, cfg.Verbose)
				} else {
					performAction(client, cfg.BaseURL, p.token, stats)
				}
			}
		}
	}
}

func connectWebSockets(cfg Config, players []player, stats *Stats) map[int]*wsConn {
	result := make(map[int]*wsConn)
	var mu sync.Mutex

	wsURL := strings.Replace(cfg.BaseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)

	sem := make(chan struct{}, 50)
	var wg sync.WaitGroup

	for i, p := range players {
		if p.token == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, token string) {
			defer wg.Done()
			defer func() { <-sem }()

			u, _ := url.Parse(wsURL + "/ws")
			q := u.Query()
			q.Set("token", token)
			u.RawQuery = q.Encode()

			dialer := websocket.Dialer{
				HandshakeTimeout: 10 * time.Second,
			}
			headers := http.Header{}
			headers.Set("Origin", "http://localhost:3000")
			conn, _, err := dialer.Dial(u.String(), headers)
			if err != nil {
				stats.wsErrors.Add(1)
				return
			}
			stats.wsConnected.Add(1)

			wsc := newWSConn(conn)
			go wsc.readLoop()

			mu.Lock()
			result[idx] = wsc
			mu.Unlock()
		}(i, p.token)
	}
	wg.Wait()
	return result
}

func errorRate(errors, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total) * 100
}
