# Architecture

A Go-based Language Server Protocol (LSP) implementation for PHP 8.0–8.5, with Laravel and Symfony DI container awareness. No external PHP parser — includes a custom tokenizer and lightweight AST.

## Project Structure

```
php-lsp/
├── cmd/tusk-php/main.go             # Entry point: CLI flags, stdio, server startup
├── internal/
│   ├── parser/                      # PHP tokenizer + lightweight AST
│   ├── symbols/                     # Central symbol table (Index)
│   ├── lsp/                         # JSON-RPC server, message dispatch, indexing
│   ├── completion/                  # Context-aware completions
│   ├── hover/                       # Hover cards with type resolution
│   ├── analyzer/                    # Go-to-definition, references, document symbols, signature help
│   ├── diagnostics/                 # Static checks + PHPStan/Pint integration
│   ├── composer/                    # Parses composer.json autoload (PSR-4 + files)
│   ├── container/                   # Laravel/Symfony DI container analysis
│   ├── config/                      # .php-lsp.json + client options + framework detection
│   └── protocol/                    # LSP type definitions (no logic)
├── editors/
│   ├── vscode/                      # TypeScript extension
│   └── zed/                         # Rust/WASM extension
├── testdata/project/                # Test fixtures (mock PHP project)
├── scripts/build.sh                 # Cross-compilation script
└── Makefile                         # Build, test, dev targets
```

## Entry Point

`cmd/tusk-php/main.go` parses CLI flags (`--version`, `--log`, `--stdio`), creates a `lsp.Server` with stdin/stdout, and calls `server.Run()` which enters the JSON-RPC message loop.

## JSON-RPC Protocol

Communication uses stdio with `Content-Length` headers. The server reads messages in a single-threaded loop, dispatches to handlers, and sends responses/notifications.

### Supported Methods

| Method | Purpose |
|--------|---------|
| `initialize` | Handshake: load config, detect framework, create providers |
| `initialized` | Async start: workspace indexing, composer deps, container analysis |
| `textDocument/didOpen` | Store document, index, run diagnostics |
| `textDocument/didChange` | Update content, re-index, re-run diagnostics |
| `textDocument/didSave` | Re-index, trigger PHPStan/Pint |
| `textDocument/completion` | Context-aware completions at cursor |
| `textDocument/hover` | Type info and docs at cursor |
| `textDocument/definition` | Jump to symbol definition |
| `textDocument/references` | Find all references |
| `textDocument/documentSymbol` | File outline (classes, methods, properties) |
| `textDocument/signatureHelp` | Parameter hints in function calls |

## Core Data Structures

### Symbol

Represents any PHP symbol (class, method, property, function, constant, enum, etc.):

```go
type Symbol struct {
    Name       string        // Short name: "Logger"
    FQN        string        // Fully qualified: "Monolog\Logger"
    Kind       SymbolKind    // Class, Method, Property, Function, ...
    Source     SymbolSource  // Project, Builtin, or Vendor
    URI        string        // file:///path/to/file.php
    Range      protocol.Range
    Visibility string        // public, protected, private
    IsStatic   bool
    Type       string        // Property type hint
    ReturnType string        // Method/function return type
    DocComment string        // PHPDoc block
    ParentFQN  string        // Owning class FQN (for methods/properties)
    Params     []ParamInfo   // Parameters (methods/functions)
    Children   []*Symbol     // Nested symbols (class → methods/properties)
    Implements []string      // Interfaces (classes/enums)
    Extends    string        // Parent class
    // ... modifiers: IsAbstract, IsFinal, IsReadonly, BackedType, Value
}
```

### Index

Thread-safe central symbol table. All providers read from it:

```go
type Index struct {
    mu             sync.RWMutex
    symbols        map[string]*Symbol      // FQN → Symbol
    nameIndex      map[string][]string     // Short name → [FQNs]
    fileSymbols    map[string][]*Symbol    // URI → symbols in file
    namespaceIndex map[string][]string     // Namespace → [FQNs]
    inheritanceMap map[string]string       // Child → Parent FQN
    implementsMap  map[string][]string     // Class → [Interface FQNs]
    traitMap       map[string][]string     // Class → [Trait FQNs]
}
```

Key methods: `IndexFileWithSource()`, `Lookup(fqn)`, `LookupByName(name)`, `SearchByPrefix(prefix)`, `SearchByFQNPrefix(prefix)`, `GetClassMembers(classFQN)`, `GetInheritanceChain(classFQN)`.

### FileNode

Result of parsing a PHP file (from `parser.ParseFile()`):

```go
type FileNode struct {
    Namespace  string
    Uses       []UseNode          // use statements
    Classes    []ClassNode        // with Methods, Properties, Constants
    Interfaces []InterfaceNode
    Traits     []TraitNode
    Enums      []EnumNode         // with Cases, Methods
    Functions  []FunctionNode
}
```

## Initialization Sequence

```
Client                              Server
  │                                   │
  │──── initialize ──────────────────>│  Load config, detect framework,
  │<─── InitializeResult ────────────│  create all providers, register builtins
  │                                   │
  │──── initialized ─────────────────>│  Spawn 3 concurrent goroutines:
  │                                   │    1. indexWorkspace()
  │                                   │    2. indexComposerDependencies()
  │                                   │    3. container.Analyze()
```

**indexWorkspace**: Walks filesystem, indexes `.php` files as `SourceProject`, skips excluded dirs.

**indexComposerDependencies**: Parses `composer.json` and `vendor/composer/installed.json` for PSR-4 directories and `autoload.files`. Indexes vendor PHP files as `SourceVendor`.

**container.Analyze**: For Laravel — scans service providers and pre-loads framework bindings. For Symfony — parses `services.yaml` and attributes.

## How Indexing Works

`IndexFileWithSource(uri, source, src)`:

1. Parse source → `FileNode` (namespace, uses, classes, functions, etc.)
2. Lock index, remove old symbols for this URI
3. For each declaration: build FQN, resolve type names via `use` imports, create `Symbol` with source tag
4. Store in all index maps (by FQN, by name, by file, by namespace)
5. Track inheritance, implements, and trait relationships

**Type resolution**: Short names → FQN via `use` statements → current namespace fallback → global.

**Symbol sources**: `SourceProject` (workspace files), `SourceBuiltin` (PHP built-ins), `SourceVendor` (composer dependencies). Used for completion sorting.

## Package Responsibilities

### parser

Custom PHP tokenizer + structural parser. Produces `FileNode` with classes, methods, properties, functions, etc. Handles PHP 8.0–8.5 syntax: union/intersection types, nullable types, enums, readonly classes, attributes, named arguments, pipe operator. Reserved keywords are valid as method names (PHP 7.0+). Fault-tolerant: recovers from missing braces, produces partial results.

### symbols

The `Index` is the shared data store. All providers receive `*Index` and query it. Handles FQN resolution, inheritance chain traversal, trait merging, namespace membership. `PickBestStandalone()` selects the most appropriate symbol when multiple share a name (prefers functions over methods in standalone context).

### lsp

The `Server` struct owns all providers and the index. `Run()` is the JSON-RPC message loop. `handleMessage()` dispatches by method. Documents stored in `sync.Map`. Panic recovery wraps all handlers and goroutines via `recoverPanic`/`goSafe`.

### completion

Detects cursor context from the line prefix:

| Context | Detection | Result |
|---------|-----------|--------|
| `$obj->` | Suffix `->` | Instance methods, properties |
| `$obj->meth` | Contains `->` with trailing text | Filtered instance members |
| `Class::` | Suffix `::` | Static methods, constants, enum cases |
| `Class::meth` | Contains `::` with trailing text | Filtered static members |
| `new ` | Last word is `new` | Class names |
| `use Ns\` | Starts with `use`, contains `\` | Namespace segments + symbols |
| `\Ns\` | Contains `\` | Namespace segments + symbols |
| `#[` | Contains `#[` | Attribute names |
| `app(` | Contains `app(` | Container bindings |
| (default) | None of above | Types, functions, keywords, `$this` |

**Sort order**: `"0"` types → `"1"` same-namespace project → `"2"` other project → `"3"` builtins → `"4"` vendor → `"5"` keywords.

Variable type resolution: method parameters → `$var = new Class()` assignments → `$var = 'literal'` inference → `@var` annotations → class properties.

### hover

Resolves the symbol under the cursor and renders a markdown card:

1. Bold FQN header
2. Docblock summary
3. PHP code block with declaration
4. Context (parent class, override/implements info, container bindings)
5. Docblock details (params, returns, throws, tags)
6. PHP manual link (for builtins)

Access chain resolution walks `$this->logger->info()` by resolving types at each step through the index. Primitive types (`string`, `int`, etc.) produce no hover. Variables typed with primitives show `type $varName` in the code block.

### analyzer

- **FindDefinition**: Resolves symbol → returns `Location` (URI + range). Uses `PickBestStandalone` for fallback lookup.
- **FindReferences**: Searches all indexed files for symbol name matches.
- **GetDocumentSymbols**: Parses file → returns nested `DocumentSymbol` tree. Skips empty names.
- **GetSignatureHelp**: Identifies enclosing function call → returns parameter info.

### diagnostics

Two layers:

1. **Fast checks** (on every change): deprecated functions, unused imports, abstract methods in non-abstract classes.
2. **External tools** (on save): PHPStan (JSON output → diagnostics with token-level ranges) and Laravel Pint (diff output → style warnings). Results cached per-file.

PHPStan diagnostic ranges highlight the specific identifier mentioned in the error message, falling back to the trimmed line content.

### composer

Parses `composer.json` (PSR-4 + `autoload.files`) and `vendor/composer/installed.json`. Returns `[]AutoloadEntry` with path, namespace, vendor flag, and file flag. Supports both Composer v1 and v2 formats.

### container

Analyzes DI container bindings. Laravel: scans service providers for `bind`/`singleton` calls, pre-loads 25+ framework bindings. Symfony: parses `services.yaml`, PHP config files, and `#[Autowire]` attributes. Used by completion (container resolve context) and hover (binding info).

### config

Loads `.php-lsp.json`, merges client `initializationOptions`. Detects framework from `artisan` (Laravel), `bin/console` (Symfony), or `composer.json` requires. Defaults: 10000 max index files, excludes `vendor/node_modules/.git/storage`.

## Editor Extensions

### VSCode (`editors/vscode/`)

TypeScript. Discovers binary from settings → bundled `bin/{platform}-{arch}/php-lsp` → PATH. Passes user settings as `initializationOptions`. Watches `.php` and `composer.json` files. Commands: restart, reindex.

### Zed (`editors/zed/`)

Rust/WASM. Zero user configuration. Downloads binary from GitHub releases for current platform. Falls back to PATH. Server must have sensible defaults.

## Concurrency Model

- **Index**: `sync.RWMutex` — multiple concurrent readers, exclusive writer
- **Documents**: `sync.Map` — lock-free concurrent access
- **Message loop**: Single-threaded in `Server.Run()`
- **Background work**: `goSafe()` spawns goroutines with panic recovery
- **Providers**: Stateless query objects, safe for concurrent use

## Request Flow Example

```
User hovers over "$this->logger->info()" on "info"
  ↓
VSCode sends textDocument/hover {uri, position}
  ↓
Server.handleHover()
  ↓
hover.Provider.GetHover(uri, source, position)
  ├─ getWordAt() → "info"
  ├─ resolveAccessChain() walks left:
  │   "info" ← "->" ← "logger" ← "->" ← "$this"
  │   $this → App\Service (enclosing class)
  │   logger → property type Monolog\Logger
  │   returns "Monolog\Logger"
  ├─ findMember("Monolog\Logger", "info") → Symbol
  └─ formatHover(symbol) → markdown card
  ↓
Server.sendResponse(id, hover)
  ↓
VSCode renders hover popup
```
