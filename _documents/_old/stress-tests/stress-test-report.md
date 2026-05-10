# Homelab the Game — Stress Test Report

**Date:** 2026-03-22
**Server:** Single VM, Go backend + PostgreSQL + TimescaleDB
**Test Tool:** Custom Go stress tester (`apps/stress-tests/`)
**Test Duration:** 30 seconds per run
**Action Rate:** 500ms per player (each player performs one action every 500ms)

---

## Test Configurations

Four rounds of stress testing were performed on the same codebase to measure the impact of vertical scaling and transport mode.

|                   | Test Set 1                 | Test Set 2                      | Test Set 3                 | Test Set 4                 |
| ----------------- | -------------------------- | ------------------------------- | -------------------------- | -------------------------- |
| **CPU**           | 12 vCPU                    | 32 vCPU                         | 32 vCPU                    | 32 vCPU                    |
| **RAM**           | 16 GB                      | 32 GB                           | 32 GB                      | 32 GB                      |
| **Mode**          | WebSocket-only             | WebSocket + HTTP                | WebSocket-only             | HTTP-only                  |
| **Player counts** | 50, 200, 500, 1,000, 2,000 | 50, 100, 250, 500, 1,000, 2,000 | 50, 200, 500, 1,000, 2,000 | 50, 200, 500, 1,000, 2,000 |

**WebSocket-only mode** routes all game actions through WebSocket connections (no HTTP action requests).
**WebSocket + HTTP mode** uses WebSocket for game actions while also performing HTTP state-fetch requests in parallel (80% WS actions, 20% HTTP state fetches).
**HTTP-only mode** sends all game actions and state fetches over standard HTTP requests (no WebSocket connections).

---

## Test Set 1: 12 vCPU / 16 GB RAM (WebSocket-Only)

All game actions sent exclusively over WebSocket connections.

### Summary Table

| Players | Error Rate | WS Actions/sec | WS P50 |    WS P90 |    WS P95 |    WS P99 |     WS Max |
| ------: | :--------: | -------------: | -----: | --------: | --------: | --------: | ---------: |
|      50 |   0.00%    |           91.6 |  2.9ms |     9.8ms |    13.6ms |    58.8ms |    102.5ms |
|     200 |   0.00%    |          365.6 |  5.8ms |    70.2ms |   108.2ms |   194.6ms |    345.4ms |
|     500 |   0.00%    |          907.4 |  6.4ms |    67.1ms |   220.3ms |   464.7ms |    868.9ms |
|   1,000 |   0.00%    |        1,602.2 | 13.2ms |   489.2ms |   895.9ms | 1,531.5ms |  2,846.9ms |
|   2,000 | **0.10%**  |        1,488.1 | 54.3ms | 2,802.1ms | 4,475.8ms | 7,364.0ms | 10,002.5ms |

### Detailed Results

#### 50 Players

| Metric             | Value |
| ------------------ | ----- |
| Connected          | 50/50 |
| Successful actions | 3,208 |
| Failed actions     | 0     |
| WS Average latency | 6.5ms |

#### 200 Players

| Metric             | Value   |
| ------------------ | ------- |
| Connected          | 200/200 |
| Successful actions | 12,834  |
| Failed actions     | 0       |
| WS Average latency | 20.9ms  |

#### 500 Players

| Metric             | Value   |
| ------------------ | ------- |
| Connected          | 500/500 |
| Successful actions | 32,114  |
| Failed actions     | 0       |
| WS Average latency | 31.0ms  |

#### 1,000 Players

| Metric             | Value       |
| ------------------ | ----------- |
| Connected          | 1,000/1,000 |
| Successful actions | 66,056      |
| Failed actions     | 0           |
| WS Average latency | 128.6ms     |

#### 2,000 Players

| Metric             | Value       |
| ------------------ | ----------- |
| Connected          | 2,000/2,000 |
| Successful actions | 71,165      |
| Failed actions     | **68**      |
| WS Average latency | 781.4ms     |

> At 2,000 players on 12 vCPU, the server starts dropping actions (0.10% error rate), P99 latency hits 7.4 seconds, and max latency reaches the 10-second timeout. Throughput actually decreases vs. 1,000 players (1,488 vs. 1,602 actions/sec), indicating the server is past saturation.

---

## Test Set 2: 32 vCPU / 32 GB RAM (WebSocket + HTTP)

Game actions sent over WebSocket with concurrent HTTP state-fetch requests.

### Summary Table — WebSocket Actions

| Players | Error Rate | WS Actions/sec | WS P50 |  WS P90 |    WS P95 |    WS P99 |    WS Max |
| ------: | :--------: | -------------: | -----: | ------: | --------: | --------: | --------: |
|      50 |   0.00%    |           73.8 |  2.0ms |   5.5ms |     6.0ms |     7.7ms |    22.4ms |
|     100 |   0.00%    |          146.2 |  1.9ms |   5.4ms |     5.9ms |     6.8ms |    23.5ms |
|     250 |   0.00%    |          364.5 |  1.9ms |   5.9ms |     6.5ms |    27.2ms |    73.9ms |
|     500 |   0.00%    |          725.8 |  2.2ms |   6.7ms |     7.5ms |    80.2ms |   179.4ms |
|   1,000 |   0.00%    |        1,447.3 |  5.5ms |  18.0ms |   183.2ms |   430.2ms |   728.1ms |
|   2,000 |   0.00%    |        2,219.3 | 16.4ms | 964.6ms | 1,260.7ms | 1,845.2ms | 2,761.9ms |

### Summary Table — HTTP State Fetches

| Players | Error Rate | HTTP Req/sec | HTTP P50 |  HTTP P90 |  HTTP P95 |  HTTP P99 |  HTTP Max |
| ------: | :--------: | -----------: | -------: | --------: | --------: | --------: | --------: |
|      50 |   0.00%    |         17.8 |    5.3ms |     6.4ms |     6.8ms |    14.2ms |    24.0ms |
|     100 |   0.00%    |         36.9 |    5.1ms |     6.2ms |     6.6ms |     8.7ms |    22.9ms |
|     250 |   0.00%    |         92.0 |    5.2ms |     6.4ms |     6.9ms |    55.2ms |    69.6ms |
|     500 |   0.00%    |        185.5 |    5.8ms |     7.2ms |     8.1ms |   122.4ms |   150.9ms |
|   1,000 |   0.00%    |        365.2 |    7.0ms |   270.3ms |   417.3ms |   538.3ms |   581.6ms |
|   2,000 |   0.00%    |        546.1 |  979.6ms | 1,600.7ms | 1,664.9ms | 1,712.3ms | 2,653.8ms |

### Detailed Results

#### 50 Players

| Metric                | Value |
| --------------------- | ----- |
| WS connected          | 50/50 |
| WS successful actions | 2,586 |
| WS failed actions     | 0     |
| HTTP total requests   | 624   |
| HTTP failed           | 0     |

#### 100 Players

| Metric                | Value   |
| --------------------- | ------- |
| WS connected          | 100/100 |
| WS successful actions | 5,125   |
| WS failed actions     | 0       |
| HTTP total requests   | 1,293   |
| HTTP failed           | 0       |

#### 250 Players

| Metric                | Value   |
| --------------------- | ------- |
| WS connected          | 250/250 |
| WS successful actions | 12,804  |
| WS failed actions     | 0       |
| HTTP total requests   | 3,230   |
| HTTP failed           | 0       |

#### 500 Players

| Metric                | Value   |
| --------------------- | ------- |
| WS connected          | 500/500 |
| WS successful actions | 25,568  |
| WS failed actions     | 0       |
| HTTP total requests   | 6,534   |
| HTTP failed           | 0       |

#### 1,000 Players

| Metric                | Value       |
| --------------------- | ----------- |
| WS connected          | 1,000/1,000 |
| WS successful actions | 51,472      |
| WS failed actions     | 0           |
| HTTP total requests   | 12,990      |
| HTTP failed           | 0           |

#### 2,000 Players

| Metric                | Value       |
| --------------------- | ----------- |
| WS connected          | 2,000/2,000 |
| WS successful actions | 81,572      |
| WS failed actions     | 0           |
| HTTP total requests   | 20,072      |
| HTTP failed           | 0           |

---

## Test Set 3: 32 vCPU / 32 GB RAM (WebSocket-Only)

All game actions sent exclusively over WebSocket connections — identical mode to Test Set 1 for a direct hardware comparison.

### Summary Table

| Players | Error Rate | WS Actions/sec | WS P50 |    WS P90 |    WS P95 |    WS P99 |    WS Max |
| ------: | :--------: | -------------: | -----: | --------: | --------: | --------: | --------: |
|      50 |   0.00%    |           91.5 |  1.9ms |     6.9ms |     8.2ms |    12.1ms |    21.2ms |
|     200 |   0.00%    |          364.8 |  2.1ms |    13.6ms |    17.0ms |    69.5ms |   146.9ms |
|     500 |   0.00%    |          911.5 |  2.3ms |     8.1ms |     9.8ms |   100.3ms |   235.2ms |
|   1,000 |   0.00%    |        1,807.6 |  6.5ms |    23.2ms |   233.5ms |   498.5ms |   807.1ms |
|   2,000 |   0.00%    |        2,516.7 | 31.6ms | 1,571.2ms | 2,212.8ms | 3,428.6ms | 4,926.7ms |

### Detailed Results

#### 50 Players

| Metric             | Value |
| ------------------ | ----- |
| Connected          | 50/50 |
| Successful actions | 3,207 |
| Failed actions     | 0     |
| WS Average latency | 3.9ms |

#### 200 Players

| Metric             | Value   |
| ------------------ | ------- |
| Connected          | 200/200 |
| Successful actions | 12,825  |
| Failed actions     | 0       |
| WS Average latency | 7.1ms   |

#### 500 Players

| Metric             | Value   |
| ------------------ | ------- |
| Connected          | 500/500 |
| Successful actions | 32,126  |
| Failed actions     | 0       |
| WS Average latency | 6.5ms   |

#### 1,000 Players

| Metric             | Value       |
| ------------------ | ----------- |
| Connected          | 1,000/1,000 |
| Successful actions | 64,486      |
| Failed actions     | 0           |
| WS Average latency | 30.3ms      |

#### 2,000 Players

| Metric             | Value       |
| ------------------ | ----------- |
| Connected          | 2,000/2,000 |
| Successful actions | 92,671      |
| Failed actions     | 0           |
| WS Average latency | 371.5ms     |

> At 2,000 players on 32 vCPU (WS-only), the server handles all connections with zero errors and 2,517 actions/sec throughput — a 69% improvement over the 12 vCPU result (1,488/sec). P99 latency is 3.4s vs. 7.4s, and max latency is 4.9s vs. 10s.

---

## Test Set 4: 32 vCPU / 32 GB RAM (HTTP-Only)

All game actions and state fetches sent over standard HTTP requests — no WebSocket connections.

### Summary Table

| Players | Error Rate | HTTP Req/sec | HTTP Actions/sec | HTTP P50 |  HTTP P90 |  HTTP P95 |  HTTP P99 |  HTTP Max |
| ------: | :--------: | -----------: | ---------------: | -------: | --------: | --------: | --------: | --------: |
|      50 |   0.00%    |         91.5 |             73.4 |    6.6ms |     7.5ms |     8.0ms |     9.3ms |    12.8ms |
|     200 |   0.00%    |        365.5 |            290.7 |    6.5ms |     8.1ms |     8.8ms |    10.1ms |    13.8ms |
|     500 |   0.00%    |        911.4 |            728.5 |    6.9ms |     9.5ms |    10.3ms |    11.5ms |    21.7ms |
|   1,000 |   0.00%    |      1,819.2 |          1,459.5 |    7.9ms |    11.7ms |    12.5ms |    14.0ms |    22.8ms |
|   2,000 |   0.00%    |      2,435.1 |          1,944.2 |   27.2ms | 1,524.5ms | 1,540.5ms | 1,576.5ms | 3,032.1ms |

### Detailed Results

#### 50 Players

| Metric             | Value |
| ------------------ | ----- |
| Total requests     | 3,205 |
| Successful actions | 2,572 |
| Failed actions     | 0     |
| HTTP Average latency | 4.9ms |

#### 200 Players

| Metric             | Value  |
| ------------------ | ------ |
| Total requests     | 12,831 |
| Successful actions | 10,206 |
| Failed actions     | 0      |
| HTTP Average latency | 5.0ms |

#### 500 Players

| Metric             | Value  |
| ------------------ | ------ |
| Total requests     | 32,122 |
| Successful actions | 25,677 |
| Failed actions     | 0      |
| HTTP Average latency | 5.5ms |

#### 1,000 Players

| Metric             | Value  |
| ------------------ | ------ |
| Total requests     | 64,384 |
| Successful actions | 51,652 |
| Failed actions     | 0      |
| HTTP Average latency | 6.6ms |

#### 2,000 Players

| Metric             | Value  |
| ------------------ | ------ |
| Total requests     | 88,596 |
| Successful actions | 70,736 |
| Failed actions     | 0      |
| HTTP Average latency | 394.2ms |

> HTTP-only mode is dramatically faster than WebSocket at every player count up to 1,000 — P99 stays under 14ms even at 1,000 players (vs. 498ms for WS-only). At 2,000 players, latency jumps sharply (P90 hits 1.5s), but throughput peaks at 2,435 req/sec with zero errors.

---

## Comparison: Hardware Scaling (WS-Only, Apples-to-Apples)

Direct comparison using the same test mode (WebSocket-only) on both hardware configurations.

### WebSocket P99 Latency (Lower is Better)

| Players | 12 vCPU / 16 GB | 32 vCPU / 32 GB | Improvement |
| ------: | ---------------: | ---------------: | :---------: |
|      50 |           58.8ms |           12.1ms |  **4.9x**   |
|     200 |          194.6ms |           69.5ms |  **2.8x**   |
|     500 |          464.7ms |          100.3ms |  **4.6x**   |
|   1,000 |        1,531.5ms |          498.5ms |  **3.1x**   |
|   2,000 |        7,364.0ms |        3,428.6ms |  **2.1x**   |

### WebSocket Throughput (Higher is Better)

| Players | 12 vCPU / 16 GB | 32 vCPU / 32 GB |  Change  |
| ------: | ---------------: | ---------------: | :------: |
|      50 |        91.6/sec |        91.5/sec |    —     |
|     200 |       365.6/sec |       364.8/sec |    —     |
|     500 |       907.4/sec |       911.5/sec |    —     |
|   1,000 |     1,602.2/sec |     1,807.6/sec | **+13%** |
|   2,000 |     1,488.1/sec |     2,516.7/sec | **+69%** |

> Throughput is identical up to 500 players — the bottleneck at lower player counts is the action rate (500ms/player), not hardware. The extra CPUs only matter once the server is under contention at 1,000+ players.

### WebSocket Average Latency

| Players | 12 vCPU / 16 GB | 32 vCPU / 32 GB | Improvement |
| ------: | ---------------: | ---------------: | :---------: |
|      50 |           6.5ms |           3.9ms |  **1.7x**   |
|     200 |          20.9ms |           7.1ms |  **2.9x**   |
|     500 |          31.0ms |           6.5ms |  **4.8x**   |
|   1,000 |         128.6ms |          30.3ms |  **4.2x**   |
|   2,000 |         781.4ms |         371.5ms |  **2.1x**   |

### Error Rate

|  Players |         12 vCPU         | 32 vCPU |
| -------: | :---------------------: | :-----: |
| 50-1,000 |          0.00%          |  0.00%  |
|    2,000 | **0.10%** (68 failures) |  0.00%  |

---

## Comparison: Transport Mode (32 vCPU — All Three Modes)

Isolates the impact of transport protocol on the same hardware. P99 latency comparison uses the primary channel for each mode (HTTP for HTTP-only, WS for WS-only, WS for WS+HTTP).

### P99 Latency (Lower is Better)

| Players | HTTP-only (Set 4) | WS+HTTP (Set 2) | WS-only (Set 3) |
| ------: | ----------------: | ---------------: | ---------------: |
|      50 |             9.3ms |            7.7ms |           12.1ms |
|     200 |            10.1ms |              —   |           69.5ms |
|     500 |            11.5ms |           80.2ms |          100.3ms |
|   1,000 |            14.0ms |          430.2ms |          498.5ms |
|   2,000 |         1,576.5ms |        1,845.2ms |        3,428.6ms |

### Throughput (Higher is Better)

| Players | HTTP-only (Set 4) | WS+HTTP (Set 2) | WS-only (Set 3) |
| ------: | ----------------: | ---------------: | ---------------: |
|      50 |         91.5/sec |              —   |        91.5/sec |
|     200 |        365.5/sec |              —   |       364.8/sec |
|     500 |        911.4/sec |       725.8/sec |       911.5/sec |
|   1,000 |      1,819.2/sec |     1,447.3/sec |     1,807.6/sec |
|   2,000 |      2,435.1/sec |     2,219.3/sec |     2,516.7/sec |

### Average Latency

| Players | HTTP-only (Set 4) | WS+HTTP (Set 2) | WS-only (Set 3) |
| ------: | ----------------: | ---------------: | ---------------: |
|      50 |            4.9ms |           3.4ms |           3.9ms |
|     200 |            5.0ms |              —   |           7.1ms |
|     500 |            5.5ms |           5.4ms |           6.5ms |
|   1,000 |            6.6ms |          25.7ms |          30.3ms |
|   2,000 |          394.2ms |         225.7ms |         371.5ms |

> **HTTP-only is the clear latency winner up to 1,000 players** — P99 stays under 14ms even at 1,000 concurrent players, while WebSocket modes hit 430-498ms. This is because HTTP requests are stateless and short-lived, avoiding the connection management and broadcast overhead of WebSocket. At 2,000 players, all modes converge toward similar high latency, indicating the bottleneck shifts to database/game engine contention regardless of transport.

---

## Capacity Analysis

### Latency Thresholds

For an idle/clicker game, these are practical latency budgets:

- **Excellent** (< 50ms P99): Players won't notice any delay
- **Good** (50-200ms P99): Acceptable for an idle/clicker game
- **Degraded** (200ms-1s P99): Noticeable lag on some actions
- **Poor** (> 1s P99): Actively hurts the player experience

### Recommended Capacity Per Configuration

| Config                           | Excellent      | Good           | Degraded       | Max (before errors) |
| -------------------------------- | -------------- | -------------- | -------------- | ------------------- |
| **12 vCPU / 16 GB (WS-only)**   | ~50 players    | ~200 players   | ~500 players   | ~1,000 players      |
| **32 vCPU / 32 GB (WS+HTTP)**   | ~250 players   | ~500 players   | ~1,000 players | 2,000+ players      |
| **32 vCPU / 32 GB (WS-only)**   | ~200 players   | ~500 players   | ~1,000 players | 2,000+ players      |
| **32 vCPU / 32 GB (HTTP-only)** | ~1,000 players | ~1,000 players | ~1,500 players | 2,000+ players      |

### Scaling Observations

1. **HTTP-only mode is dramatically faster than WebSocket** — P99 of 14ms at 1,000 players vs. 430-498ms for WebSocket modes, a 30-35x improvement
2. **Vertical scaling provides ~2-5x latency improvement** going from 12 to 32 vCPU, with the biggest gains at moderate load (200-500 players)
3. **Throughput gains only appear above 500 players** — below that, the per-player action rate is the bottleneck, not hardware
4. **The server never drops connections** — all players successfully connect even at 2,000 on all configurations
5. **Latency degrades gracefully** — no cliff-edge failures, just steadily increasing response times
6. **32 vCPU eliminates errors at 2,000 players** — 0% error rate vs. 0.10% on 12 vCPU
7. **Throughput ceiling is ~2,500 req/sec** across all modes on 32 vCPU, suggesting a processing bottleneck beyond raw CPU (likely database or game engine tick contention)
8. **P50 latency remains low even under heavy load** — most requests are fast, but tail latency (P99) spikes significantly due to periodic blocking
9. **WebSocket overhead is significant** — the persistent connection management and broadcast fan-out add substantial latency compared to stateless HTTP, especially at 500-1,000 players
10. **All modes converge at 2,000 players** — P99 hits 1.5-3.4s regardless of transport, confirming the bottleneck is server-side processing, not the network layer

### Bottleneck Indicators

- **12 vCPU at 2,000 players:** Throughput regression (1,488 vs 1,602 at 1,000) + errors = CPU saturation
- **32 vCPU at 2,000 players:** No errors but P99 at 1.6-3.4s across all modes = database or lock contention
- **P50-to-P99 spread widens dramatically** at higher player counts, suggesting periodic blocking (e.g., game tick processing, database writes, or GC pauses)
- **HTTP-only stays flat until 2,000 players** — the sudden jump from 14ms to 1,576ms P99 suggests a hard contention threshold between 1,000 and 2,000 concurrent players
