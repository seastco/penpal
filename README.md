# penpal

A terminal messaging app where letters take real time to travel between US cities.

Write a letter to a friend in another city and watch it hop across the country — routed through real US cities, arriving in hours or days depending on how you ship it. Messages are end-to-end encrypted; the server relays letters it cannot read.

<!-- TODO: add terminal screenshot -->

## Features

- **Real-time transit** — letters travel through US cities at realistic speeds, with tracking updates at each relay hop
- **End-to-end encryption** — NaCl box (XSalsa20-Poly1305) with ed25519 keys derived from a 12-word recovery phrase
- **3 shipping tiers** — First Class (~700 mi/day), Priority (~1500 mi/day), Express (~2000 mi/day)
- **City routing** — Dijkstra shortest path on a graph of ~30K US cities, capped at 10 relay hops
- **Stamp collecting** — earn common, rare, and ultra-rare stamps through registration and weekly rewards
- **TOFU key pinning** — first-contact public keys are cached locally; key changes are flagged as potential MITM
- **Account recovery** — your 12-word mnemonic *is* your account; re-enter it on any device to restore access

## Install

<!-- TODO: curl install one-liner -->

On first launch you'll register: choose a username, write down your 12-word recovery phrase, and pick a home city.

## How It Works

When you send a letter, the server computes a route from your city to the recipient's city using Dijkstra's algorithm on a precomputed K-nearest-neighbor graph of US cities (K=8). Long routes are capped at 10 relay hops with randomly sampled interior waypoints, so each letter takes a slightly different path.

Each hop gets an ETA based on the shipping tier's speed plus handling overhead. The server's delivery loop runs every 30 seconds, checking which letters have a `release_at` time in the past. There's no background state machine — the current hop is always determined by comparing `now()` against the stored ETAs.

## Stamps

Every user earns stamps through registration and weekly rewards. Stamps come in three rarities:

| Rarity | Examples |
|---|---|
| Common | flag, heart, star, quill, blossom, sunflower |
| Rare | state-specific stamps (e.g. `state:ma`) |
| Ultra Rare | TBD |

## Security

Messages are end-to-end encrypted — the server never sees plaintext. See [SECURITY.md](SECURITY.md) for the full cryptographic design.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PENPAL_DB` | `postgres://localhost:5432/penpal?sslmode=disable` | Postgres connection string |
| `PENPAL_LISTEN` | `:8282` | Server listen address |
| `PENPAL_CITIES` | `data/graph.json` | Path to precomputed city graph |
| `PENPAL_SERVER` | `ws://localhost:8282` | Client WebSocket URL |
| `PENPAL_HOME` | `~/.penpal` | Client config directory |

## Development

```bash
# Run tests
go test ./internal/crypto -v              # crypto unit tests
go test ./internal/routing -v             # routing unit tests
go test ./internal/client -v              # TUI unit tests

# End-to-end test (requires running server + database)
go test ./internal/ -v -run TestEndToEnd

# Lint
go vet ./...

# Rebuild city graph from source data
go run ./cmd/preprocess -input data/us_cities_continental.json -output data/graph.json
```

## License

TBD
