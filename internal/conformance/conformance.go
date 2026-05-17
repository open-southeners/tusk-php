//go:build conformance

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/open-southeners/tusk-php/internal/analyzer"
	"github.com/open-southeners/tusk-php/internal/completion"
	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/hover"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// corpusEntry holds a collected PHP file with its URI and source.
type corpusEntry struct {
	path   string // absolute filesystem path
	uri    string // file:// URI used for indexing
	source string // raw PHP source
}

// testdataRoot returns the absolute path to the testdata directory.
func testdataRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// collectCorpus walks testdata/ and, if present, testdata/corpus/_repos/ for
// every .php file. Absence of the _repos directory is normal (CI only) and
// never causes an error.
func collectCorpus() ([]corpusEntry, error) {
	roots := []string{testdataRoot()}

	reposDir := filepath.Join(testdataRoot(), "corpus", "_repos")
	if info, err := os.Stat(reposDir); err == nil && info.IsDir() {
		roots = append(roots, reposDir)
	}

	var entries []corpusEntry
	seen := make(map[string]bool)

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Skip unreadable paths gracefully.
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".php") {
				return nil
			}
			if seen[path] {
				return nil
			}
			seen[path] = true

			src, readErr := os.ReadFile(path)
			if readErr != nil {
				// Skip unreadable files; they are not a harness failure.
				return nil
			}
			uri := pathToURI(path)
			entries = append(entries, corpusEntry{
				path:   path,
				uri:    uri,
				source: string(src),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	return entries, nil
}

// pathToURI converts an absolute file-system path to a file:// URI.
func pathToURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		// Windows drive letter: /C:/...
		path = "/" + path
	}
	return "file://" + path
}

// buildIndex constructs a symbols.Index over the provided corpus entries.
func buildIndex(entries []corpusEntry) *symbols.Index {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for _, e := range entries {
		idx.IndexFile(e.uri, e.source)
	}
	return idx
}

// providers bundles all the LSP providers exercised by the harness.
type providers struct {
	ana  *analyzer.Analyzer
	hov  *hover.Provider
	comp *completion.Provider
}

// newProviders creates the provider set from a shared index.
// A fresh, empty ContainerAnalyzer is used (no service provider analysis).
func newProviders(idx *symbols.Index) *providers {
	ca := container.NewContainerAnalyzer(idx, "", "none")
	a := analyzer.NewAnalyzer(idx, ca)
	h := hover.NewProvider(idx, ca, "none")
	c := completion.NewProvider(idx, ca, "none")
	return &providers{ana: a, hov: h, comp: c}
}

// anchorPoint represents a position in a file where we exercise operations.
type anchorPoint struct {
	line int
	col  int
	kind anchorKind
}

type anchorKind int

const (
	anchorArrow       anchorKind = iota // ->
	anchorNullsafe                      // ?->
	anchorDoubleColon                   // ::
	anchorNew                           // new keyword
	anchorClassName                     // class-name identifier
	anchorMethodName                    // method/function name
	anchorUseImport                     // use import short name
)

// extractAnchors scans the token stream of a parse result and returns
// deterministic anchor positions for every operator and name token of interest.
func extractAnchors(source string) []anchorPoint {
	p := parser.New()
	result := p.Parse(source)
	if result == nil {
		return nil
	}

	lines := strings.Split(source, "\n")
	lineCount := len(lines)

	var anchors []anchorPoint

	for i := 0; i < len(result.Tokens); i++ {
		tok := result.Tokens[i]
		line := tok.Line
		if line < 0 || line >= lineCount {
			continue
		}

		switch tok.Kind {
		case parser.TokenArrow:
			// Position cursor just after ->, on the member name.
			if i+1 < len(result.Tokens) {
				next := result.Tokens[i+1]
				if next.Kind == parser.TokenIdentifier || next.Kind == parser.TokenVariable {
					anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorArrow})
				}
			}

		case parser.TokenQuestion:
			// Nullsafe: ?-> — peek for -> immediately after.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenArrow {
				if i+2 < len(result.Tokens) {
					next := result.Tokens[i+2]
					if next.Kind == parser.TokenIdentifier {
						anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorNullsafe})
					}
				}
			}

		case parser.TokenDoubleColon:
			// ::member.
			if i+1 < len(result.Tokens) {
				next := result.Tokens[i+1]
				if next.Kind == parser.TokenIdentifier || next.Kind == parser.TokenVariable {
					anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorDoubleColon})
				}
			}

		case parser.TokenNew:
			// new ClassName — position on the class name.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenIdentifier {
				next := result.Tokens[i+1]
				anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorNew})
			}

		case parser.TokenUse:
			// use Foo\Bar as Baz; — collect the alias (last segment or explicit alias).
			// Skip to the end of the use path, then record the last identifier.
			j := i + 1
			var lastName *parser.Token
			for j < len(result.Tokens) {
				tt := result.Tokens[j]
				if tt.Kind == parser.TokenSemicolon || tt.Kind == parser.TokenEOF {
					break
				}
				if tt.Kind == parser.TokenIdentifier {
					lastName = &result.Tokens[j]
				}
				j++
			}
			if lastName != nil {
				anchors = append(anchors, anchorPoint{line: lastName.Line, col: lastName.Column, kind: anchorUseImport})
			}

		case parser.TokenClass, parser.TokenInterface, parser.TokenTrait, parser.TokenEnum:
			// Structural declaration: record the name token.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenIdentifier {
				next := result.Tokens[i+1]
				anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorClassName})
			}

		case parser.TokenFunction:
			// Function/method name.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenIdentifier {
				next := result.Tokens[i+1]
				anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorMethodName})
			}
		}
	}

	return anchors
}

// operationResults holds the outputs of exercising all operations at one anchor.
// FindReferences is excluded from the per-anchor loop because its implementation
// scans every indexed file from disk (O(corpus) per call), making it too slow to
// run at every anchor point across the full corpus. It is still crash-tested via
// checkFindReferencesSafe separately.
type operationResults struct {
	definition  *protocol.Location
	docSymbols  []protocol.DocumentSymbol
	hover       *protocol.Hover
	completions []protocol.CompletionItem
	sigHelp     *protocol.SignatureHelp
}

// runOperations exercises provider operations at anchor pos, catching panics.
// The kind parameter controls which operations are attempted: hover and completion
// run on use-import and member-access anchors; definition, document symbols, and
// signature help run on all anchors.
func runOperations(uri, source string, pos protocol.Position, kind anchorKind, prov *providers) (res operationResults, panicErr error) {
	defer func() {
		if r := recover(); r != nil {
			panicErr = fmt.Errorf("panic at %s line %d col %d: %v", uri, pos.Line, pos.Character, r)
		}
	}()

	res.definition = prov.ana.FindDefinition(uri, source, pos)
	res.docSymbols = prov.ana.GetDocumentSymbols(uri, source)
	res.sigHelp = prov.ana.GetSignatureHelp(uri, source, pos)

	// Hover and completion are exercised on use-import anchors and on all
	// member-access anchors (->, ?->, ::). The chain-resolver re-entrancy guard
	// (C1, fixed in internal/resolve) prevents the fatal stack overflow that
	// previously occurred when resolveAccessChain ↔ ResolveVariableType cycled
	// via ChainResolver on self/mutually-referential assignments.
	if kind == anchorUseImport || kind == anchorArrow || kind == anchorNullsafe || kind == anchorDoubleColon {
		res.hover = prov.hov.GetHover(uri, source, pos)
		res.completions = prov.comp.GetCompletions(uri, source, pos)
	}
	return res, nil
}

// isProjectEntry returns true if the corpus entry comes from the small in-repo
// project testdata (testdata/project/), not the larger laravel/symfony fixtures.
func isProjectEntry(e corpusEntry) bool {
	return strings.Contains(e.path, "/testdata/project/") ||
		strings.Contains(e.path, `\testdata\project\`)
}

// maxAnchorsPerFile limits the number of anchor points exercised per file to
// keep the operation phase fast across a large corpus. Anchors are sampled
// deterministically (every N-th anchor).
const maxAnchorsPerFile = 5

// sampleAnchors returns a deterministic subset of at most maxAnchorsPerFile
// anchors. If there are fewer than maxAnchorsPerFile anchors, all are returned.
func sampleAnchors(anchors []anchorPoint) []anchorPoint {
	if len(anchors) <= maxAnchorsPerFile {
		return anchors
	}
	step := len(anchors) / maxAnchorsPerFile
	if step < 1 {
		step = 1
	}
	out := make([]anchorPoint, 0, maxAnchorsPerFile)
	for i := 0; i < len(anchors) && len(out) < maxAnchorsPerFile; i += step {
		out = append(out, anchors[i])
	}
	return out
}
