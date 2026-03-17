#!/usr/bin/env bash
set -euo pipefail

# One-time VPS bootstrap for Penpal on Ubuntu 24.04
# Run as root: bash setup.sh

if [ "$(id -u)" -ne 0 ]; then
    echo "Error: run as root"
    exit 1
fi

echo "==> Creating penpal system user"
if ! id penpal &>/dev/null; then
    useradd --system --shell /usr/sbin/nologin --home-dir /opt/penpal penpal
fi

echo "==> Installing PostgreSQL 18"
apt-get update -qq
apt-get install -y -qq curl ca-certificates
install -d /usr/share/postgresql-common/pgdg
curl -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc --fail https://www.postgresql.org/media/keys/ACCC4CF8.asc
echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" > /etc/apt/sources.list.d/pgdg.list
apt-get update -qq
apt-get install -y -qq postgresql-18

echo "==> Creating Postgres role and database"
sudo -u postgres psql -tc "SELECT 1 FROM pg_roles WHERE rolname='penpal'" | grep -q 1 || \
    sudo -u postgres createuser penpal
sudo -u postgres psql -tc "SELECT 1 FROM pg_database WHERE datname='penpal'" | grep -q 1 || \
    sudo -u postgres createdb --owner=penpal penpal

echo "==> Installing Caddy"
apt-get install -y -qq debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | tee /etc/apt/sources.list.d/caddy-stable.list
apt-get update -qq
apt-get install -y -qq caddy

echo "==> Setting up /opt/penpal"
mkdir -p /opt/penpal/backups
chown -R penpal:penpal /opt/penpal

echo "==> Installing systemd unit"
cp "$(dirname "$0")/penpal.service" /etc/systemd/system/penpal.service
systemctl daemon-reload
systemctl enable penpal

echo "==> Installing Caddyfile"
mkdir -p /var/log/caddy
cp "$(dirname "$0")/Caddyfile" /etc/caddy/Caddyfile
systemctl restart caddy

echo "==> Configuring UFW"
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable

echo "==> Setting up daily Postgres backup cron"
cat > /etc/cron.daily/penpal-backup << 'CRON'
#!/bin/bash
sudo -u penpal pg_dump penpal | gzip > /opt/penpal/backups/penpal-$(date +%Y%m%d).sql.gz
find /opt/penpal/backups -name "*.sql.gz" -mtime +7 -delete
CRON
chmod +x /etc/cron.daily/penpal-backup

echo ""
echo "Setup complete. Next steps:"
echo "  1. Run deploy.sh to install the server binary and graph.json"
echo "  2. Verify: sudo systemctl status penpal"
echo "  3. Verify: curl https://getpenpal.dev/v1/health"
