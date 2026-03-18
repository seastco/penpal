#!/usr/bin/env bash
set -euo pipefail

# Rollback to previous Penpal server binary.
# Run as root: bash rollback.sh

INSTALL_DIR="/opt/penpal"

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: run as root"
    exit 1
fi

if [ ! -f "${INSTALL_DIR}/penpal-server.prev" ]; then
    echo "Error: no previous binary found at ${INSTALL_DIR}/penpal-server.prev"
    exit 1
fi

echo "==> Rolling back to previous binary"
cp "${INSTALL_DIR}/penpal-server.prev" "${INSTALL_DIR}/penpal-server"
chown penpal:penpal "${INSTALL_DIR}/penpal-server"
chmod 755 "${INSTALL_DIR}/penpal-server"

echo "==> Restarting penpal service"
systemctl restart penpal

echo "==> Rollback complete"
systemctl status penpal --no-pager
