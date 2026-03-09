.PHONY: build install test clean dev

VERSION ?= 0.1.0
BINARY  := php-lsp
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

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
	cd editors/vscode && npm install && npm run compile
