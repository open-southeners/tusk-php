# Current Issues

Issues discovered during the **core-solidity-conformance** orchestrated work
(see `.claude/plans/core-solidity-conformance.md`). Each open entry records
**Where**, **What**, and a suggested **Fix**, ordered by severity.

**All issues logged so far have been resolved** — see the "Resolved" section.

---

## Critical

_None open._

---

## High

_None open._

---

## Medium

_None open._

---

## Low / performance

_None open._

---

## Resolved

### C1 — Fatal stack overflow in the hover/completion chain resolver
- Fixed by an atomic re-entrancy depth guard (`const maxResolveDepth = 32`) added to
  `resolve.Resolver`; `ResolveVariableType` and `ResolveVariableTypeTyped` bail out once depth
  exceeds the bound. Regression tests in `internal/resolve/recursion_test.go`. The conformance
  harness now exercises hover/completion on member-access anchors (`->`, `?->`, `::`) with no overflow.

### H1–H4 — PHP 8.3–8.5 syntax silently dropped
- Fixed in `internal/parser/parser.go`: **H1** property hooks (`{ get => …; set => …; }`) are
  parsed brace-balanced and recorded as `PropertyDef.Hooks`; **H2** the `|>` pipe operator
  tokenizes to a dedicated `TokenPipeArrow`; **H3** asymmetric visibility (`public private(set) …`)
  is consumed as a modifier and recorded as `PropertyDef.SetVisibility`; **H4** dynamic class
  constant fetch (`Class::{$name}`) parses cleanly. Tests in `internal/parser/modern_syntax_test.go`.

### M1 — Completion returned items with an empty `Label`
- `GetCompletions` now routes every return path through a `sanitizeCompletions` helper that
  drops empty-`Label` items; `symbols.SearchByFQNPrefix` was also hardened to never return
  empty-name symbols. `internal/completion/provider.go`, `internal/symbols/index.go`.

### M2 — Non-deterministic completion ordering
- `sanitizeCompletions` sorts results deterministically (`SortText` → `Label` → `Kind` →
  `Detail`), preserving the existing priority buckets. Two identical calls now produce
  byte-identical output. `internal/completion/provider.go`.

### M3 — SignatureHelp `activeParameter` could exceed the parameter count
- `GetSignatureHelp` clamps `activeParameter` to `max(0, len(params)-1)`.
  `internal/analyzer/analyzer.go`.

### M4 — `lsp` `exit` notification called `os.Exit(0)` directly
- `Server` gained an injectable `exitFunc func(int)` field (defaults to `os.Exit`, set in
  `NewServer`); the `exit` handler calls it, making the full lifecycle testable.
  `internal/lsp/server.go`.

### L1 — `SearchByFQNPrefix` was O(N) over all symbols
- Replaced with binary search over a maintained sorted-FQN slice (dirty-flag rebuild done
  under the write lock — which also fixed the same latent rebuild race in the pre-existing
  `SearchByPrefix`). `internal/symbols/index.go`.

### L2 — `FindReferences` re-read every indexed file from disk per call
- `symbols.Index` now stores per-URI source in memory (`GetFileSource`), populated by
  `IndexFileWithSource` and `IndexIDEHelperFile`; `findSymbolOccurrences` uses it and falls
  back to disk only when no in-memory copy exists. `internal/symbols/index.go`,
  `internal/analyzer/analyzer.go`.

### L3 — `handleDidOpen` / `handleDidChange` indexed synchronously on the message loop
- Documents larger than `largeDocThreshold` (5000 lines) are indexed asynchronously via
  `goSafe`; smaller documents stay synchronous (so ordinary use and tests are deterministic).
  `internal/lsp/server.go`.

### L4 — strict-mode goroutine re-panic was undocumented
- `SetStrict` and `recoverPanic` now carry doc comments explaining that strict mode
  (`--strict` / `TUSK_STRICT`) causes fatal termination on any recovered panic, including
  from background goroutines. `internal/lsp/server.go`.

### L5 — `FuzzTokenize` was not run by the conformance workflow
- Added a nightly `FuzzTokenize` step (bounded `-fuzztime`) alongside `FuzzParseFile` and
  `FuzzIndexFile`. `.github/workflows/conformance.yml`.

### L6 — Corpus manifest refs
- Corrected in `testdata/corpus/manifest.json`: `guzzlehttp/{guzzle,psr7}` repo URLs → the
  `guzzle/*` GitHub org, `symfony/string` → `v7.2.6`, `tempestphp/tempest` → the `v1.5.0` tag.
  The conformance CI job now fetches the corpus successfully.

### L7 — Chain-resolver depth-guard amplification
- Fixed by adding a `break` after the `ChainResolver` call in `ResolveVariableType`, matching
  `ResolveVariableTypeTyped` — the scan stops at the nearest preceding assignment, removing the
  per-line re-invocation that amplified pathological cost.

### L8 — PHP 8.4 property metadata (hooks, set-visibility) threaded through to hover
- `PropertyNode` and `symbols.Symbol` gained `Hooks` and `SetVisibility` fields, populated
  through `toPropertyNode` and the symbol indexer; hover cards now render asymmetric visibility
  (`public private(set) …`) and `{ get; set; }` hooks. `internal/parser/compat.go`,
  `internal/symbols/index.go`, `internal/hover/format.go`.
