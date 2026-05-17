# Current Issues

Issues discovered during the **core-solidity-conformance** orchestrated work
(see `.claude/plans/core-solidity-conformance.md`). Each entry: **Where**,
**What**, suggested **Fix**. None blocked the units that surfaced them — all are
logged for follow-up. Ordered by severity.

---

## Critical

### C1 — Fatal stack overflow in the hover/completion chain resolver
- **Where:** `internal/resolve/resolve.go:678` ↔ `internal/hover/provider.go:49` ↔ `internal/completion/provider.go:35`
- **What:** `resolveAccessChain` → `ResolveVariableType` → `ChainResolver` (`resolveExpressionType`) → `resolveAccessChain` is an infinite mutual recursion on certain inputs. It overflows the goroutine stack — a **fatal** runtime error that `recover()` cannot catch, so panic recovery does not save it. The conformance harness only avoids it by restricting hover/completion to `use`-import anchors; any real hover/completion on a member-access expression can trigger it.
- **Fix:** Add a recursion depth counter (or a visited-expression set) in `ResolveVariableType` (`resolve.go:677`) before it calls `ChainResolver`; bail out and return an unresolved type once depth exceeds a small bound.

---

## High

### H1 — PHP 8.4 property hooks are silently dropped
- **Where:** `internal/parser/parser.go`
- **What:** `get`/`set` hook syntax in property declarations (`public float $f { get => ...; set => ...; }`) is neither tokenized nor parsed. No `ParseError` is recorded — hook bodies are discarded, so hooked properties look like ordinary properties.
- **Fix:** Tokenize `get`/`set` as keywords inside a property-hook context (brace immediately after a typed property), then parse the hook bodies in `parseStructure`.

### H2 — PHP 8.5 pipe operator `|>` is silently ignored
- **Where:** `internal/parser/parser.go`
- **What:** `|>` is not in the token grammar; `|` and `>` are consumed as separate unrecognized tokens with no error. Pipe expressions are invisible to the LSP. (README/CLAUDE.md already advertise `|>` support.)
- **Fix:** Add two-character lookahead for `|>` in the tokenizer producing a dedicated token; handle it in expression parsing.

### H3 — Asymmetric visibility `public private(set)` is not parsed
- **Where:** `internal/parser/parser.go`
- **What:** The `private(set)` modifier after a primary visibility modifier is not recognised; the parser only recovers silently, which can confuse the member-parsing state machine.
- **Fix:** After a primary visibility modifier, detect the `identifier(set)`/`identifier(get)` pattern and consume it as a compound asymmetric-visibility specifier.

### H4 — Dynamic class constant fetch `Class::{$name}` is not parsed
- **Where:** `internal/parser/parser.go`
- **What:** After `::`, a brace-enclosed variable expression (`{$name}`) is not handled and the dynamic access is silently dropped.
- **Fix:** After `TokenDoubleColon`, recognise `{` `<variable>` `}` and record a dynamic constant-fetch node.

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

### L6 — Corpus manifest refs are unverified
- **Where:** `testdata/corpus/manifest.json`
- **What:** Git tags were chosen from known release patterns but not network-verified. Uncertain: `guzzlehttp/guzzle`, `guzzlehttp/psr7`, `tempestphp/tempest` (repo may be `tempest-framework`), `laravel/framework` `v12.13.0`, `symfony/demo` `v2.7.0`.
- **Fix:** Run `bash scripts/fetch-corpus.sh` once with network access; commit the resulting `testdata/corpus/corpus.lock` (resolved SHAs) as the true pin.
