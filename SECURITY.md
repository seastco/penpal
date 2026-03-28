# Security Model

penpal uses end-to-end encryption so the relay server transports messages it cannot read.

## How It Works

When you create a penpal account, you get a 12-word secret phrase. This phrase generates a unique pair of keys: a **public key** (like a mailing address anyone can see) and a **private key** (like a key to your mailbox that only you have).

When you send a letter, your app uses *your* private key and the *recipient's* public key to scramble the message into unreadable gibberish. Only the recipient can unscramble it, because only they have the matching private key. The penpal server just passes this gibberish along -- it never has the keys needed to read it.

Think of it like putting a letter in a box with two locks. You lock it with your lock, and it can only be opened with the recipient's key. The mail carrier (the server) moves the box but can never open it.

The first time you message someone, your app saves their public key locally. If the server ever tries to slip in a different key later, your app will warn you. This is called Trust-On-First-Use.

**What the server can see:** who sent a letter to whom, when, and from which city. Basically, what a postal worker could read off an envelope.

**What the server can never see:** what you actually wrote.

**The catch:** your 12-word phrase *is* your account. Anyone who knows it can read your messages and pretend to be you. If you lose it, your account is gone forever -- there is no "forgot password" flow. Write it down and keep it safe.

## Cryptographic Details

| Component | Primitive |
|---|---|
| Mnemonic | BIP39, 128-bit entropy (12 words) |
| Key derivation | PBKDF2-SHA512, 210K iterations, salt `penpal-ed25519-v1` |
| Identity keypair | ed25519 |
| Authentication | Challenge-response (server sends 32-byte nonce, client signs with ed25519) |
| Encryption | NaCl box (XSalsa20-Poly1305) |
| Key agreement | X25519 (ed25519 keys converted to Curve25519 via `filippo.io/edwards25519`) |
| Wire format | `[24-byte nonce \|\| ciphertext]` |
| Key pinning | TOFU, cached in `~/.penpal/known_keys` |

## Known Limitations

These are honest trade-offs, not bugs:

- **No perfect forward secrecy** — keypairs are static, derived from the mnemonic. Compromising a private key compromises all past and future messages encrypted with that key.
- **Metadata is visible** — the server sees who talks to whom, when, from where, and which shipping tier they chose. Only message *content* is encrypted.
- **TOFU trust boundary** — the first key exchange must be authentic. If the server substitutes a key on first contact, the client has no way to detect it. Subsequent swaps are detected.
- **No key rotation** — since keys are deterministically derived from the mnemonic, there is no mechanism to rotate keys without creating a new account.
- **Single point of failure** — the 12-word mnemonic is the only secret. If it is compromised, the attacker can derive the same keypair and impersonate the user. If it is lost, the account is unrecoverable.
