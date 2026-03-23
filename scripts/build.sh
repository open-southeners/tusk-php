#!/usr/bin/env bash
set -euo pipefail
VERSION="${VERSION:-0.3.0}"
OUTPUT_DIR="./build"
VSCODE_BIN_DIR="./editors/vscode/bin"
rm -rf "${OUTPUT_DIR}" && mkdir -p "${OUTPUT_DIR}"
rm -rf "${VSCODE_BIN_DIR}"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
    IFS='/' read -r GOOS GOARCH <<< "$target"
    ext=""; [ "$GOOS" = "windows" ] && ext=".exe"
    out="${OUTPUT_DIR}/tusk-php-${GOOS}-${GOARCH}${ext}"
    echo "Building ${GOOS}/${GOARCH}..."
    GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${out}" ./cmd/tusk-php/
    # Bundle into VSCode extension
    vscode_dir="${VSCODE_BIN_DIR}/${GOOS}-${GOARCH}"
    mkdir -p "${vscode_dir}"
    cp "${out}" "${vscode_dir}/php-lsp${ext}"
done
echo "Build complete. Binaries in: ${OUTPUT_DIR}/"
ls -lh "${OUTPUT_DIR}/"
echo "VSCode binaries bundled in: ${VSCODE_BIN_DIR}/"
ls -R "${VSCODE_BIN_DIR}/"
