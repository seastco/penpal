# Security Model

penpal uses end-to-end encryption so the relay server transports messages it cannot read. This document describes the cryptographic design for developers who want to understand the threat model.

## How It Works (Plain English)

When you create a penpal account, you get a 12-word secret phrase. This phrase generates a unique pair of keys: a **public key** (like a mailing address anyone can see) and a **private key** (like a key to your mailbox that only you have).

When you send a letter, your app uses *your* private key and the *recipient's* public key to scramble the message into unreadable gibberish. Only the recipient can unscramble it, because only they have the matching private key. The penpal server just passes this gibberish along -- it never has the keys needed to read it.

**What the server can see:** who sent a letter to whom, when, and from which city. Basically, what a postal worker could read off an envelope.

**What the server can never see:** what you actually wrote.

**The catch:** your 12-word phrase *is* your account. Anyone who knows it can read your messages and pretend to be you. If you lose it, your account is gone. Write it down and keep it safe.

---

The rest of this document covers the cryptographic details for developers who want to understand the threat model.

## Key Generation

Account identity is derived entirely from a 12-word BIP39 mnemonic:

1. **Generate mnemonic** — 128 bits of entropy → 12-word BIP39 phrase
2. **Derive seed** — mnemonic → PBKDF2-SHA512 (210,000 iterations, fixed salt `penpal-ed25519-v1`) → 32-byte seed
3. **Generate keypair** — seed → ed25519 keypair

The same mnemonic always produces the same keypair. The mnemonic *is* the account — there are no passwords.

The fixed salt is acceptable because the mnemonic itself provides 128 bits of entropy, making precomputation attacks infeasible.

## Authentication

The server authenticates clients using ed25519 challenge-response:

1. Client sends username to initiate login
2. Server generates a 32-byte random nonce and sends it as a challenge
3. Client signs the nonce with their ed25519 private key
4. Server verifies the signature against the stored public key

No passwords are transmitted or stored. Authentication proves possession of the private key without revealing it.

## Encryption

Messages are encrypted using NaCl box (XSalsa20-Poly1305), which provides authenticated encryption:

1. **Key conversion** — ed25519 keys are converted to X25519 for Diffie-Hellman key agreement:
   - Private key: SHA-512 hash of the ed25519 seed, first 32 bytes clamped (identical to libsodium's `crypto_sign_ed25519_sk_to_curve25519`)
   - Public key: Edwards-to-Montgomery point conversion via `filippo.io/edwards25519`
2. **Encrypt** — sender's X25519 private key + recipient's X25519 public key + random 24-byte nonce → NaCl box
3. **Wire format** — `[24-byte nonce || ciphertext]` (nonce prepended to ciphertext)
4. **Decrypt** — recipient's X25519 private key + sender's X25519 public key + nonce from wire format → plaintext

Each message uses a fresh cryptographically random nonce.

## Key Pinning (TOFU)

penpal uses Trust-On-First-Use key pinning to detect key changes:

- **First contact**: the recipient's ed25519 public key is cached in `~/.penpal/known_keys`
- **Subsequent messages**: the cached key is compared byte-for-byte against the key provided by the server
- **Key mismatch**: returns an error — `"WARNING: remote public key has changed — possible MITM attack"`

The pin store is loaded into memory on first use and checked without disk I/O on subsequent verifications.

## What the Server Sees

| Server can see | Server cannot see |
|---|---|
| Sender and recipient IDs | Message content (plaintext) |
| Timestamps (sent, delivered, read) | Private keys |
| Route and relay hops | Mnemonic / recovery phrase |
| Shipping tier | |
| Home city | |
| Public keys | |

The server stores and relays `encrypted_body` as an opaque blob. It has no mechanism to decrypt messages because it never possesses any private key.

## What the Server Cannot Do

- **Read message content** — the server only sees the encrypted blob
- **Forge messages** — would require the sender's ed25519 private key to produce a valid NaCl box
- **Silently swap keys** — TOFU pinning on the client detects any change to a contact's public key

## Known Limitations

These are honest trade-offs, not bugs:

- **No perfect forward secrecy** — keypairs are static, derived from the mnemonic. Compromising a private key compromises all past and future messages encrypted with that key.
- **Metadata is visible** — the server sees who talks to whom, when, from where, and which shipping tier they chose. Only message *content* is encrypted.
- **TOFU trust boundary** — the first key exchange must be authentic. If the server substitutes a key on first contact, the client has no way to detect it. Subsequent swaps are detected.
- **No key rotation** — since keys are deterministically derived from the mnemonic, there is no mechanism to rotate keys without creating a new account.
- **Single point of failure** — the 12-word mnemonic is the only secret. If it is compromised, the attacker can derive the same keypair and impersonate the user. If it is lost, the account is unrecoverable.

## Account Recovery

The 12-word mnemonic deterministically regenerates the exact same ed25519 keypair. To recover an account on a new device:

1. Enter the mnemonic during registration
2. The client derives the same keypair and registers with the same public key
3. The server recognizes the public key and restores the account

There is no server-side recovery mechanism. If you lose the mnemonic, the account is gone.
