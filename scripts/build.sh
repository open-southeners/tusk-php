#!/usr/bin/env bash
set -euo pipefail
VERSION="${VERSION:-0.1.0}"
OUTPUT_DIR="./build"
rm -rf "${OUTPUT_DIR}" && mkdir -p "${OUTPUT_DIR}"
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
    IFS='/' read -r GOOS GOARCH <<< "$target"
    out="${OUTPUT_DIR}/php-lsp-${GOOS}-${GOARCH}"
    [ "$GOOS" = "windows" ] && out="${out}.exe"
    echo "Building ${GOOS}/${GOARCH}..."
    GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${out}" ./cmd/php-lsp/
done
go build -ldflags="-s -w -X main.version=${VERSION}" -trimpath -o "${OUTPUT_DIR}/php-lsp" ./cmd/php-lsp/
echo "Build complete. Binaries in: ${OUTPUT_DIR}/"
ls -lh "${OUTPUT_DIR}/"
