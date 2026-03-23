.PHONY: build install test clean dev cross-build vscode-ext vscode-package zed-ext zed-package

VERSION ?= 0.2.0
BINARY  := php-lsp
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"
DIST_DIR := dist
ZED_DIR := editors/zed
ZED_TARGET := $(ZED_DIR)/target/wasm32-wasip1/release/tusk_php_zed.wasm
ZED_PACKAGE_DIR := $(DIST_DIR)/tusk-php-zed-$(VERSION)
ZED_PACKAGE_ZIP := $(DIST_DIR)/tusk-php-zed-$(VERSION).zip

build:
	go build $(LDFLAGS) -trimpath -o build/$(BINARY) ./cmd/php-lsp/

install: build
	cp build/$(BINARY) $(HOME)/.local/bin/$(BINARY)

dev:
	go run ./cmd/php-lsp/ --log /tmp/php-lsp.log

test:
	go test -v -race ./...

clean:
	rm -rf build/

cross-build:
	bash scripts/build.sh

vscode-ext:
	cd editors/vscode && bun install && bun run compile

vscode-package: cross-build vscode-ext
	cd editors/vscode && bunx @vscode/vsce package

zed-ext:
	cargo build --manifest-path $(ZED_DIR)/Cargo.toml --target wasm32-wasip1 --release

zed-package: zed-ext
	rm -rf $(ZED_PACKAGE_DIR) $(ZED_PACKAGE_ZIP)
	mkdir -p $(ZED_PACKAGE_DIR)
	cp $(ZED_DIR)/extension.toml $(ZED_PACKAGE_DIR)/
	cp $(ZED_DIR)/Cargo.toml $(ZED_PACKAGE_DIR)/
	cp $(ZED_DIR)/Cargo.lock $(ZED_PACKAGE_DIR)/
	cp $(ZED_TARGET) $(ZED_PACKAGE_DIR)/extension.wasm
	cd $(DIST_DIR) && zip -r $(notdir $(ZED_PACKAGE_ZIP)) $(notdir $(ZED_PACKAGE_DIR))
