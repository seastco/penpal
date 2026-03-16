# Architecture

penpal is a Go monorepo with two binaries: a Bubbletea TUI client and a WebSocket relay server backed by Postgres. This document explains how the system works.

## Project Structure

```
cmd/
  penpal/         TUI client entry point
  server/         Relay server entry point
  preprocess/     City graph preprocessor (builds graph.json from raw city data)
  seed/           Test data generator

internal/
  client/         TUI screens, network layer, styles, drafts, PIN lock
  server/         WebSocket handlers, delivery loop, hub, stamp awards
  protocol/       Shared message types and request/response payloads
  models/         Domain types: User, Message, Stamp, RouteHop, ShippingTier
  db/             Postgres queries and embedded SQL migrations
  crypto/         BIP39 seed derivation, ed25519 keypairs, NaCl box encryption
  routing/        City graph, Dijkstra shortest path, haversine distance

data/
  graph.json          Production city graph (~30K cities, 11.2 MB, gitignored)
  graph_dev.json      Dev graph (229 cities)
  graph_e2e.json      E2E test graph
  cities_dev.json     Raw city data for dev
```

## Client-Server Protocol

All communication happens over a single WebSocket connection at `/v1/ws` using `nhooyr.io/websocket`.

Every message is a JSON envelope:

```json
{
  "type": "send_letter",
  "payload": { ... },
  "req_id": "abc123",
  "error": ""
}
```

The `type` field identifies the message. `req_id` is an optional correlation ID that the server echoes back, allowing the client to match responses to requests. `error` is set on failure responses.

Messages are grouped by flow:

**Authentication:** `auth` > `auth_challenge` > `auth_response` > `auth_ok`. The server sends a 32-byte random nonce; the client signs it with ed25519. Registration uses `register` > `register_ok`. Recovery uses `recover` > `recover_ok`.

**Letters:** `send_letter` > `letter_sent`. The client encrypts the body before sending. The server computes the route, stores the message, and returns the route and ETA.

**Reading:** `get_inbox`, `get_sent`, `get_in_transit`, `get_tracking`, `get_message`, `mark_read`.

**Contacts:** `get_contacts`, `add_contact`, `delete_contact`, `block_user`.

**Other:** `get_stamps`, `get_shipping`, `get_public_key`, `search_cities`, `update_home_city`.

All message types and their payloads are defined in `internal/protocol/protocol.go`.

## Letter Lifecycle

1. **Compose.** The sender picks a recipient from their contacts, writes the letter body, selects a stamp, and chooses a shipping tier.

2. **Encrypt.** The client fetches the recipient's ed25519 public key from the server, converts both keys to X25519, and encrypts the body using NaCl box (XSalsa20-Poly1305) with a fresh random nonce. See [SECURITY.md](SECURITY.md) for details.

3. **Send.** The client sends a `send_letter` message containing the encrypted body, recipient ID, shipping tier, and stamp IDs.

4. **Route.** The server finds the nearest graph node to both the sender's and recipient's home cities, runs Dijkstra to find the shortest path, and samples it down to at most 10 relay hops. Each hop gets an ETA based on: `distance / tier speed + handling overhead`. The full route is stored as JSONB in the `messages` table.

5. **In transit.** The message sits in the database with `status = 'in_transit'`. There is no background state machine tracking hop-by-hop progress. The current hop is always computed on the fly by comparing `now()` against the stored ETAs.

6. **Delivery.** A server loop runs every 30 seconds, querying for messages where `release_at <= now()` and `status = 'in_transit'`. Matching messages are flipped to `status = 'delivered'` and a push notification is sent to the recipient if they're connected.

7. **Read.** The recipient opens the letter. The client decrypts the body using their private key and the sender's public key. The server is notified via `mark_read`.

## City Routing

City data comes from a dataset of ~30K continental US cities. At build time, `cmd/preprocess` constructs a K-nearest-neighbor graph (K=8) where each city connects to its 8 closest neighbors by haversine distance. This graph is serialized to `data/graph.json` and loaded by the server on startup.

When a letter is sent, the server:

1. Finds the nearest graph node to the sender's coordinates
2. Finds the nearest graph node to the recipient's coordinates
3. Runs Dijkstra's algorithm to find the shortest path by distance
4. If the path has more than 10 cities, samples it down: keeps the first and last city, divides the interior into buckets, and randomly picks one city from each bucket (so every letter takes a slightly different route)
5. Schedules ETAs for each hop based on the shipping tier's speed

Shipping tier speeds:

| Tier | Speed | Handling |
|---|---|---|
| First Class | 700 mi/day | 2 days |
| Priority | 1,500 mi/day | 1 day |
| Express | 2,000 mi/day | 0.5 days |

## Database

Postgres with `lib/pq`. Migrations are embedded via `//go:embed` in `internal/db/migrations/` and auto-run on server startup. The schema has six tables:

- **users** - identity, public key, home city coordinates, discriminator (4-digit tag for duplicate usernames)
- **messages** - encrypted body, sender/recipient, shipping tier, route (JSONB array of hops with ETAs), status, timestamps
- **contacts** - bidirectional contact list (owner_id, contact_id)
- **stamps** - collectible stamps with type, rarity, and provenance (registration, weekly, delivery, transfer)
- **stamp_attachments** - links stamps to messages (one stamp per letter)
- **blocks** - user block list

Key indexes: `messages(status, release_at) WHERE status = 'in_transit'` for the delivery loop, `users(public_key)` for account recovery.

## TUI

Built with [Bubbletea](https://github.com/charmbracelet/bubbletea). The root model `TUI` (`internal/client/tui.go`) holds the current screen and handles transitions. Each screen (home, inbox, compose, tracking, stamps, settings, etc.) is its own `tea.Model` with `Init`, `Update`, and `View`.

Shared state lives in `AppState` (`internal/client/app.go`): user identity, keys, network connection, decrypted message cache, glamour renderer.

Network requests are async. The pattern: a screen returns a `tea.Cmd` (a function) that makes the network call and returns a message. Bubbletea runs the function in a goroutine and delivers the result message back to `Update`. This keeps the UI responsive during server calls.

The network layer (`internal/client/network.go`) wraps the WebSocket connection with request-response correlation using `req_id`. Each `Send` call generates a unique ID, sends the envelope, and waits for the matching response.

Client data is stored in `~/.penpal/`:
- `key` - ed25519 keypair (64 bytes)
- `identity` - username and discriminator
- `known_keys` - TOFU key pin store
- `pin` - optional PIN lock hash
- `theme` - selected color theme name
- `drafts/` - auto-saved letter drafts

## Data Files

The city graph must be precomputed before the server can start:

```bash
go run ./cmd/preprocess -input data/us_cities_continental.json -output data/graph.json
```

This takes about 2 minutes for the full dataset. For development, `data/graph_dev.json` (229 cities) is checked in and loads instantly.
