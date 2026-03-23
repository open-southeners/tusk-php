package diagnostics

import (
	"io"
	"log"
	"testing"

	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func newTestProvider() *Provider {
	idx := symbols.NewIndex()
	logger := log.New(io.Discard, "", 0)
	cfg := config.DefaultConfig()
	// Disable external tools for unit tests
	f := false
	cfg.PHPStanEnabled = &f
	cfg.PintEnabled = &f
	return NewProvider(idx, "none", "/tmp", logger, cfg)
}

func TestStaticDeprecations(t *testing.T) {
	p := newTestProvider()
	source := `<?php
namespace App;
class Foo {
    public function bar(): void {
        $data = each($arr);
        $fn = create_function('$a', 'return $a;');
        $enc = utf8_encode($str);
    }
}
`
	diags := p.Analyze("file:///test.php", source)
	deprecations := filterBySource(diags, "php-lsp")
	deprecations = filterByCode(deprecations, "deprecated")
	if len(deprecations) != 3 {
		t.Fatalf("expected 3 deprecation diagnostics, got %d", len(deprecations))
	}
	for _, d := range deprecations {
		if d.Severity != protocol.DiagnosticSeverityWarning {
			t.Errorf("expected warning severity, got %d", d.Severity)
		}
		if d.Range.Start.Character == 0 && d.Range.End.Character == 0 {
			t.Error("expected non-zero column for deprecation")
		}
	}
}

func TestStaticUnusedImports(t *testing.T) {
	p := newTestProvider()
	source := `<?php
namespace App;
use Some\UnusedClass;
use Some\UsedClass;
class Foo {
    public function bar(UsedClass $x): void {}
}
`
	diags := p.Analyze("file:///test.php", source)
	unused := filterByCode(diags, "unused-import")
	if len(unused) != 1 {
		t.Fatalf("expected 1 unused import diagnostic, got %d", len(unused))
	}
	if unused[0].Severity != protocol.DiagnosticSeverityHint {
		t.Errorf("expected hint severity, got %d", unused[0].Severity)
	}
}

func TestStaticAbstractInConcrete(t *testing.T) {
	p := newTestProvider()
	source := `<?php
class Foo {
    abstract public function bar(): void;
}
`
	diags := p.Analyze("file:///test.php", source)
	abstracts := filterByCode(diags, "abstract-in-concrete")
	if len(abstracts) != 1 {
		t.Fatalf("expected 1 abstract-in-concrete diagnostic, got %d", len(abstracts))
	}
	if abstracts[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected error severity, got %d", abstracts[0].Severity)
	}
}

func TestToolResultsCaching(t *testing.T) {
	p := newTestProvider()
	uri := "file:///test.php"

	// Simulate cached tool results
	p.mu.Lock()
	p.toolResults[uri] = []protocol.Diagnostic{
		{
			Range:    protocol.Range{Start: protocol.Position{Line: 5, Character: 0}},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "phpstan",
			Message:  "Test error",
			Code:     "test.error",
		},
	}
	p.mu.Unlock()

	source := `<?php
class Foo {
    public function bar(): void {}
}
`
	diags := p.Analyze(uri, source)
	phpstanDiags := filterBySource(diags, "phpstan")
	if len(phpstanDiags) != 1 {
		t.Fatalf("expected 1 cached phpstan diagnostic, got %d", len(phpstanDiags))
	}
	if phpstanDiags[0].Message != "Test error" {
		t.Errorf("expected 'Test error', got %q", phpstanDiags[0].Message)
	}

	// Clear cache and verify it's gone
	p.ClearCache(uri)
	diags = p.Analyze(uri, source)
	phpstanDiags = filterBySource(diags, "phpstan")
	if len(phpstanDiags) != 0 {
		t.Errorf("expected 0 phpstan diagnostics after clear, got %d", len(phpstanDiags))
	}
}

func TestPHPStanOutputParsing(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"totals": {"errors": 0, "file_errors": 2},
		"files": {
			"/project/src/Service.php": {
				"errors": 2,
				"messages": [
					{
						"message": "Parameter #1 $message of method info() expects string, int given.",
						"line": 15,
						"ignorable": true,
						"identifier": "argument.type"
					},
					{
						"message": "Call to undefined method Foo::bar().",
						"line": 22,
						"ignorable": true,
						"identifier": "method.notFound"
					}
				]
			}
		}
	}`)

	diags := r.parseOutput(output, nil)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	// First diagnostic
	if diags[0].Range.Start.Line != 14 { // 15 → 14 (0-based)
		t.Errorf("expected line 14, got %d", diags[0].Range.Start.Line)
	}
	if diags[0].Source != "phpstan" {
		t.Errorf("expected source 'phpstan', got %q", diags[0].Source)
	}
	if diags[0].Code != "argument.type" {
		t.Errorf("expected code 'argument.type', got %q", diags[0].Code)
	}
	if diags[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected error severity, got %d", diags[0].Severity)
	}

	// Second diagnostic
	if diags[1].Range.Start.Line != 21 {
		t.Errorf("expected line 21, got %d", diags[1].Range.Start.Line)
	}
	if diags[1].Code != "method.notFound" {
		t.Errorf("expected code 'method.notFound', got %q", diags[1].Code)
	}
}

func TestPHPStanOutputWithPreamble(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`Note: Using configuration file phpstan.neon.
{"totals":{"errors":0,"file_errors":1},"files":{"/test.php":{"errors":1,"messages":[{"message":"Undefined variable: $x","line":3,"ignorable":true,"identifier":"variable.undefined"}]}}}`)

	diags := r.parseOutput(output, nil)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Range.Start.Line != 2 {
		t.Errorf("expected line 2, got %d", diags[0].Range.Start.Line)
	}
}

func TestPHPStanDiagnosticRange(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{"totals":{"errors":0,"file_errors":2},"files":{"/test.php":{"errors":2,"messages":[
		{"message":"Function clients not found.","line":5,"ignorable":true,"identifier":"function.notFound"},
		{"message":"Call to undefined method App\\Service::missing().","line":8,"ignorable":true,"identifier":"method.notFound"}
	]}}}`)

	lines := []string{
		"<?php",                                   // 0
		"",                                        // 1
		"namespace App\\Http\\Controllers;",       // 2
		"",                                        // 3
		"        clients();",                      // 4 (line 5 in PHPStan = 0-based 4)
		"",                                        // 5
		"        $svc = new Service();",           // 6
		"        $svc->missing();",                // 7 (line 8 in PHPStan = 0-based 7)
	}

	diags := r.parseOutput(output, lines)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	// "Function clients not found" → should highlight "clients" on line 4
	d0 := diags[0]
	if d0.Range.Start.Line != 4 {
		t.Errorf("d0: expected line 4, got %d", d0.Range.Start.Line)
	}
	// "clients" starts at col 8 in "        clients();"
	if d0.Range.Start.Character != 8 {
		t.Errorf("d0: expected start col 8, got %d", d0.Range.Start.Character)
	}
	if d0.Range.End.Character != 8+len("clients") {
		t.Errorf("d0: expected end col %d, got %d", 8+len("clients"), d0.Range.End.Character)
	}

	// "Call to undefined method App\Service::missing()" → should highlight "missing"
	d1 := diags[1]
	if d1.Range.Start.Line != 7 {
		t.Errorf("d1: expected line 7, got %d", d1.Range.Start.Line)
	}
	// "missing" appears after "$svc->" at col 14 in "        $svc->missing();"
	if d1.Range.Start.Character != 14 {
		t.Errorf("d1: expected start col 14 for 'missing', got %d", d1.Range.Start.Character)
	}
	if d1.Range.End.Character != 14+len("missing") {
		t.Errorf("d1: expected end col %d, got %d", 14+len("missing"), d1.Range.End.Character)
	}
}

func TestPHPStanDiagnosticRangeFallback(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{"totals":{"errors":0,"file_errors":1},"files":{"/test.php":{"errors":1,"messages":[
		{"message":"Some obscure error.","line":2,"ignorable":true,"identifier":"phpstan"}
	]}}}`)

	lines := []string{
		"<?php",                         // 0
		"    $x = something_weird();",   // 1 (line 2 in PHPStan = 0-based 1)
	}

	diags := r.parseOutput(output, lines)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	// No identifier found in message → should highlight trimmed line content
	d := diags[0]
	if d.Range.Start.Character != 4 { // skip 4 leading spaces
		t.Errorf("expected start col 4, got %d", d.Range.Start.Character)
	}
	if d.Range.End.Character != len("    $x = something_weird();") {
		t.Errorf("expected end col %d, got %d", len("    $x = something_weird();"), d.Range.End.Character)
	}
}

func TestPintDiffParsing(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"files": [
			{
				"name": "src/Service.php",
				"diff": "--- Original\n+++ New\n@@ -3,7 +3,6 @@\n namespace App;\n \n-use Illuminate\\Contracts\\Auth\\MustVerifyEmail;\n use Illuminate\\Database\\Eloquent\\Factories\\HasFactory;\n use Illuminate\\Foundation\\Auth\\User as Authenticatable;\n",
				"appliedFixers": ["no_unused_imports"]
			}
		]
	}`)

	diags := r.parseOutput(output)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	// The removed line is at original line 5 (hunk starts at 3, context +2)
	// @@ -3,7 +3,6 @@ → starts at original line 3
	// " namespace App;" → context line 3 → origLine becomes 4
	// "" → context line 4 → origLine becomes 5
	// "-use Illuminate..." → removed at origLine 5 → reports line 4 (0-based)
	if diags[0].Range.Start.Line != 4 {
		t.Errorf("expected line 4 (0-based), got %d", diags[0].Range.Start.Line)
	}
	if diags[0].Source != "pint" {
		t.Errorf("expected source 'pint', got %q", diags[0].Source)
	}
	if diags[0].Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("expected warning severity, got %d", diags[0].Severity)
	}
}

func TestPintMultipleHunks(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"files": [
			{
				"name": "src/Foo.php",
				"diff": "--- Original\n+++ New\n@@ -5,3 +5,3 @@\n class Foo\n-{\n+{\n@@ -10,3 +10,3 @@\n     public function bar()\n-    {\n+    {",
				"appliedFixers": ["braces"]
			}
		]
	}`)

	diags := r.parseOutput(output)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics (one per hunk), got %d", len(diags))
	}
	// First hunk: @@ -5 → origLine 5, context "class Foo" at 5→6, removed at 6 → line 5 (0-based)
	if diags[0].Range.Start.Line != 5 {
		t.Errorf("first diagnostic: expected line 5, got %d", diags[0].Range.Start.Line)
	}
	// Second hunk: @@ -10 → origLine 10, context at 10→11, removed at 11 → line 10 (0-based)
	if diags[1].Range.Start.Line != 10 {
		t.Errorf("second diagnostic: expected line 10, got %d", diags[1].Range.Start.Line)
	}
}

func TestPintNoFixers(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{"files": []}`)
	diags := r.parseOutput(output)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for clean output, got %d", len(diags))
	}
}

func TestPintWithoutDiff(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"files": [
			{
				"name": "src/Foo.php",
				"appliedFixers": ["no_unused_imports", "braces"]
			}
		]
	}`)

	diags := r.parseOutput(output)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics (one per fixer), got %d", len(diags))
	}
	// Without diff, all diagnostics should be at line 0
	for _, d := range diags {
		if d.Range.Start.Line != 0 {
			t.Errorf("expected line 0 for no-diff diagnostic, got %d", d.Range.Start.Line)
		}
	}
}

func TestRunnerAvailability(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	t.Run("disabled by config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		f := false
		cfg.PHPStanEnabled = &f
		cfg.PintEnabled = &f
		r1 := newPHPStanRunner("/nonexistent", cfg, logger)
		r2 := newPintRunner("/nonexistent", cfg, logger)
		if r1.available() {
			t.Error("phpstan should not be available when disabled")
		}
		if r2.available() {
			t.Error("pint should not be available when disabled")
		}
	})

	t.Run("unavailable binary", func(t *testing.T) {
		cfg := config.DefaultConfig()
		r1 := newPHPStanRunner("/nonexistent", cfg, logger)
		r2 := newPintRunner("/nonexistent", cfg, logger)
		if r1.available() {
			t.Error("phpstan should not be available with missing binary")
		}
		if r2.available() {
			t.Error("pint should not be available with missing binary")
		}
	})
}

func filterBySource(diags []protocol.Diagnostic, source string) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, d := range diags {
		if d.Source == source {
			out = append(out, d)
		}
	}
	return out
}

func filterByCode(diags []protocol.Diagnostic, code string) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, d := range diags {
		if d.Code == code {
			out = append(out, d)
		}
	}
	return out
}
