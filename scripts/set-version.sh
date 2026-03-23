#!/usr/bin/env bash
set -euo pipefail

usage() {
    cat <<'EOF'
Usage: scripts/set-version.sh <version> [--skip-zed-lock]

Updates the project-owned release version fields across the repository.

Examples:
  scripts/set-version.sh 0.2.1
  scripts/set-version.sh 0.3.0-beta.1 --skip-zed-lock
EOF
}

if [[ $# -lt 1 ]]; then
    usage
    exit 1
fi

VERSION=""
SKIP_ZED_LOCK=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-zed-lock)
            SKIP_ZED_LOCK=1
            shift
            ;;
        -h|--help)
            usage
            exit 0
            ;;
        *)
            if [[ -n "$VERSION" ]]; then
                echo "Unexpected argument: $1" >&2
                usage
                exit 1
            fi
            VERSION="$1"
            shift
            ;;
    esac
done

if [[ -z "$VERSION" ]]; then
    echo "Missing version argument." >&2
    usage
    exit 1
fi

if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.-]+)?$ ]]; then
    echo "Invalid version: $VERSION" >&2
    echo "Expected semantic version like 0.2.1 or 0.3.0-beta.1" >&2
    exit 1
fi

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export VERSION

replace_in_file() {
    local file="$1"
    local expression="$2"
    perl -0pi -e "$expression" "$file"
}

echo "Setting version to $VERSION"

replace_in_file "Makefile" 's/^VERSION \?= .+$/VERSION ?= $ENV{VERSION}/m'
replace_in_file "scripts/build.sh" 's/^VERSION="\$\{VERSION:-[^}]+\}"$/VERSION="\${VERSION:-$ENV{VERSION}}"/m'
replace_in_file "scripts/install.sh" 's/^VERSION="\$\{VERSION:-[^}]+\}"$/VERSION="\${VERSION:-$ENV{VERSION}}"/m'
replace_in_file "cmd/tusk-php/main.go" 's/^(\s*version\s*=\s*")[^"]+(")/$1$ENV{VERSION}$2/m'
replace_in_file "internal/lsp/server.go" 's/^(const ServerVersion = ")[^"]+(")/$1$ENV{VERSION}$2/m'
replace_in_file "editors/vscode/package.json" 's/^(\s*"version": ")[^"]+(")/$1$ENV{VERSION}$2/m'
replace_in_file "editors/zed/Cargo.toml" 's/(\[package\]\n(?:[^\n]*\n)*?version = ")[^"]+(")/$1$ENV{VERSION}$2/s'
replace_in_file "editors/zed/extension.toml" 's/\A(id = "[^"]+"\nname = "[^"]+"\nversion = ")[^"]+(")/$1$ENV{VERSION}$2/s'
replace_in_file "CONTRIBUTING.md" 's/(Pushing a semver tag \(e\.g\., `v)[^`]+(`\) triggers:)/$1$ENV{VERSION}$2/'

if [[ $SKIP_ZED_LOCK -eq 0 ]]; then
    if ! command -v cargo >/dev/null 2>&1; then
        echo "cargo is required to refresh editors/zed/Cargo.lock. Re-run with --skip-zed-lock to skip this step." >&2
        exit 1
    fi

    echo "Refreshing editors/zed/Cargo.lock"
    cargo generate-lockfile --manifest-path editors/zed/Cargo.toml
    cargo metadata --format-version 1 --locked --manifest-path editors/zed/Cargo.toml >/dev/null
fi

echo "Updated version references:"
printf '  %s\n' \
    "Makefile" \
    "scripts/build.sh" \
    "scripts/install.sh" \
    "cmd/tusk-php/main.go" \
    "internal/lsp/server.go" \
    "editors/vscode/package.json" \
    "editors/zed/Cargo.toml" \
    "editors/zed/extension.toml" \
    "CONTRIBUTING.md"

if [[ $SKIP_ZED_LOCK -eq 0 ]]; then
    echo "Validated editors/zed/Cargo.lock with cargo metadata"
else
    echo "Skipped editors/zed/Cargo.lock refresh"
fi
