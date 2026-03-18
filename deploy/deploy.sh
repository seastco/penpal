#!/usr/bin/env bash
set -euo pipefail

# Deploy latest Penpal release (or specified version)
# Run as root: bash deploy.sh [version] [--force-graph]
#
# Examples:
#   bash deploy.sh              # latest release
#   bash deploy.sh v0.0.6       # specific version
#   bash deploy.sh --force-graph  # latest + re-download graph.json

REPO="seastco/penpal"
INSTALL_DIR="/opt/penpal"
ARCH="amd64"
FORCE_GRAPH=false
VERSION=""

for arg in "$@"; do
    case "$arg" in
        --force-graph) FORCE_GRAPH=true ;;
        v*) VERSION="$arg" ;;
    esac
done

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: run as root"
    exit 1
fi

# Resolve latest version if not specified
if [ -z "$VERSION" ]; then
    VERSION=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest release"
        exit 1
    fi
fi

echo "==> Deploying Penpal ${VERSION}"

# Pre-deploy DB backup
echo "==> Pre-deploy database backup"
BACKUP_FILE="${INSTALL_DIR}/backups/pre-deploy-${VERSION}-$(date +%Y%m%d%H%M%S).sql.gz"
sudo -u penpal pg_dump penpal | gzip > "$BACKUP_FILE" || echo "WARNING: backup failed, continuing deploy"

# Save previous binary for rollback
if [ -f "${INSTALL_DIR}/penpal-server" ]; then
    cp "${INSTALL_DIR}/penpal-server" "${INSTALL_DIR}/penpal-server.prev"
    echo "==> Saved previous binary as penpal-server.prev"
fi

ARCHIVE="penpal-server-linux-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE}"

echo "==> Downloading ${ARCHIVE}"
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

curl -sfL -o "${TMPDIR}/${ARCHIVE}" "$URL"
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

# Download graph.json if missing or forced
if [ ! -f "${INSTALL_DIR}/graph.json" ] || [ "$FORCE_GRAPH" = true ]; then
    echo "==> Downloading graph.json"
    GRAPH_URL="https://github.com/${REPO}/releases/download/${VERSION}/graph.json"
    curl -sfL -o "${INSTALL_DIR}/graph.json" "$GRAPH_URL"
    chown penpal:penpal "${INSTALL_DIR}/graph.json"
fi

# Stop service before overwriting binary (avoids "Text file busy" error)
echo "==> Stopping penpal service"
systemctl stop penpal

cp "${TMPDIR}/penpal-server" "${INSTALL_DIR}/penpal-server"
chown penpal:penpal "${INSTALL_DIR}/penpal-server"
chmod 755 "${INSTALL_DIR}/penpal-server"

echo "==> Starting penpal service"
systemctl start penpal

echo "==> Deploy complete"
systemctl status penpal --no-pager
