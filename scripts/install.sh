#!/usr/bin/env bash
set -euo pipefail
VERSION="${VERSION:-0.4.0}"
INSTALL_DIR="${HOME}/.local/bin"
mkdir -p "${INSTALL_DIR}"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH="amd64";; aarch64|arm64) ARCH="arm64";; *) echo "Unsupported: $ARCH"; exit 1;; esac
if command -v go &> /dev/null; then
    echo "Building from source..."
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    cd "$(dirname "$SCRIPT_DIR")"
    go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${INSTALL_DIR}/php-lsp" ./cmd/tusk-php/
else
    echo "Downloading binary..."
    curl -fsSL -o "${INSTALL_DIR}/php-lsp" "https://github.com/open-southeners/tusk-php/releases/download/v${VERSION}/tusk-php-${OS}-${ARCH}"
    chmod +x "${INSTALL_DIR}/php-lsp"
fi
echo "Installed to ${INSTALL_DIR}/php-lsp"
[[ ":$PATH:" != *":${INSTALL_DIR}:"* ]] && echo "Add to PATH: export PATH=\"\${HOME}/.local/bin:\${PATH}\""
