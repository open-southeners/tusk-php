# Tusk PHP

[![CI](https://github.com/open-southeners/php-lsp/actions/workflows/test.yml/badge.svg)](https://github.com/open-southeners/php-lsp/actions/workflows/test.yml)
[![Release](https://github.com/open-southeners/php-lsp/actions/workflows/release.yml/badge.svg)](https://github.com/open-southeners/php-lsp/actions/workflows/release.yml)
[![VS Code Marketplace](https://img.shields.io/visual-studio-marketplace/v/open-southeners.tusk-php?label=VS%20Code%20Marketplace)](https://marketplace.visualstudio.com/items?itemName=open-southeners.tusk-php)
[![Open VSX](https://img.shields.io/open-vsx/v/open-southeners/tusk-php?label=Open%20VSX)](https://open-vsx.org/extension/open-southeners/tusk-php)
[![GitHub Release](https://img.shields.io/github/v/release/open-southeners/php-lsp)](https://github.com/open-southeners/php-lsp/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
![PHP 8.0-8.5](https://img.shields.io/badge/PHP-8.0--8.5-777BB4?logo=php&logoColor=white)

A high-performance Language Server for PHP written in Go, with deep understanding of **Laravel** and **Symfony** dependency injection.

## Intelligent Completions

Context-aware suggestions that understand your code. Method chains resolve through return types and `@return` docblocks, so `Category::query()->with('tags')->where()->` shows the right `Builder` methods, not random matches from vendor.

- **Member access** (`->`, `::`, `?->`) with full chain resolution
- **Auto-import** — selecting a class automatically adds the `use` statement
- **Smart parens** — skips adding `()` when they already exist
- **Container bindings** — `app('request')->` completes with `Request` methods
- **Config keys** — `config('database.')` navigates with dot-notation
- **Array shapes** — `$config['']` completes keys from `@var` annotations and literal arrays
- **`new`, `use`, type hints, attributes, pipe operator `|>`**

## Hover & Navigation

- **Hover** — type signatures, docblock summaries, inheritance info, container bindings
- **Go to Definition** — jump to classes, methods, properties, functions, container bindings
- **Find References** — all usages across the workspace, including member access chains
- **Document Symbols** — full file outline with classes, methods, properties, constants
- **Signature Help** — parameter hints while typing function calls
- **Rename** — rename variables (scoped), classes, methods, properties across the workspace

## Refactoring

- **Copy Namespace** — copy the fully qualified name of the current file's class
- **Move to Namespace** — move a class to a different namespace, updating all references and the file path per PSR-4

## Diagnostics

- **Structural checks** — missing methods, undefined types, deprecated usage
- **PHPStan integration** — runs on save, shows errors inline (requires PHPStan in your project)
- **Laravel Pint** — formatting diagnostics on save (requires Pint in your project)

## Framework Intelligence

### Laravel

Scans service providers for `bind()` / `singleton()` calls and pre-loads 25+ core framework bindings. Completions and go-to-definition work through:

- `app('request')`, `resolve(Cache::class)`, `$this->app->make(...)`
- `config()` with dot-notation key navigation and value preview
- Eloquent method chains: `Model::query()->with()->where()->get()`
- Interface-to-concrete resolution from container bindings

### Symfony

Parses `services.yaml`, PHP service configs, and autowiring attributes:

- `#[Autowire]`, `#[AsController]`, `#[AsCommand]`, `#[AsEventListener]`, `#[AsMessageHandler]`
- Auto-wired services from `src/`
- `$container->get(...)` with registered service completions
- Interface-to-concrete resolution from `implements` declarations

### PHP 8.0 - 8.5

Full support for modern PHP syntax: union/intersection/DNF types, enums, fibers, readonly classes, match expressions, named arguments, attributes, property hooks, asymmetric visibility, typed constants, pipe operator `|>`.

## Installation

### VS Code / VS Codium

Install from the [VS Code Marketplace](https://marketplace.visualstudio.com/items?itemName=open-southeners.tusk-php) or [Open VSX Registry](https://open-vsx.org/extension/open-southeners/tusk-php):

```
ext install open-southeners.tusk-php
```

Or search for **"Tusk PHP"** in the Extensions panel. The extension bundles the language server binary — no additional setup required.

### Zed

Search for **"Tusk PHP"** in `zed: extensions`. The extension automatically downloads the correct binary for your platform.

### Neovim

Install the binary (see below), then add to your config:

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

### Other Editors

Any LSP client works. The server uses **stdio** with JSON-RPC 2.0:

```bash
php-lsp --transport stdio
```

### Binary Installation

**Download** from [GitHub Releases](https://github.com/open-southeners/php-lsp/releases/latest) (Linux, macOS, Windows — amd64/arm64).

**Quick install** (Linux / macOS):

```bash
curl -fsSL https://raw.githubusercontent.com/open-southeners/php-lsp/main/scripts/install.sh | bash
```

**From source** (requires Go 1.22+):

```bash
git clone https://github.com/open-southeners/php-lsp.git && cd php-lsp && make install
```

## Configuration

### Editor Settings (VS Code)

| Setting | Default | Description |
|---------|---------|-------------|
| `phpLsp.enable` | `true` | Enable/disable the extension |
| `phpLsp.executablePath` | `""` | Custom path to the binary |
| `phpLsp.phpVersion` | `"8.5"` | Target PHP version (`8.0`-`8.5`) |
| `phpLsp.framework` | `"auto"` | Framework: `auto`, `laravel`, `symfony`, `none` |
| `phpLsp.containerAware` | `true` | Enable DI container analysis |
| `phpLsp.diagnostics.enable` | `true` | Enable diagnostics |
| `phpLsp.diagnostics.phpstan.enable` | `true` | Run PHPStan on save |
| `phpLsp.diagnostics.pint.enable` | `true` | Run Laravel Pint on save |
| `phpLsp.maxIndexFiles` | `10000` | Maximum PHP files to index |
| `phpLsp.excludePaths` | `["vendor", ...]` | Paths to skip when indexing |

### Project Configuration

Create `.php-lsp.json` in your project root to override settings per project:

```json
{
  "phpVersion": "8.5",
  "framework": "auto",
  "containerAware": true,
  "diagnosticsEnabled": true,
  "excludePaths": ["vendor", "node_modules", ".git"],
  "maxIndexFiles": 10000
}
```

Framework is auto-detected from `artisan` (Laravel), `bin/console` (Symfony), or `composer.json`.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

[MIT](LICENSE)
