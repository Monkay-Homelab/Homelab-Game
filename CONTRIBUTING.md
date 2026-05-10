# Contributing to Homelab the Game

Welcome! This guide helps you get set up and contributing quickly. For deeper detail on
architecture, conventions, and game design, see `CLAUDE.md` and the `_documents/` directory.

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.25+ | Backend server |
| Node.js | 22+ | Frontend tooling |
| pnpm | latest | JavaScript package manager |
| PostgreSQL | 16 | Primary database |
| TimescaleDB | extension | Time-series analytics tables |
| Rust + Cargo | latest | Tauri desktop builds only (optional for web dev) |

## Getting Started

### 1. Clone and install dependencies

```bash
git clone <repo-url>
cd homelab-the-game
pnpm install          # Install all JS/TS workspace dependencies
```

### 2. Set up the database

PostgreSQL 16 with TimescaleDB must be running locally. The database and user are both
named `homelab_game`.

Apply migrations in order:

```bash
cat apps/backend/internal/database/migrations/NNN_name.sql | sudo -u postgres psql -d homelab_game
```

Grant permissions on any new tables:

```bash
echo "GRANT ALL ON <table_name> TO homelab_game;" | sudo -u postgres psql -d homelab_game
```

### 3. Configure environment

The backend reads from environment variables or a `.env` file in `apps/backend/`. Key
variables:

| Variable | Default | Notes |
|---|---|---|
| `PORT` | `8080` | Backend server port |
| `DB_HOST` | `localhost` | |
| `DB_PORT` | `5432` | |
| `DB_USER` | `homelab_game` | |
| `DB_PASSWORD` | `""` | |
| `DB_NAME` | `homelab_game` | |
| `JWT_SECRET` | auto-generated | Logs a warning if missing |
| `REDIS_ADDR` | `redis:6379` | Optional -- falls back to in-memory |

The frontend uses `VITE_API_URL` (defaults to `https://api.homelab.living`). For local
development, set it to `http://localhost:8080`.

### 4. Run the backend

```bash
cd apps/backend
go run ./cmd/server/
```

The server starts on the configured port (default 8080). There is no hot reload -- restart
the process after code changes.

### 5. Run the frontend

```bash
cd apps/desktop
pnpm dev              # Vite dev server on port 3000 (hot reload)
```

## Development Workflow

1. **Branch from `main`**. Use descriptive branch names: `feature/description`,
   `fix/description`, `chore/description`.
2. **Make your changes**. Follow the code style and patterns described below.
3. **Test locally**. Run the checks listed in the Testing section.
4. **Open a pull request** against `main`. See PR Expectations below.

`main` is the production branch -- code merged to main runs directly on the production
server. There is no staging environment.

## Code Style

### Go (backend)

- Follow standard Go naming conventions: `MixedCaps` for exports, `mixedCaps` for
  unexported. Uppercase acronyms (`UserID`, `JWT`, `CU`).
- Error messages: lowercase, no trailing punctuation. Include relevant values
  (e.g., `"need %d, have %d"`).
- HTTP errors: return `{"error":"..."}` JSON using `http.Error()` with inline JSON strings.
- Use `%w` wrapping only in the database layer. The engine creates new error values with
  user-facing messages.
- New query files go in `database/queries/` with a `New<Entity>Queries(pool)` constructor.
- No inline `TODO`/`FIXME` annotations -- track work items externally.

### TypeScript (frontend)

- **Strict mode** is enforced (`"strict": true` in `tsconfig.json`).
- React components: named function exports (`export function ComponentName()`), props typed
  inline.
- State management: add new actions to the `GameStore` interface in `gameStore.ts`.
- Styling: Tailwind CSS for layout, CSS custom properties (`--currency-*`, `--bg-*`,
  `--accent-*`) for colors. Use the `CURRENCY_COLORS` utility from
  `utils/currencyColors.ts` -- never hardcode currency color hex values.

See `_documents/spec/code-quality.md` for the full conventions reference.

## Testing

### Backend

```bash
cd apps/backend
go test ./...
```

Tests use the standard library `testing` package. Follow these patterns:
- Table-driven tests for parameterized cases.
- `httptest.NewRequest`/`httptest.NewRecorder` for HTTP handler tests.
- Mark test helpers with `t.Helper()`.

### Frontend

```bash
cd packages/shared
pnpm typecheck        # TypeScript type checking (shared package)
```

No frontend test framework is configured yet. At minimum, verify your changes compile
with `pnpm build` in `apps/desktop/`.

### CI checks

The GitHub Actions workflow (`.github/workflows/build.yml`) runs on every PR:
- `go test ./...` (backend)
- `pnpm typecheck` (shared package)

Both must pass before merge.

## Pull Request Expectations

When opening a PR, include:

1. **Summary**: What changed and why (1-3 sentences).
2. **Risk assessment**: Which areas does this touch? Game engine and currency math changes
   need extra scrutiny.
3. **Testing**: What was tested and how? Include test output for game engine changes.
4. **Migration**: If a database migration is required, include rollback SQL.
5. **Catalog/balance changes**: If game numbers changed, explain the rationale.

### Pre-merge checklist

- [ ] `go test ./...` passes
- [ ] `pnpm typecheck` passes in `packages/shared/`
- [ ] Error paths return appropriate HTTP status codes
- [ ] New endpoints are added to `routes/routes.go` with appropriate middleware
- [ ] New DB tables have `GRANT` statements documented
- [ ] Catalog changes have corresponding engine validation
- [ ] WebSocket message format changes are backward-compatible

## Key Patterns to Follow

These patterns are load-bearing -- deviating from them causes bugs or security issues.

### Server-authoritative

All game state mutations go through `engine.ProcessAction()` on the backend. The client
sends actions and renders the server's response. Never validate or mutate game state
client-side (client-side interpolation between ticks via `requestAnimationFrame` is the
one exception).

### Per-user mutex

Always acquire the per-user lock before reading or writing a user's game state. The
`userMutexMap` in `game.go` serializes concurrent actions for the same user. Verify that
every code path releases the lock, including error paths and panics.

### Batch database loading

Use the `LoadFullGameState` pattern in `database/queries/batch.go` -- it batches 8+
child-table queries into 2 database round-trips. Do not add individual queries for data
that is already loaded in the batch.

### Currency color system

Six currencies (CU, Money, Reputation, Knowledge Points, Bitcoin, Power) each have a color
defined via CSS custom properties (`--currency-*` in `global.css`) and a TypeScript utility
(`CURRENCY_COLORS` in `utils/currencyColors.ts`). Cost buttons use the currency color (what
it costs), not the category color (where it lives). See `_documents/ux/currency-colors.md`.

### CORS origins

The CORS allowlist is maintained in two places: `middleware/cors.go` and `ws/hub.go`. If
you add a new allowed origin, update both files.

## Where to Learn More

| Topic | Location |
|---|---|
| Project overview and architecture | `CLAUDE.md` |
| Game design | `_documents/PLAN.md` |
| Implementation roadmap | `_documents/ROADMAP.md` |
| Technical design documents | `_documents/tdd/` |
| UX design specs | `_documents/ux/` |
| Code conventions (full detail) | `_documents/spec/code-quality.md` |
| Review strategy and risk map | `_documents/spec/review-strategy.md` |
