## Summary

<!-- What changed and why (1-3 sentences). -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor / code improvement
- [ ] Infrastructure / deployment
- [ ] Database migration
- [ ] Game balance / catalog
- [ ] Documentation

## High-risk areas touched

<!-- Check any that apply. See _documents/spec/review-strategy.md section 2 for details. -->

- [ ] Game engine (`engine.go`) — currency math, actions, prestige
- [ ] Game handler (`game.go`) — tick system, persistence, WS handler
- [ ] Batch data loader (`batch.go`) — column/scan alignment
- [ ] Auth flow — JWT, login, registration
- [ ] WebSocket protocol — connection lifecycle, goroutines
- [ ] Catalog definitions — costs, multipliers, balance
- [ ] Database migrations — schema changes

## Testing

<!-- What was tested and how to verify. Include test output for engine or balance changes. -->

- [ ] Ran `go test ./...` (backend)
- [ ] Ran `pnpm typecheck` in `packages/shared/`
- [ ] Manual testing: <!-- describe what you tested manually -->

## Checklist

- [ ] Tests pass locally
- [ ] No unrelated changes included
- [ ] Error paths return appropriate HTTP status codes
- [ ] New DB tables have GRANT statements documented
- [ ] No new TODO/FIXME without a linked issue
- [ ] Migration includes rollback SQL (if applicable)
