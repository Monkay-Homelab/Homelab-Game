# Stress Test Results

Test environment: single machine, Go backend + PostgreSQL, localhost connections.
Tool: `/stress-tests/` (custom Go load tester)

Each player performs actions every 200ms (80% game actions, 20% state fetches).
Tests run for 60 seconds after ramp-up.

## 2026-03-19 Results

### Summary Table

| Players | Actions/sec | Req/sec | P50   | P90   | P95    | P99    | Max    | Errors |
| ------- | ----------- | ------- | ----- | ----- | ------ | ------ | ------ | ------ |
| 100     | 160         | 191     | 2.9ms | 8.1ms | 10.1ms | 14.4ms | 35.9ms | 0%     |
| 500     | 1,918       | 2,392   | 3.1ms | 4.4ms | 4.7ms  | 5.5ms  | 16.3ms | 0%     |
| 1,000   | 3,691       | 4,615   | 3.9ms | 6.2ms | 7.6ms  | 13.1ms | 46.1ms | 0%     |
| 2,500   | 4,478       | 5,592   | 431ms | 457ms | 461ms  | 472ms  | 491ms  | 0%     |
| 5,000   | 4,472       | 5,589   | 843ms | 918ms | 929ms  | 942ms  | 970ms  | 0%     |

### Key Findings

- **Throughput ceiling:** ~4,500 actions/sec (~5,600 req/sec total). The server handles 1,000 concurrent players with sub-15ms P99 latency. Beyond that, requests start queueing.
- **Latency wall at 5,000 players:** P50 jumps from 3.9ms (1K) to 843ms (5K). Throughput only increases ~20% while player count increased 5x, meaning most time is spent waiting.
- **Zero errors at all levels:** The server never crashes, returns 500s, or drops connections even under heavy load. Very stable.
- **Bottleneck:** Likely the PostgreSQL connection pool and per-request DB queries (~10 reads + 2-20 writes per action request). The per-user mutex lock also serializes concurrent requests from the same user.

### Observations

- Registration phase scales linearly (~6s for 1,000 players, ~30s for 5,000) due to bcrypt hashing cost.
- The game action rate limiter (7,200/min per user = 120/sec) was never hit since each player sends ~5 req/sec.
- Auth IP-based rate limit (10/min) requires unique X-Forwarded-For per simulated player during registration.

### Potential Optimizations

- Connection pooling: increase PostgreSQL max connections and pgx pool size
- Batch DB queries: combine the ~10 individual queries per request into fewer queries
- Read caching: cache game state in memory with write-through to DB
- Async writes: queue non-critical writes (event logs, history) for background processing
