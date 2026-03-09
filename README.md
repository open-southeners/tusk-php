# PHP LSP — Go-based Language Server for PHP 8.5+

A high-performance Language Server Protocol implementation written in **Go** for PHP 8.5+, with deep understanding of **Laravel** and **Symfony** dependency injection containers.

## Features

### Language Intelligence
- **Full PHP 8.0–8.5 Tokenizer & Parser** — Union types, intersection types, DNF types, enums, fibers, readonly classes, property hooks, asymmetric visibility, pipe operator `|>`
- **Context-Aware Completion** — Member access (`->`), static access (`::`), nullsafe (`?->`), `new`, `use`, type hints, variables, PHP 8 attributes, pipe targets
- **Hover Information** — Rich markdown with type signatures, inheritance chains, docblocks
- **Go to Definition / Find References** — Jump to and find all usages of classes, methods, functions
- **Document Symbols** — Full outline with classes, interfaces, enums, methods, properties
- **Signature Help** — Parameter hints while typing function/method calls
- **Diagnostics** — Deprecated functions, structural errors, unused imports

### Container-Aware Intelligence
- **Laravel** — Resolves `app()`, `bind()`, `singleton()`, facade accessors, service providers. 25+ pre-loaded framework bindings.
- **Symfony** — Parses `services.yaml`, PHP service configs, `#[Autowire]`, `#[AsController]`, `#[AsCommand]`, `#[AsEventListener]`, `#[AsMessageHandler]`, and auto-wiring from `src/`.
- **Constructor Injection** — Hover over type-hinted params to see what the container injects.
- **Interface → Concrete Resolution** — See which implementation is bound to an interface.

### PHP 8.5 Support
- Pipe operator `|>` with callable target completion
- All PHP 8.0–8.4 features: enums, fibers, readonly, match, named arguments, attributes, property hooks, asymmetric visibility, `#[\Override]`, typed constants

## Architecture

```
cmd/php-lsp/          Entry point (stdio JSON-RPC transport)
internal/
  protocol/           LSP type definitions
  config/             Configuration + framework auto-detection
  parser/             PHP tokenizer + lightweight AST parser
  symbols/            Symbol table, index, built-in stubs
  container/          DI container analyzer (Laravel + Symfony)
  completion/         Context-aware completion provider
  hover/              Hover information provider
  diagnostics/        Real-time diagnostics
  analyzer/           Go-to-definition, references, document symbols, signature help
  lsp/                JSON-RPC server, message dispatch
editors/
  vscode/             VS Code extension (TypeScript + vscode-languageclient)
  zed/                Zed extension (Rust + zed_extension_api)
scripts/              Build + install scripts
```

## Installation

### From Source (requires Go 1.22+)

```bash
git clone https://github.com/open-southeners/php-lsp.git
cd php-lsp
make install
```

### Quick Install

```bash
curl -fsSL https://raw.githubusercontent.com/rubenrobles/php-lsp/main/scripts/install.sh | bash
```

### Cross-platform Build

```bash
make cross-build   # Builds for linux/darwin/windows × amd64/arm64
```

## Editor Setup

### VS Code

```bash
cd editors/vscode && npm install && npm run compile
# Then: Ctrl+Shift+P → "Developer: Install Extension from Location..."
```

Or set `phpLsp.executablePath` in VS Code settings to point to the `php-lsp` binary.

| Setting | Default | Description |
|---------|---------|-------------|
| `phpLsp.phpVersion` | `"8.5"` | Target PHP version |
| `phpLsp.framework` | `"auto"` | `auto`, `laravel`, `symfony`, `none` |
| `phpLsp.containerAware` | `true` | Enable DI container analysis |
| `phpLsp.diagnostics.enable` | `true` | Real-time diagnostics |
| `phpLsp.excludePaths` | `[...]` | Paths to skip when indexing |

### Zed

Ensure `php-lsp` is in your `PATH`, then install the extension from `editors/zed/`.

### Neovim (lspconfig)

```lua
require('lspconfig.configs').php_lsp = {
  default_config = {
    cmd = { 'php-lsp', '--transport', 'stdio' },
    filetypes = { 'php' },
    root_dir = require('lspconfig.util').root_pattern('composer.json', '.git'),
  }
}
require('lspconfig').php_lsp.setup({})
```

### Any LSP Client

The server communicates over **stdio** using standard JSON-RPC 2.0:

```bash
php-lsp --transport stdio
php-lsp --transport stdio --log /tmp/php-lsp.log
```

## Project Configuration

Create `.php-lsp.json` in your project root (see `.php-lsp.json.example`):

```json
{
  "phpVersion": "8.5",
  "framework": "auto",
  "excludePaths": ["vendor", "node_modules", ".git"],
  "containerAware": true,
  "diagnosticsEnabled": true,
  "maxIndexFiles": 10000
}
```

Framework is auto-detected from `artisan` (Laravel), `bin/console` (Symfony), or `composer.json` requires.

## How Container Analysis Works

### Laravel
Scans `app/Providers/*.php` for `$this->app->bind()` and `$this->app->singleton()` calls. Pre-loads 25+ core framework bindings (auth, cache, config, db, events, filesystem, queue, router, session, view, etc.).

### Symfony
Parses `config/services.yaml`, XML service definitions, and PHP config files. Auto-wires classes in `src/` and resolves interface bindings from `implements` declarations. Understands Symfony attributes like `#[AsController]`, `#[AsCommand]`, `#[Autowire]`.

## License

MIT
