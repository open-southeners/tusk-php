# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] - 2026-05-17

### Added

- PHP 8.3–8.5 syntax support in the parser: property hooks (`{ get => ...; set => ...; }`), the `|>` pipe operator, asymmetric visibility (`public private(set)`), and dynamic class constant fetch (`Class::{$name}`) — constructs that were previously silently dropped or mis-parsed.
- Hover cards for properties now show PHP 8.4 property hooks and asymmetric `(set)` visibility.

### Changed

- Faster workspace-wide symbol prefix search via binary lookup, improving completion responsiveness on large projects.
- Find references now reuses indexed in-memory source instead of re-reading every file from disk on each request.
- Large files are indexed off the JSON-RPC message loop, keeping the server responsive while big documents are processed.

### Fixed

- Fatal crash (stack overflow) in hover and completion when resolving self-referential or mutually-referential variable assignments.
- Excessive work and apparent hangs in the variable-type chain resolver on certain repeated assignment patterns.
- Property hook bodies leaking their local variables (`$value`, `$this`) as spurious class properties in completion and document symbols.
- Completion returning blank entries when completing namespace segments in `use` statements.
- Completion results being returned in a non-deterministic order between identical requests.
- Signature help highlighting a parameter position past the end of the parameter list when more arguments than parameters were typed.

## [0.4.0] - 2026-03-25

### Added

- Laravel Facade support: static calls like `Cache::get()` now resolve through the container via `getFacadeAccessor()`, providing completions and hover cards from the concrete class's methods.
- Facade concrete method completion: methods not declared in `@method static` annotations but present on the resolved concrete class are surfaced as static completions.
- Nested trait member resolution at all depth levels (trait using another trait using another trait, etc.), propagating methods and properties through the full chain.

### Fixed

- `@method static` docblock annotations not setting `IsStatic` on virtual members, causing them to be filtered out of `ClassName::` completions.
- Trait `use` declarations inside other traits being silently discarded by the parser, preventing nested trait members from appearing in completions and hover.
- `traitMap` entries not being cleaned up on file re-index, causing stale trait associations to persist until process restart.
- Windows workspace path containing percent-encoded characters (`%3A` for `:`) not being decoded, causing the LSP server to index zero files.
- All `file://` URI-to-path conversions now use proper URL decoding and Windows drive letter handling via the shared `symbols.URIToPath` helper.

## [0.3.2] - 2026-03-24

### Fixed

- VSCode extension shipping without bundled LSP binaries due to release CI not copying platform binaries into the extension's `bin/` directory before packaging the `.vsix`.

## [0.3.1] - 2026-03-24

### Fixed

- VSCode extension failing to start the bundled LSP binary on Windows (`spawn php-lsp ENOENT`) by appending `.exe` to the PATH fallback command.
- VSCode extension failing to start the bundled LSP binary on Linux/macOS remote environments by ensuring execute permissions after `.vsix` extraction.
- Zed extension build failure caused by `unicode-segmentation` 1.13.0 breaking `heck` 0.4.1 (`UnicodeWords` made private); pinned to 1.12.0.

## [0.3.0] - 2026-03-24

### Added

- Generic-aware type resolution for template-annotated classes and methods, preserving concrete type parameters through completions, hover, and chained calls.
- Generic-aware support for Laravel Eloquent builders and collections, including concrete model propagation for methods such as `query()`, `where()`, `get()`, `first()`, `find()`, and related collection helpers.
- Array literal and array-shape inference in resolved expression types, improving type preservation for inferred variables and collection payloads.
- Local `scripts/set-version.sh` utility to update repository version strings for releases.

### Changed

- Release automation now targets plain semantic version tags without the previous release prefix.
- Expanded Laravel and Symfony end-to-end fixtures and coverage around generics, array shapes, and framework controller flows.

### Fixed

- Incorrect generic propagation across LSP completions, hover results, and shared resolver paths.
- Variable type inference for generic return values, including Laravel helpers such as `Arr::first()` and `Collection::first()`.
- Symfony test fixture compatibility issues that were causing CI instability.

## [0.2.0] - 2026-03-23

### Added

- Multi-source model attribute discovery for Eloquent and Doctrine ORM models.
- Virtual property injection from `@property`/`@method` docblock tags on classes.
- IDE helper file support (`_ide_helper_models.php`, `_ide_helper.php`) with member merging into existing indexed classes.
- Eloquent relation discovery from method return types and `$this->hasMany()`/`belongsTo()`/etc. calls.
- Eloquent accessor/mutator detection (both legacy `getNameAttribute()` and modern `Attribute` cast).
- Eloquent virtual static methods (`where`, `find`, `first`, `with`, `orderBy`, etc.) mimicking `__callStatic` forwarding.
- Doctrine ORM entity support: `#[Column]`, `#[Id]`, `#[GeneratedValue]`, `#[OneToMany]`, `#[ManyToOne]`, `#[OneToOne]`, `#[ManyToMany]` PHP 8 attribute parsing.
- Doctrine repository class detection and association to entities via `#[Entity(repositoryClass: ...)]`.
- Database schema introspection for MySQL, PostgreSQL, and SQLite via `.env` credentials with in-memory caching.
- Laravel migration file parsing to extract column definitions (`$table->string()`, `$table->foreignId()`, `$table->timestamps()`, etc.).
- Builder string argument completion for column names inside `where('`, `orderBy('`, `select('`, `pluck('`, `groupBy('`, etc.
- Builder string argument completion for relation names inside `with('`, `has('`, `whereHas('`, `load('`, `withCount('`, etc.
- Array argument support for Builder completions (`select(['`, `get(['`, `with(['`, etc.).
- Strict DB-only column filtering for `get()` (only suggests columns from IDE helper, database introspection, or migrations).
- `.env` file parser for reading database connection settings.
- `database` configuration option in LSP initialization settings to enable/disable DB introspection.
- Built-in diagnostics engine with standalone `internal/checks/` package reusable by CLI tools and CI pipelines.
- Unused import detection with word-boundary scanning, aliased imports, `use function`/`use const`, and PHP 8 attributes.
- Unused private method and property detection (excludes magic methods).
- Unreachable code detection after `return`, `throw`, `exit`/`die`, `continue`, `break`.
- Redundant union member detection (duplicates, `?Type|null`, supertype subsumption for `mixed`, `object`, `iterable`).
- Redundant nullsafe `?->` operator detection on non-nullable types.
- Unknown column validation in Builder string arguments (`where`, `orderBy`, `select`, `get`, etc.).
- Unknown relation validation in Builder string arguments (`with`, `has`, `whereHas`, `load`, etc.).
- Aggregate relation method second-arg validation (`withSum('relation', 'column')` checks column on the related model).
- `DiagnosticTag` support: unused code greyed out (`Unnecessary`), deprecated functions struck through (`Deprecated`).
- `diagnosticRules` configuration in `.php-lsp.json` to enable/disable individual diagnostic rules.
- Multi-line method chain resolution for hover, go-to-definition, and completions.
- `resolve.JoinChainLines()` helper to join continuation lines starting with `->`, `::`, or `?->`.
- Comprehensive chain resolution test suite covering single-line and multi-line Eloquent method chains.

### Fixed

- Hover cards and go-to-definition landing on wrong vendor symbols when method chains span multiple lines.
- Completion provider now resolves multi-line chains correctly instead of falling through to global completions.

## [0.1.0] - 2026-03-21

### Added

- Initial `Tusk PHP LSP` release with a Go-based PHP language server binary.
- VS Code extension packaging for the bundled LSP client.
- Zed extension packaging for the WebAssembly-based extension.
- Cross-platform release artifacts for Linux, macOS, and Windows.
