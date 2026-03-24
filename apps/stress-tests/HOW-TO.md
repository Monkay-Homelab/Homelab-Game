Phases:

1. Register — Creates N unique players concurrently (50 at a time)
2. Warm-up — Fetches initial game state for all players
3. WebSocket — Optionally opens persistent WS connections for all players
4. Main test — Each player loops performing actions at the configured rate (80% game actions, 20%
   state fetches) with staggered ramp-up

Metrics reported:

- Requests/sec and actions/sec throughput
- Latency percentiles (P50, P90, P95, P99, max)
- Error rate and rate-limited count
- Live stats every 5 seconds during the test

Usage examples:
cd /root/project/stress-tests
go build -o stresstest .

# 100 players, 60s, action every 500ms (default)

./stresstest -url http://localhost:8080

# Heavy load: 500 players, 2 min, faster actions

./stresstest -players 500 -duration 2m -rate 200ms

# With WebSocket connections

./stresstest -players 200 -ws

# Quick smoke test

./stresstest -players 1000 -duration 15s -verbose
