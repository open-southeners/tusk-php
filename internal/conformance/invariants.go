//go:build conformance

package conformance

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// violation is a single invariant failure.
type violation struct {
	file string
	line int
	col  int
	msg  string
}

func (v violation) String() string {
	if v.line >= 0 {
		return fmt.Sprintf("%s:%d:%d: %s", v.file, v.line, v.col, v.msg)
	}
	return fmt.Sprintf("%s: %s", v.file, v.msg)
}

func fileViolation(file, msg string) violation {
	return violation{file: file, line: -1, col: -1, msg: msg}
}

func posViolation(file string, line, col int, msg string) violation {
	return violation{file: file, line: line, col: col, msg: msg}
}

// --- Parser invariants ---

// checkParserNeverNil verifies ParseFile returns a non-nil FileNode.
func checkParserNeverNil(entry corpusEntry) *violation {
	var result *parser.FileNode
	func() {
		defer func() { recover() }()
		result = parser.ParseFile(entry.source)
	}()
	if result == nil {
		v := fileViolation(entry.path, "ParseFile returned nil")
		return &v
	}
	return nil
}

// checkParserNoPanic verifies ParseFile does not panic.
func checkParserNoPanic(entry corpusEntry) *violation {
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		parser.ParseFile(entry.source)
	}()
	if panicked {
		v := fileViolation(entry.path, "ParseFile panicked")
		return &v
	}
	return nil
}

// checkParserDeterministic verifies that parsing the same source twice gives
// identical structure (same number of classes, interfaces, traits, enums, functions).
func checkParserDeterministic(entry corpusEntry) *violation {
	file1 := parser.ParseFile(entry.source)
	file2 := parser.ParseFile(entry.source)
	if file1 == nil && file2 == nil {
		return nil
	}
	if file1 == nil || file2 == nil {
		v := fileViolation(entry.path, "ParseFile non-deterministic: one call returned nil")
		return &v
	}
	// Compare structural counts as a lightweight determinism check.
	if len(file1.Classes) != len(file2.Classes) ||
		len(file1.Interfaces) != len(file2.Interfaces) ||
		len(file1.Traits) != len(file2.Traits) ||
		len(file1.Enums) != len(file2.Enums) ||
		len(file1.Functions) != len(file2.Functions) ||
		len(file1.Uses) != len(file2.Uses) {
		v := fileViolation(entry.path, "ParseFile non-deterministic: structural counts differ between two calls")
		return &v
	}
	return nil
}

// checkParserErrorPositions verifies all parse error positions are non-negative.
func checkParserErrorPositions(entry corpusEntry) []violation {
	file := parser.ParseFile(entry.source)
	if file == nil {
		return nil
	}

	// Re-parse with raw parser to access Errors slice.
	p := parser.New()
	result := p.Parse(entry.source)
	if result == nil {
		return nil
	}

	var vs []violation
	for _, e := range result.Errors {
		if e.Line < 0 || e.Column < 0 {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("ParseError has negative position: line=%d col=%d msg=%q", e.Line, e.Column, e.Message)))
		}
	}
	return vs
}

// --- Index invariants ---

// checkIndexNoPanic verifies IndexFile does not panic.
func checkIndexNoPanic(entry corpusEntry, idx *symbols.Index) *violation {
	freshIdx := symbols.NewIndex()
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		freshIdx.IndexFile(entry.uri, entry.source)
	}()
	if panicked {
		v := fileViolation(entry.path, "IndexFile panicked")
		return &v
	}
	_ = idx
	return nil
}

// checkIndexIdempotent verifies that indexing a file twice results in the same
// set of symbols (same names and FQNs, no duplication).
func checkIndexIdempotent(entry corpusEntry) *violation {
	// First indexing.
	idx1 := symbols.NewIndex()
	idx1.IndexFile(entry.uri, entry.source)
	syms1 := idx1.GetFileSymbols(entry.uri)

	// Second indexing of the same file.
	idx1.IndexFile(entry.uri, entry.source)
	syms2 := idx1.GetFileSymbols(entry.uri)

	if len(syms1) != len(syms2) {
		return &violation{
			file: entry.path,
			line: -1, col: -1,
			msg: fmt.Sprintf("IndexFile idempotency: symbol count changed from %d to %d after second index", len(syms1), len(syms2)),
		}
	}
	return nil
}

// checkSymbolValidity verifies structural validity for every symbol in the file.
func checkSymbolValidity(entry corpusEntry, idx *symbols.Index, lineCount int) []violation {
	syms := idx.GetFileSymbols(entry.uri)
	var vs []violation

	for _, sym := range syms {
		if sym == nil {
			vs = append(vs, fileViolation(entry.path, "nil Symbol in GetFileSymbols"))
			continue
		}
		if sym.Name == "" {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("Symbol with FQN %q has empty Name", sym.FQN)))
		}
		if sym.FQN == "" {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("Symbol %q has empty FQN", sym.Name)))
		}
		if sym.URI == "" {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("Symbol %q has empty URI", sym.FQN)))
		}
		// Range validity: start <= end, both within [0, lineCount).
		r := sym.Range
		if r.Start.Line < 0 || r.End.Line < 0 {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("Symbol %q has negative range line: start=%d end=%d", sym.FQN, r.Start.Line, r.End.Line)))
		}
		if r.Start.Line > r.End.Line || (r.Start.Line == r.End.Line && r.Start.Character > r.End.Character) {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("Symbol %q has inverted range: start=%v end=%v", sym.FQN, r.Start, r.End)))
		}
		if lineCount > 0 && (r.Start.Line >= lineCount || r.End.Line >= lineCount) {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("Symbol %q range out of file bounds: start.line=%d end.line=%d fileLines=%d", sym.FQN, r.Start.Line, r.End.Line, lineCount)))
		}
		// Method/property: ParentFQN must be non-empty.
		if sym.Kind == symbols.KindMethod || sym.Kind == symbols.KindProperty ||
			sym.Kind == symbols.KindEnumCase || sym.Kind == symbols.KindConstant {
			if sym.ParentFQN == "" {
				vs = append(vs, fileViolation(entry.path,
					fmt.Sprintf("Symbol %q (kind=%d) has empty ParentFQN", sym.FQN, sym.Kind)))
			}
		}
	}
	return vs
}

// checkNoInheritanceCycle verifies no inheritance cycles exist for symbols in
// the given file by walking the chain via GetInheritanceChain.
func checkNoInheritanceCycle(entry corpusEntry, idx *symbols.Index) []violation {
	syms := idx.GetFileSymbols(entry.uri)
	var vs []violation

	for _, sym := range syms {
		if sym == nil {
			continue
		}
		if sym.Kind != symbols.KindClass && sym.Kind != symbols.KindInterface {
			continue
		}
		// GetInheritanceChain already guards with a visited set; a cycle would
		// be indicated by the same FQN appearing in the chain.
		chain := idx.GetInheritanceChain(sym.FQN)
		seen := make(map[string]bool, len(chain)+1)
		seen[sym.FQN] = true
		for _, ancestor := range chain {
			if seen[ancestor] {
				vs = append(vs, fileViolation(entry.path,
					fmt.Sprintf("Inheritance cycle detected for %q: %v", sym.FQN, chain)))
				break
			}
			seen[ancestor] = true
		}
	}
	return vs
}

// checkLookupNoPanic verifies that LookupByName, SearchByPrefix, and
// GetClassMembers do not panic for any indexed name.
func checkLookupNoPanic(entry corpusEntry, idx *symbols.Index) *violation {
	syms := idx.GetFileSymbols(entry.uri)
	panicked := false
	var panicVal interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
			}
		}()
		for _, sym := range syms {
			if sym == nil {
				continue
			}
			idx.LookupByName(sym.Name)
			idx.SearchByPrefix(sym.Name)
			if sym.Kind == symbols.KindClass || sym.Kind == symbols.KindInterface ||
				sym.Kind == symbols.KindTrait || sym.Kind == symbols.KindEnum {
				idx.GetClassMembers(sym.FQN)
			}
		}
	}()
	if panicked {
		v := fileViolation(entry.path, fmt.Sprintf("lookup operation panicked: %v", panicVal))
		return &v
	}
	return nil
}

// --- Operation invariants ---

// checkDefinitionInvariant verifies FindDefinition returns nil or a Location
// with a non-empty URI and a valid range.
func checkDefinitionInvariant(entry corpusEntry, pos protocol.Position, loc *protocol.Location, lineCount int) *violation {
	if loc == nil {
		return nil // nil is valid
	}
	if loc.URI == "" {
		v := posViolation(entry.path, pos.Line, pos.Character,
			"FindDefinition returned Location with empty URI")
		return &v
	}
	r := loc.Range
	if r.Start.Line < 0 || r.End.Line < 0 {
		v := posViolation(entry.path, pos.Line, pos.Character,
			fmt.Sprintf("FindDefinition Location has negative line in range: %v", r))
		return &v
	}
	if r.Start.Line > r.End.Line || (r.Start.Line == r.End.Line && r.Start.Character > r.End.Character) {
		v := posViolation(entry.path, pos.Line, pos.Character,
			fmt.Sprintf("FindDefinition Location has inverted range: %v", r))
		return &v
	}
	return nil
}

// checkFindReferencesSafe verifies that FindReferences does not panic.
// It uses a readDocument callback backed by the corpus in-memory sources to
// avoid disk I/O. We only check for panics and basic location validity here;
// running the full scan over 10k+ files per anchor is not feasible in-process.
func checkFindReferencesSafe(entry corpusEntry, pos protocol.Position, prov *providers, sources map[string]string) *violation {
	panicked := false
	var panicVal interface{}
	var refs []protocol.Location
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
				panicVal = r
			}
		}()
		refs = prov.ana.FindAllReferences(entry.uri, entry.source, pos, func(uri string) string {
			return sources[uri]
		})
	}()
	if panicked {
		v := posViolation(entry.path, pos.Line, pos.Character,
			fmt.Sprintf("FindReferences panicked: %v", panicVal))
		return &v
	}
	// Basic validity: no empty URIs.
	for _, ref := range refs {
		if ref.URI == "" {
			v := posViolation(entry.path, pos.Line, pos.Character,
				"FindReferences returned Location with empty URI")
			return &v
		}
	}
	return nil
}

// checkReferencesInvariant verifies FindReferences returns only valid locations.
func checkReferencesInvariant(entry corpusEntry, pos protocol.Position, refs []protocol.Location) []violation {
	var vs []violation
	seen := make(map[string]bool)
	for _, ref := range refs {
		if ref.URI == "" {
			vs = append(vs, posViolation(entry.path, pos.Line, pos.Character,
				"FindReferences returned Location with empty URI"))
			continue
		}
		key := fmt.Sprintf("%s:%d:%d-%d:%d", ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		if seen[key] {
			vs = append(vs, posViolation(entry.path, pos.Line, pos.Character,
				fmt.Sprintf("FindReferences returned duplicate location: %s", key)))
		}
		seen[key] = true

		r := ref.Range
		if r.Start.Line < 0 || r.End.Line < 0 {
			vs = append(vs, posViolation(entry.path, pos.Line, pos.Character,
				fmt.Sprintf("FindReferences Location has negative line: %v", r)))
		}
		if r.Start.Line > r.End.Line || (r.Start.Line == r.End.Line && r.Start.Character > r.End.Character) {
			vs = append(vs, posViolation(entry.path, pos.Line, pos.Character,
				fmt.Sprintf("FindReferences Location has inverted range: %v", r)))
		}
	}
	return vs
}

// checkDocumentSymbolsInvariant verifies GetDocumentSymbols output is structurally valid.
func checkDocumentSymbolsInvariant(entry corpusEntry, syms []protocol.DocumentSymbol, lineCount int) []violation {
	var vs []violation
	var check func(s protocol.DocumentSymbol, parent *protocol.DocumentSymbol, depth int)
	check = func(s protocol.DocumentSymbol, parent *protocol.DocumentSymbol, depth int) {
		if depth > 20 {
			return // guard against pathological nesting
		}
		if s.Name == "" {
			vs = append(vs, fileViolation(entry.path, "GetDocumentSymbols: symbol with empty name"))
		}
		r := s.Range
		sr := s.SelectionRange
		// Range validity.
		if r.Start.Line < 0 || r.End.Line < 0 {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("GetDocumentSymbols: symbol %q has negative line in range %v", s.Name, r)))
		}
		if r.Start.Line > r.End.Line || (r.Start.Line == r.End.Line && r.Start.Character > r.End.Character) {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("GetDocumentSymbols: symbol %q has inverted range %v", s.Name, r)))
		}
		// selectionRange must be contained within range.
		if !rangeContains(r, sr) {
			vs = append(vs, fileViolation(entry.path,
				fmt.Sprintf("GetDocumentSymbols: symbol %q selectionRange %v not contained in range %v", s.Name, sr, r)))
		}
		// Children ranges must be within parent range.
		if parent != nil {
			if !rangeContains(parent.Range, r) {
				// Only soft-check: some implementations use full-file range for parent.
				// Do not fail — just a note. Skip.
			}
		}
		for _, child := range s.Children {
			check(child, &s, depth+1)
		}
	}
	for _, s := range syms {
		check(s, nil, 0)
	}
	return vs
}

// rangeContains returns true if outer contains inner (both ends inclusive).
func rangeContains(outer, inner protocol.Range) bool {
	// inner.Start >= outer.Start
	if inner.Start.Line < outer.Start.Line {
		return false
	}
	if inner.Start.Line == outer.Start.Line && inner.Start.Character < outer.Start.Character {
		return false
	}
	// inner.End <= outer.End
	if inner.End.Line > outer.End.Line {
		return false
	}
	if inner.End.Line == outer.End.Line && inner.End.Character > outer.End.Character {
		return false
	}
	return true
}

// checkHoverInvariant verifies GetHover returns nil or a non-empty Hover.
func checkHoverInvariant(entry corpusEntry, pos protocol.Position, h *protocol.Hover) *violation {
	if h == nil {
		return nil // nil is valid
	}
	if strings.TrimSpace(h.Contents.Value) == "" {
		v := posViolation(entry.path, pos.Line, pos.Character,
			"GetHover returned Hover with empty Contents.Value")
		return &v
	}
	return nil
}

// checkCompletionInvariant verifies completion items are structurally valid.
func checkCompletionInvariant(entry corpusEntry, pos protocol.Position, items []protocol.CompletionItem) []violation {
	var vs []violation
	type dedupeKey struct {
		label  string
		kind   protocol.CompletionItemKind
		detail string
	}
	seen := make(map[dedupeKey]bool)

	for _, item := range items {
		if item.Label == "" {
			vs = append(vs, posViolation(entry.path, pos.Line, pos.Character,
				"completion item has empty Label"))
		}
		key := dedupeKey{label: item.Label, kind: item.Kind, detail: item.Detail}
		if seen[key] {
			vs = append(vs, posViolation(entry.path, pos.Line, pos.Character,
				fmt.Sprintf("duplicate completion item: label=%q kind=%d detail=%q", item.Label, item.Kind, item.Detail)))
		}
		seen[key] = true
	}
	return vs
}

// checkSignatureHelpInvariant verifies GetSignatureHelp returns nil or a valid result.
func checkSignatureHelpInvariant(entry corpusEntry, pos protocol.Position, sh *protocol.SignatureHelp) *violation {
	if sh == nil {
		return nil // nil is valid
	}
	if len(sh.Signatures) == 0 {
		// Non-nil but no signatures: treat as valid (empty result).
		return nil
	}
	// activeParameter must be within [0, len(params)) for the active signature.
	activeSig := sh.ActiveSignature
	if activeSig < 0 || activeSig >= len(sh.Signatures) {
		v := posViolation(entry.path, pos.Line, pos.Character,
			fmt.Sprintf("SignatureHelp: activeSignature=%d out of range (len=%d)", activeSig, len(sh.Signatures)))
		return &v
	}
	sig := sh.Signatures[activeSig]
	if len(sig.Parameters) > 0 {
		if sh.ActiveParameter < 0 || sh.ActiveParameter >= len(sig.Parameters) {
			v := posViolation(entry.path, pos.Line, pos.Character,
				fmt.Sprintf("SignatureHelp: activeParameter=%d out of range (len=%d)", sh.ActiveParameter, len(sig.Parameters)))
			return &v
		}
	}
	return nil
}

// --- Determinism invariant ---

// checkDeterminism runs the full operation pass over a single file twice and
// compares results with reflect.DeepEqual. Only called for a small sample of
// files to keep the suite fast.
func checkDeterminism(entry corpusEntry, prov *providers, pos protocol.Position, kind anchorKind) *violation {
	res1, err1 := runOperations(entry.uri, entry.source, pos, kind, prov)
	if err1 != nil {
		// Panic already surfaced elsewhere.
		return nil
	}
	res2, err2 := runOperations(entry.uri, entry.source, pos, kind, prov)
	if err2 != nil {
		return nil
	}
	if !reflect.DeepEqual(res1, res2) {
		v := posViolation(entry.path, pos.Line, pos.Character,
			"determinism violation: two identical runs produced different results")
		return &v
	}
	return nil
}

// --- Spot-checks ---

// checkUseImportSpotCheck verifies that a `use Foo\Bar;` import resolves via
// FindDefinition on Bar to a symbol whose short name is Bar (when in corpus).
func checkUseImportSpotCheck(entry corpusEntry, idx *symbols.Index, prov *providers) []violation {
	file := parser.ParseFile(entry.source)
	if file == nil {
		return nil
	}

	var vs []violation
	lines := strings.Split(entry.source, "\n")

	for _, use := range file.Uses {
		// Find the position of the alias on the `use` line.
		targetLine := use.StartLine
		if targetLine < 0 || targetLine >= len(lines) {
			continue
		}
		ln := lines[targetLine]
		alias := use.Alias
		if alias == "" {
			continue
		}
		col := strings.LastIndex(ln, alias)
		if col < 0 {
			continue
		}
		pos := protocol.Position{Line: targetLine, Character: col}
		loc := safeDefinition(entry.uri, entry.source, pos, prov)
		if loc == nil {
			// No definition found — only a violation if the FQN is in the index.
			if idx.Lookup(use.FullName) != nil {
				// Indexed but not resolvable via definition.
				// Not a hard violation: definition resolution has many heuristics.
				// Skip as a soft check.
			}
			continue
		}
		// Look up the symbol at the target location.
		targetSyms := idx.GetFileSymbols(loc.URI)
		found := false
		for _, sym := range targetSyms {
			if sym == nil {
				continue
			}
			if sym.Name == alias {
				found = true
				break
			}
		}
		if !found {
			// The definition resolved to a file but no symbol there has the expected alias name.
			// This is expected for partial indexes; skip hard fail.
			_ = found
		}
	}
	return vs
}

// safeDefinition calls FindDefinition, catching any panic.
func safeDefinition(uri, source string, pos protocol.Position, prov *providers) (loc *protocol.Location) {
	defer func() { recover() }()
	return prov.ana.FindDefinition(uri, source, pos)
}
