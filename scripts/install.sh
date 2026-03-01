#!/usr/bin/env bash
# Discard — Ubuntu 22.04 install script
# Installs dependencies, builds from source, sets up systemd service.
# Idempotent: safe to re-run.
set -euo pipefail

# ─── Colors ──────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

info()  { printf "${CYAN}[info]${NC}  %s\n" "$*"; }
ok()    { printf "${GREEN}[ok]${NC}    %s\n" "$*"; }
warn()  { printf "${YELLOW}[warn]${NC}  %s\n" "$*"; }
fail()  { printf "${RED}[error]${NC} %s\n" "$*"; exit 1; }

# ─── Pre-flight ──────────────────────────────────────────────────────────────

[[ $EUID -eq 0 ]] || fail "This script must be run as root (sudo)."

if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    if [[ "${ID:-}" != "ubuntu" ]]; then
        warn "This script is designed for Ubuntu. Detected: ${PRETTY_NAME:-unknown}."
        warn "Proceeding anyway — some commands may differ."
    fi
else
    warn "Cannot detect OS. Proceeding anyway."
fi

INSTALL_DIR="/opt/discard"
UPLOAD_DIR="/var/discard/uploads"
DB_USER="discard"
DB_NAME="discard"
DB_PASS="${DISCARD_DB_PASS:-$(openssl rand -hex 16)}"
SERVICE_USER="discard"
PORT="${DISCARD_PORT:-4000}"

# ─── System packages ────────────────────────────────────────────────────────

info "Updating package lists..."
apt-get update -qq

info "Installing system dependencies..."
apt-get install -y -qq \
    postgresql \
    webp \
    ffmpeg \
    curl \
    git \
    > /dev/null

ok "System packages installed."

# ─── yt-dlp ──────────────────────────────────────────────────────────────────

if command -v yt-dlp &>/dev/null; then
    ok "yt-dlp already installed."
else
    info "Installing yt-dlp..."
    curl -sL https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
        -o /usr/local/bin/yt-dlp
    chmod a+rx /usr/local/bin/yt-dlp
    ok "yt-dlp installed."
fi

# ─── Node.js (LTS via NodeSource) ───────────────────────────────────────────

if command -v node &>/dev/null; then
    ok "Node.js already installed: $(node --version)"
else
    info "Installing Node.js 22 LTS..."
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
    apt-get install -y -qq nodejs > /dev/null
    ok "Node.js installed: $(node --version)"
fi

# ─── Go ──────────────────────────────────────────────────────────────────────

GO_VERSION="1.23.4"

if command -v go &>/dev/null && go version | grep -q "go${GO_VERSION}"; then
    ok "Go ${GO_VERSION} already installed."
else
    info "Installing Go ${GO_VERSION}..."
    curl -sL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf /tmp/go.tar.gz
    rm /tmp/go.tar.gz

    # Ensure Go is on PATH for this script and future logins
    if ! grep -q '/usr/local/go/bin' /etc/profile.d/go.sh 2>/dev/null; then
        echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
    fi
    export PATH=$PATH:/usr/local/go/bin
    ok "Go installed: $(go version)"
fi

# ─── PostgreSQL ──────────────────────────────────────────────────────────────

info "Configuring PostgreSQL..."
systemctl enable --now postgresql > /dev/null 2>&1

# Create user and database (idempotent)
if sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" | grep -q 1; then
    ok "Postgres user '${DB_USER}' already exists."
else
    sudo -u postgres psql -c "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';" > /dev/null
    ok "Postgres user '${DB_USER}' created."
fi

if sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" | grep -q 1; then
    ok "Database '${DB_NAME}' already exists."
else
    sudo -u postgres createdb "${DB_NAME}" -O "${DB_USER}"
    ok "Database '${DB_NAME}' created."
fi

# ─── System user ─────────────────────────────────────────────────────────────

if id "${SERVICE_USER}" &>/dev/null; then
    ok "System user '${SERVICE_USER}' already exists."
else
    useradd --system --shell /usr/sbin/nologin --home-dir "${INSTALL_DIR}" "${SERVICE_USER}"
    ok "System user '${SERVICE_USER}' created."
fi

# ─── Build from source ──────────────────────────────────────────────────────

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ ! -f "${REPO_DIR}/go.mod" ]]; then
    fail "Cannot find go.mod. Run this script from the Discard repo: scripts/install.sh"
fi

info "Building frontend..."
cd "${REPO_DIR}/web"
npm ci --silent
npm run build
ok "Frontend built."

info "Building Go binary..."
cd "${REPO_DIR}"
export PATH=$PATH:/usr/local/go/bin
CGO_ENABLED=0 go build -o discard ./cmd/discard
ok "Binary built: ${REPO_DIR}/discard"

# ─── Install ─────────────────────────────────────────────────────────────────

info "Installing to ${INSTALL_DIR}..."
mkdir -p "${INSTALL_DIR}"
cp "${REPO_DIR}/discard" "${INSTALL_DIR}/discard"
chmod +x "${INSTALL_DIR}/discard"

mkdir -p "${UPLOAD_DIR}"
chown -R "${SERVICE_USER}:${SERVICE_USER}" "${INSTALL_DIR}" "${UPLOAD_DIR}"
ok "Installed."

# ─── Environment file ───────────────────────────────────────────────────────

ENV_FILE="${INSTALL_DIR}/discard.env"

if [[ -f "${ENV_FILE}" ]]; then
    warn "Environment file already exists: ${ENV_FILE} (not overwriting)."
else
    cat > "${ENV_FILE}" <<EOF
DATABASE_URL=postgres://${DB_USER}:${DB_PASS}@localhost:5432/${DB_NAME}?sslmode=disable
UPLOAD_DIR=${UPLOAD_DIR}
PORT=${PORT}
EOF
    chown "${SERVICE_USER}:${SERVICE_USER}" "${ENV_FILE}"
    chmod 600 "${ENV_FILE}"
    ok "Environment file written: ${ENV_FILE}"
fi

# ─── systemd service ────────────────────────────────────────────────────────

SERVICE_FILE="/etc/systemd/system/discard.service"

info "Writing systemd service..."
cat > "${SERVICE_FILE}" <<EOF
[Unit]
Description=Discard Chat Server
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=${SERVICE_USER}
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${ENV_FILE}
ExecStart=${INSTALL_DIR}/discard
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable discard > /dev/null 2>&1
ok "systemd service installed and enabled."

# ─── Done ────────────────────────────────────────────────────────────────────

echo ""
printf "${GREEN}════════════════════════════════════════════════════${NC}\n"
printf "${GREEN}  Discard installed successfully!${NC}\n"
printf "${GREEN}════════════════════════════════════════════════════${NC}\n"
echo ""
info "Start the service:    sudo systemctl start discard"
info "View logs:            sudo journalctl -u discard -f"
info "Config:               ${ENV_FILE}"
info "Uploads:              ${UPLOAD_DIR}"
echo ""
warn "Next steps:"
echo "  1. Edit ${ENV_FILE} if you need to change the database password or port."
echo "  2. Install Tailscale if not already installed:"
echo "       curl -fsSL https://tailscale.com/install.sh | sh"
echo "       sudo tailscale up"
echo "  3. Share your server node with friends via the Tailscale admin console."
echo "  4. Access Discard at http://<tailscale-ip>:${PORT}"
echo ""
echo "  For Docker deployment instead, see: docker compose up -d"
echo ""
