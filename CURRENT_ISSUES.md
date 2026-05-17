# Current Issues

Issues discovered during the **core-solidity-conformance** orchestrated work
(see `.claude/plans/core-solidity-conformance.md`). Each entry: **Where**,
**What**, suggested **Fix**. None blocked the units that surfaced them — all are
logged for follow-up. Ordered by severity.

---

## Critical

_None open. (C1 resolved — see "Resolved" below.)_

---

## High

_None open. (H1–H4 resolved — see "Resolved" below.)_

---

## Medium

### M1 — Completion returns items with an empty `Label`
- **Where:** `internal/completion/provider.go` (`completeUse` → `completeNamespacePath` → `symbols.SearchByFQNPrefix`)
- **What:** With the cursor on the last segment of a `use` path (e.g. `use Illuminate\Support\Str`), `GetCompletions` returns at least one `CompletionItem` whose `Label` is `""`. Seen on `testdata/laravel/config/{cache,database,session}.php` and `database/factories/UserFactory.php`.
- **Fix:** Filter empty-name entries out of `SearchByFQNPrefix` (or wherever namespace segments are produced).

### M2 — Non-deterministic completion ordering
- **Where:** `internal/completion/provider.go` (`GetCompletions`)
- **What:** Two identical completion calls on the same input return items in different order (Go map iteration is random; results are not sorted). Breaks reproducibility — and the determinism invariant the conformance suite enforces.
- **Fix:** Sort `CompletionItem` results by `Label`, then `Kind`, then `Detail` before returning.

### M3 — SignatureHelp `activeParameter` can exceed the parameter count
- **Where:** `internal/analyzer/analyzer.go:700` (`GetSignatureHelp` / `extractCallInfo`)
- **What:** `extractCallInfo` counts depth-0 commas to derive `activeParam` but never caps it against the parameter count, so it can point past the last parameter (observed `activeParameter=3` for a 2-parameter function).
- **Fix:** Cap `activeParam` to `max(0, len(sym.Params)-1)` before building the `SignatureHelp` response.

### M4 — `lsp` `exit` notification calls `os.Exit(0)` directly
- **Where:** `internal/lsp/server.go:189` (`handleMessage`)
- **What:** The `exit` notification calls `os.Exit(0)` unconditionally, making the full LSP lifecycle (including `exit`) untestable without subprocess tricks.
- **Fix:** Add an `exitFunc func(int)` field on `Server` defaulting to `os.Exit`, set in `NewServer`, so tests can inject a no-op.

---

## Low / performance

### L1 — `SearchByFQNPrefix` is O(N) over all symbols
- **Where:** `internal/symbols/index.go` (`SearchByFQNPrefix`)
- **What:** FQN prefix search iterates every indexed symbol (10k+ in the corpus). The conformance operations phase is capped at 200 files because of it.
- **Fix:** Use the existing `sortedNames` structure with binary search for FQN prefix search, then raise the conformance operations cap.

### L2 — `FindReferences` reads every indexed file from disk per call
- **Where:** `internal/analyzer/analyzer.go:442` (`findSymbolOccurrences`)
- **What:** Each call does O(N) disk reads across all indexed URIs — prohibitive at corpus scale; workspace-wide reference search is untested at scale.
- **Fix:** Store file source in the `Index` so `findSymbolOccurrences` avoids disk I/O, or route through the LSP server's in-memory `documents` map.

### L3 — `handleDidOpen` / `handleDidChange` index synchronously on the message loop
- **Where:** `internal/lsp/server.go:330` (`handleDidOpen`)
- **What:** Indexing runs inline on the JSON-RPC message loop; large documents add noticeable latency to the loop.
- **Fix:** Run `indexFileByURI` in a `goSafe` goroutine for large documents, or document the latency bound.

### L4 — `recoverPanic` re-panic in strict mode from a goroutine kills the process
- **Where:** `internal/lsp/server.go:74` (`recoverPanic`, with `--strict`/`TUSK_STRICT`)
- **What:** In strict mode a panic inside a `goSafe` goroutine re-panics and terminates the process rather than producing a structured error. This is the intended strict-mode behaviour but is currently undocumented.
- **Fix:** Document this in the `SetStrict` doc comment so the conformance harness/tooling accounts for the fatal exit.

### L5 — `FuzzTokenize` is not wired into the conformance workflow
- **Where:** `.github/workflows/conformance.yml` (nightly job)
- **What:** Unit 2 added a `FuzzTokenize` target in `internal/parser/`, but the nightly workflow only runs `FuzzParseFile` and `FuzzIndexFile`.
- **Fix:** Add a nightly step running `FuzzTokenize` with a bounded `-fuzztime`.

### L8 — PHP 8.4 property metadata (hooks, set-visibility) stops at the structural parser
- **Where:** `internal/parser/compat.go` (`PropertyNode` / `toPropertyNode`), `internal/symbols/index.go`
- **What:** The H1/H3 fixes record property hooks and asymmetric set-visibility on `parser.PropertyDef`, but `PropertyNode` (the `FileNode` AST shape consumed by `symbols`/`hover`/`completion`) does not carry them, so the new PHP 8.4 metadata is not yet surfaced in hover/completion. The parser-level bug (silent drop / mis-parse) is fixed; this is the remaining enhancement to thread the data through.
- **Fix:** Add `Hooks` and `SetVisibility` to `PropertyNode`, populate them in `toPropertyNode`, and have the symbol indexer surface them in hover text.

---

## Resolved

### C1 — Fatal stack overflow in the hover/completion chain resolver `[resolved]`
- Fixed by an atomic re-entrancy depth guard (`const maxResolveDepth = 32`) added to
  `resolve.Resolver`; `ResolveVariableType` and `ResolveVariableTypeTyped` now bail out
  (returning an empty result) once depth exceeds the bound. Regression tests in
  `internal/resolve/recursion_test.go` reproduce the self-referential and mutual-recursion
  cycles. The conformance harness was updated to exercise hover/completion on member-access
  anchors (`->`, `?->`, `::`) — previously restricted to `use`-import anchors to dodge this
  crash — and now runs that path with no overflow.

### H1–H4 — PHP 8.3–8.5 syntax silently dropped `[resolved]`
- Fixed in `internal/parser/parser.go`: **H1** property hooks (`{ get => …; set => …; }`) are
  parsed brace-balanced and recorded as `PropertyDef.Hooks`; **H2** the `|>` pipe operator now
  tokenizes to a dedicated `TokenPipeArrow`; **H3** asymmetric visibility (`public private(set) …`)
  is consumed as a modifier and recorded as `PropertyDef.SetVisibility`; **H4** dynamic class
  constant fetch (`Class::{$name}`) parses cleanly without corrupting class structure. Regression
  tests in `internal/parser/modern_syntax_test.go`. Remaining enhancement tracked as L8.

### L6 — Corpus manifest refs `[resolved]`
- Corrected in `testdata/corpus/manifest.json`: `guzzlehttp/{guzzle,psr7}` repo URLs → the
  `guzzle/*` GitHub org, `symfony/string` → `v7.2.6`, `tempestphp/tempest` → the `v1.5.0` tag.
  The conformance CI job now fetches the corpus successfully.

### L7 — Chain-resolver depth-guard amplification `[resolved]`
- Fixed by adding a `break` after the `ChainResolver` call in `ResolveVariableType`, matching
  `ResolveVariableTypeTyped` — once the nearest preceding assignment is processed the scan stops,
  removing the per-line re-invocation that amplified pathological cost.
