# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] - 2026-03-24

### Fixed

- VSCode extension failing to start the bundled LSP binary on Windows (`spawn php-lsp ENOENT`) by appending `.exe` to the PATH fallback command.
- VSCode extension failing to start the bundled LSP binary on Linux/macOS remote environments by ensuring execute permissions after `.vsix` extraction.

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
