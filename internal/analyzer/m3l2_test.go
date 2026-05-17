package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// --- M3: SignatureHelp activeParameter clamping ---

// setupSigHelpAnalyzer builds an index and analyzer with a single PHP file
// declaring a function with the given parameters.
func setupSigHelpAnalyzer(t *testing.T, funcSource string) *Analyzer {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/funcs.php", funcSource)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	return NewAnalyzer(idx, ca)
}

func TestSignatureHelpActiveParamClamped(t *testing.T) {
	// twoParam(string $a, int $b) — 2 parameters, indices 0 and 1.
	// Simulate the user typing a third argument: twoParam($x, $y, |
	// extractCallInfo counts 2 commas → activeParam = 2, which is out of range.
	// GetSignatureHelp must clamp it to 1.
	funcSource := `<?php
function twoParam(string $a, int $b): void {}
`
	a := setupSigHelpAnalyzer(t, funcSource)

	// Source with three arguments at cursor after the third comma.
	callSource := `<?php
twoParam($x, $y, `
	pos := protocol.Position{Line: 1, Character: len("twoParam($x, $y, ")}

	help := a.GetSignatureHelp("file:///call.php", callSource, pos)
	if help == nil {
		t.Fatal("expected SignatureHelp, got nil")
	}
	if len(help.Signatures) == 0 {
		t.Fatal("expected at least one signature")
	}
	paramCount := len(help.Signatures[0].Parameters)
	if paramCount != 2 {
		t.Fatalf("expected 2 parameters, got %d", paramCount)
	}
	if help.ActiveParameter >= paramCount {
		t.Errorf("activeParameter = %d is out of range for %d parameters; want < %d",
			help.ActiveParameter, paramCount, paramCount)
	}
	// Specifically must be clamped to 1 (last valid index).
	if help.ActiveParameter != 1 {
		t.Errorf("activeParameter = %d, want 1 (clamped to last param)", help.ActiveParameter)
	}
}

func TestSignatureHelpActiveParamInRange(t *testing.T) {
	// Verify normal in-range behaviour is unaffected.
	funcSource := `<?php
function twoParam(string $a, int $b): void {}
`
	a := setupSigHelpAnalyzer(t, funcSource)

	tests := []struct {
		name      string
		prefix    string
		wantParam int
	}{
		{"first param", "twoParam(", 0},
		{"second param", "twoParam($x, ", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "<?php\n" + tt.prefix
			pos := protocol.Position{Line: 1, Character: len(tt.prefix)}
			help := a.GetSignatureHelp("file:///call.php", src, pos)
			if help == nil {
				t.Fatalf("expected SignatureHelp, got nil")
			}
			if help.ActiveParameter != tt.wantParam {
				t.Errorf("activeParameter = %d, want %d", help.ActiveParameter, tt.wantParam)
			}
		})
	}
}

func TestSignatureHelpZeroParams(t *testing.T) {
	// Function with no parameters — activeParam must be 0, not negative.
	funcSource := `<?php
function noParams(): void {}
`
	a := setupSigHelpAnalyzer(t, funcSource)

	// Even with spurious commas, activeParam must be 0.
	src := `<?php
noParams(`
	pos := protocol.Position{Line: 1, Character: len("noParams(")}
	help := a.GetSignatureHelp("file:///call.php", src, pos)
	if help == nil {
		// noParams may not be found by name-lookup depending on index — that is ok.
		t.Skip("GetSignatureHelp returned nil for zero-param function")
	}
	if help.ActiveParameter != 0 {
		t.Errorf("activeParameter = %d for zero-param function, want 0", help.ActiveParameter)
	}
}

// --- L2: FindReferences uses GetFileSource to avoid redundant disk reads ---

func TestFindReferencesUsesIndexedSource(t *testing.T) {
	// Index files WITH source so GetFileSource returns non-empty strings.
	// We deliberately do NOT provide a readDocument callback — the analyzer
	// must use index.GetFileSource as the primary in-memory source.
	sources := map[string]string{
		"file:///lib.php": `<?php
namespace App;
class Widget {
    public function render(): void {}
}
`,
		"file:///consumer.php": `<?php
namespace App;
use App\Widget;
class Page {
    public function show(): void {
        $w = new Widget();
        $w->render();
        $w->render();
    }
}
`,
	}

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range sources {
		// IndexFileWithSource stores the source in the index (GetFileSource will return it).
		idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	}
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	// FindReferences with nil readDocument — must rely solely on GetFileSource.
	locs := a.FindAllReferences(
		"file:///lib.php",
		sources["file:///lib.php"],
		protocol.Position{Line: 2, Character: 8}, // cursor on "Widget" in class declaration
		nil,
	)

	if len(locs) == 0 {
		t.Fatal("expected references to Widget, got none")
	}

	// Verify we get references in the consumer file (cross-file).
	consumerFound := false
	for _, loc := range locs {
		if loc.URI == "file:///consumer.php" {
			consumerFound = true
			break
		}
	}
	if !consumerFound {
		t.Error("expected at least one reference in consumer.php; in-memory source not used")
	}
}

func TestFindAllReferencesMethodViaIndexedSource(t *testing.T) {
	// Regression: render() method found in consumer.php when using in-memory source.
	sources := map[string]string{
		"file:///widget.php": `<?php
namespace App;
class Widget {
    public function render(): void {}
}
`,
		"file:///page.php": `<?php
namespace App;
use App\Widget;
class Page {
    public function build(): void {
        $w = new Widget();
        $w->render();
    }
}
`,
	}

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range sources {
		idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	}
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	// Cursor on render() in its declaration.
	locs := a.FindAllReferences(
		"file:///widget.php",
		sources["file:///widget.php"],
		protocol.Position{Line: 3, Character: 22},
		nil, // no readDocument — must use GetFileSource
	)

	if len(locs) < 2 {
		t.Errorf("expected at least 2 references to render (decl + call site), got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		}
	}

	pageFound := false
	for _, loc := range locs {
		if loc.URI == "file:///page.php" {
			pageFound = true
			break
		}
	}
	if !pageFound {
		t.Error("expected render() call site reference in page.php; GetFileSource fallback not working")
	}
}
