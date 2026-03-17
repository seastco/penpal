# penpal

<!-- TODO: add demo GIF -->

penpal is a terminal messaging app where your letters travel across the United States. Send a letter from Chicago to a friend in Portland and it hops through real cities, taking real time to arrive (days, not milliseconds). In a world of instant everything, penpal brings back the anticipation of waiting for mail.

## Highlights

- **Real transit** — letters travel between ~30,000 US cities at realistic speeds
- **Live tracking** — watch your letter hop from city to city
- **Real delivery times** — letters travel at First Class speed, arriving in days not milliseconds
- **Stamp collecting** — earn common, state, and rare stamps as you send and receive
- **End-to-end encrypted** — the server never sees your messages ([how it works](SECURITY.md))
- **12-word recovery phrase** — your account lives in a mnemonic, recoverable on any device
- **Terminal-native** — a full TUI built with [Bubbletea](https://github.com/charmbracelet/bubbletea)

## Getting started

Install
```bash
curl -fsSL https://raw.githubusercontent.com/seastco/penpal/master/install.sh | sh
```
Run
```
penpal
```

On first launch you'll choose a username, write down your 12-word recovery phrase, and pick a home city. That's it. You're ready to start sending letters.

## Account dir

Your account data lives in `~/.penpal` by default. To use a different directory:

```bash
PENPAL_HOME=~/somewhere/else penpal
```

**Important:** Your private key in `~/.penpal` *is* your account. If you delete that folder or lose the key, the only way to recover your account is with your 12-word recovery phrase. Keep it safe in a password manager.
