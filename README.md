# penpal

A terminal messaging app where letters take real time to travel between US cities.

<!-- TODO: add demo video -->

## Features

- **Real-time transit** with tracking as letters hop between cities
- **Three shipping tiers** from days to hours
- **End-to-end encrypted** so the server never sees your messages ([details](SECURITY.md))
- **Stamp collecting** with common, state, and rare stamps
- **Account recovery** from a 12-word phrase on any device

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/seastco/penpal/master/install.sh | sh
```

Then run:

```
penpal
```

On first launch you'll register: choose a username, write down your 12-word recovery phrase, and pick a home city.

## Multiple Accounts

Each account lives in `~/.penpal`. To run a second account:

```bash
PENPAL_HOME=~/.penpal-alt penpal
```
