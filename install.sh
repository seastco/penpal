#!/bin/sh
# Install script for penpal — https://github.com/seastco/penpal
# Usage: curl -fsSL https://raw.githubusercontent.com/seastco/penpal/master/install.sh | sh
set -eu

REPO="seastco/penpal"
INSTALL_DIR="/usr/local/bin"
FALLBACK_DIR="${HOME:?HOME must be set}/.local/bin"

info() { printf '  %s\n' "$@"; }
warn() { printf '  Warning: %s\n' "$@" >&2; }
error() { printf '  Error: %s\n' "$@" >&2; exit 1; }

need_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        error "required command not found: $1"
    fi
}

download() {
    url="$1"
    output="$2"
    if command -v curl >/dev/null 2>&1; then
        curl --proto =https --tlsv1.2 -fsSL "$url" -o "$output"
    elif command -v wget >/dev/null 2>&1; then
        wget --secure-protocol=TLSv1_2 -qO "$output" "$url"
    else
        error "curl or wget is required"
    fi
}

main() {
    # Pre-flight checks
    need_cmd uname
    need_cmd tar
    if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        error "curl or wget is required"
    fi

    # Detect OS
    os="$(uname -s)"
    case "$os" in
        Darwin) os="darwin" ;;
        Linux)  os="linux" ;;
        *)      error "unsupported OS: $os" ;;
    esac

    # Detect architecture
    arch="$(uname -m)"
    case "$arch" in
        x86_64|amd64)  arch="amd64" ;;
        arm64|aarch64) arch="arm64" ;;
        *)             error "unsupported architecture: $arch" ;;
    esac

    # Rosetta detection: uname -m reports x86_64 on arm64 Macs running Rosetta
    if [ "$os" = "darwin" ] && [ "$arch" = "amd64" ]; then
        if sysctl hw.optional.arm64 2>/dev/null | grep -q ': 1'; then
            arch="arm64"
            info "Detected Apple Silicon (running under Rosetta)"
        fi
    fi

    artifact="penpal-${os}-${arch}.tar.gz"
    url="https://github.com/${REPO}/releases/latest/download/${artifact}"

    info "Detected platform: ${os}/${arch}"
    info "Downloading ${artifact}..."

    # Create temp directory and ensure cleanup
    tmp_dir="$(mktemp -d)" || error "failed to create temp directory"
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download
    download "$url" "${tmp_dir}/${artifact}" || error "download failed — check that a release exists for ${os}/${arch}"

    # Extract
    tar -xzf "${tmp_dir}/${artifact}" -C "$tmp_dir" || error "failed to extract archive"

    # Verify binary exists in archive
    if [ ! -f "${tmp_dir}/penpal" ]; then
        error "binary not found in archive"
    fi
    chmod +x "${tmp_dir}/penpal"

    # Install
    if [ -d "$INSTALL_DIR" ] && [ -w "$INSTALL_DIR" ]; then
        cp "${tmp_dir}/penpal" "${INSTALL_DIR}/penpal"
        info "Installed to ${INSTALL_DIR}/penpal"
    elif command -v sudo >/dev/null 2>&1; then
        info "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mkdir -p "$INSTALL_DIR"
        sudo cp "${tmp_dir}/penpal" "${INSTALL_DIR}/penpal"
        info "Installed to ${INSTALL_DIR}/penpal"
    else
        mkdir -p "$FALLBACK_DIR"
        cp "${tmp_dir}/penpal" "${FALLBACK_DIR}/penpal"
        info "Installed to ${FALLBACK_DIR}/penpal"
        case ":${PATH}:" in
            *":${FALLBACK_DIR}:"*) ;;
            *) warn "${FALLBACK_DIR} is not in your PATH. Add it with:"
               info "  export PATH=\"${FALLBACK_DIR}:\$PATH\""
               ;;
        esac
    fi

    info ""
    info "penpal installed successfully! Run 'penpal' to get started."
}

main "$@"
