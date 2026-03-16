# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
# Build binaries
go build ./cmd/penpal      # TUI client
go build ./cmd/server      # relay server
go build ./cmd/preprocess   # city graph preprocessor
go build ./cmd/seed         # seed data generator

# Run server (requires PostgreSQL)
PENPAL_DB="postgres://localhost:5432/penpal?sslmode=disable" go run ./cmd/server

# Run client
go run ./cmd/penpal
```

## Testing

```bash
go test ./internal/crypto -v              # crypto unit tests
go test ./internal/routing -v             # routing unit tests
go test ./internal/client -v              # TUI unit tests
go test ./internal/ -v -run TestEndToEnd  # e2e test (requires running server + DB)
go vet ./...                               # lint
```

The e2e test (`internal/e2e_test.go`) requires a running server and database. It exercises the full flow: registration, contacts, encryption, send, delivery, decrypt, stamps, and account recovery.

## Architecture

**Terminal messaging app where letters take real time to travel between US cities.** Go monorepo with a Bubbletea TUI client and a WebSocket relay server backed by Postgres.

### Client-Server Protocol
- WebSocket at `/v1/ws` using `nhooyr.io/websocket` (not gorilla, which is archived)
- JSON `Envelope` with type field, payload, and optional reqID for request-response correlation
- Message types defined in `internal/protocol/protocol.go`

### Crypto Pipeline
- BIP39 mnemonic → PBKDF2-SHA512 (210K iterations, fixed salt) → ed25519 seed
- ed25519 for signing and challenge-response authentication
- ed25519 keys converted to X25519 (via SHA-512 clamping) for NaCl box encryption
- TOFU key pinning: client caches remote public keys in `~/.penpal/known_keys`

### Letter Routing
- Cities stored as a precomputed K-nearest-neighbor graph (K=8) in `data/graph.json`
- Dijkstra shortest path from sender's nearest city to recipient's
- Long routes capped at 10 relay hops with randomly sampled interior points
- ETA per hop = distance ÷ shipping tier speed; stored as JSONB in Postgres
- Current hop determined by comparing `now()` against ETAs (no background state machine)
- Server delivery loop runs every 30s checking `release_at <= now()`

### Database
- Postgres with embedded SQL migrations (`internal/db/migrations/`, `//go:embed`)
- Migrations auto-run on server startup via `db.Migrate(ctx)`
- **During development: modify 001_initial.sql directly instead of adding new migration files.** Drop and recreate the DB (`dropdb penpal && createdb penpal`) to apply changes.

### TUI (Bubbletea)
- `TUI` main model manages screen transitions; each screen implements `tea.Model`
- Shared `AppState` struct holds user identity, contacts, inbox, sent, in-transit data
- Network requests are async via Bubbletea's command/message pattern (`internal/client/network.go`)

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PENPAL_DB` | `postgres://localhost:5432/penpal?sslmode=disable` | Postgres connection string |
| `PENPAL_LISTEN` | `:8282` | Server listen address |
| `PENPAL_CITIES` | `data/graph.json` | Path to precomputed city graph |
| `PENPAL_SERVER` | `ws://localhost:8282` | Client WebSocket URL |
| `PENPAL_HOME` | `~/.penpal` | Client config directory |

## Data Files

- `data/cities_dev.json` — 229 cities for fast dev/test
- `data/graph_dev.json` / `graph_e2e.json` — precomputed dev/test graphs
- `data/graph.json` — full production graph (~30K cities, 11.2 MB)
- Rebuild graph: `go run ./cmd/preprocess -input data/us_cities_continental.json -output data/graph.json`
