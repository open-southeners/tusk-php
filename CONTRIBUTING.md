# Contributing to PHP LSP

Thank you for your interest in contributing! This document covers everything you need to get started.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Getting Started

### Prerequisites

- **Go 1.22+** — for the language server
- **Node.js 20+** — for the VS Code extension
- **Rust (stable)** with `wasm32-wasip1` target — for the Zed extension (optional)

### Development Setup

```bash
git clone https://github.com/open-southeners/tusk-php.git
cd tusk-php

# Build the server
make build

# Run tests
make test

# Run locally with debug logging
make dev
```

### Project Structure

```
cmd/tusk-php/      Entry point
internal/          All server packages (parser, symbols, hover, completion, etc.)
editors/vscode/    VS Code extension (TypeScript)
editors/zed/       Zed extension (Rust/WASM)
testdata/          Test fixtures (mock PHP project)
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed documentation of internals.

## Development Workflow

### Running Tests

```bash
# All tests with race detection
make test

# Single test
go test -v -race -run TestHoverMethodChain ./internal/hover/

# Specific package
go test -v -race ./internal/symbols/
```

### Building Editor Extensions

```bash
# VS Code
make vscode-ext

# VS Code .vsix package (includes cross-compiled binaries)
make vscode-package
```

### Testing with an Editor

1. Build the binary: `make build`
2. Point your editor to `build/php-lsp`
3. For VS Code: set `phpLsp.executablePath` to the absolute path of `build/php-lsp`
4. Use `make dev` to run with logging to `/tmp/php-lsp.log`

## Making Changes

### Before You Start

- Check [existing issues](https://github.com/open-southeners/tusk-php/issues) to avoid duplicate work
- For larger changes, open an issue first to discuss the approach

### Pull Request Process

1. Fork the repository and create a feature branch from `main`
2. Write tests for new functionality
3. Ensure `make test` passes with no failures
4. Keep commits focused — one logical change per commit
5. Open a pull request against `main`

### Code Style

- Go: standard `gofmt` formatting (enforced by CI)
- TypeScript: project `tsconfig.json` settings
- No external Go dependencies — the server has zero `require` directives in `go.mod` by design

### Test Fixtures

Tests use `testdata/project/` which contains a mock PHP project with `composer.json`, source files, and vendor stubs. When adding test cases:

- Add PHP fixtures to `testdata/project/src/` or `testdata/project/vendor/`
- Index fixtures in your test setup and assert against hover content, symbol resolution, etc.

## Reporting Bugs

- Use the [GitHub issue tracker](https://github.com/open-southeners/tusk-php/issues)
- Include your editor, OS, PHP version, and steps to reproduce
- Attach the server log (`--log /tmp/php-lsp.log`) if relevant

## Feature Requests

Open an issue with the **feature request** label. Describe the use case and expected behavior.

## Release Process

Releases are automated via GitHub Actions. Pushing a semver tag (e.g., `v0.3.0`) triggers:

1. Full test suite
2. Cross-platform binary builds
3. VS Code extension packaging and publishing (Marketplace + Open VSX)
4. Zed extension build
5. GitHub Release with all artifacts and changelog notes

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
