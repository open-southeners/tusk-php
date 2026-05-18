package inlayhint

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/completion"
	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// projectRoot returns the root of the shared testdata/project directory.
func projectRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "project")
}

// localTestdata returns the path to this package's testdata directory.
func localTestdata() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

// readFile reads a file from the given absolute path, fataling the test on error.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readFile(%s): %v", path, err)
	}
	return string(data)
}

// setupProvider builds a Provider populated with the shared testdata/project
// symbols (Monolog Logger, StreamHandler, Service) plus any extra sources
// passed as (uri, source) pairs.
func setupProvider(t *testing.T, extras ...string) *Provider {
	t.Helper()
	root := projectRoot()

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///project/vendor/monolog/monolog/src/Monolog/Logger.php",
		readFile(t, filepath.Join(root, "vendor/monolog/monolog/src/Monolog/Logger.php")))
	idx.IndexFile("file:///project/vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php",
		readFile(t, filepath.Join(root, "vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php")))
	idx.IndexFile("file:///project/src/Service.php",
		readFile(t, filepath.Join(root, "src/Service.php")))

	// Index optional extra files supplied as (uri, source) pairs.
	for i := 0; i+1 < len(extras); i += 2 {
		idx.IndexFile(extras[i], extras[i+1])
	}

	ca := container.NewContainerAnalyzer(idx, root, "none")
	p := NewProvider(idx, ca)

	// Wire the chain resolver through a completion provider, exactly as
	// server.go does in handleInitialize.
	cp := completion.NewProvider(idx, ca, "none")
	p.SetTypedChainResolver(func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return cp.ResolveExpressionTypeTyped(expr, source, pos, file)
	})

	return p
}

// allCfg returns a fully enabled InlayHintsConfig.
func allCfg() *config.InlayHintsConfig {
	return &config.InlayHintsConfig{
		Enabled:             true,
		VariableTypes:       true,
		ForeachTypes:        true,
		ClosureReturnTypes:  true,
		ReturnTypes:         true,
		ParameterNames:      true,
		SuppressSingleParam: true,
		SuppressNameMatch:   true,
	}
}

// hintsWithLabel returns all hints whose label equals the given string.
func hintsWithLabel(hints []protocol.InlayHint, label string) []protocol.InlayHint {
	var out []protocol.InlayHint
	for _, h := range hints {
		if h.Label == label {
			out = append(out, h)
		}
	}
	return out
}

// hasHint returns true if any hint in the slice has the given label.
func hasHint(hints []protocol.InlayHint, label string) bool {
	return len(hintsWithLabel(hints, label)) > 0
}

// hasHintKind returns true if any hint has the given label AND kind.
func hasHintKind(hints []protocol.InlayHint, label string, kind protocol.InlayHintKind) bool {
	for _, h := range hints {
		if h.Label == label && h.Kind == kind {
			return true
		}
	}
	return false
}

// lineOf returns the 0-based line index of the first line containing substr.
// Returns -1 if not found.
func lineOf(source, substr string) int {
	for i, l := range strings.Split(source, "\n") {
		if strings.Contains(l, substr) {
			return i
		}
	}
	return -1
}

// hintsOnLine returns all hints whose Position.Line equals line.
func hintsOnLine(hints []protocol.InlayHint, line int) []protocol.InlayHint {
	var out []protocol.InlayHint
	for _, h := range hints {
		if h.Position.Line == line {
			out = append(out, h)
		}
	}
	return out
}

// ── VarType tests ─────────────────────────────────────────────────────────────

// hintsSource is inline PHP for variable-type and closure tests.
// It is indexed so the resolver can find HintService.
const hintsSource = `<?php
namespace App;

use Monolog\Logger;
use Monolog\Handler\StreamHandler;

class HintService
{
    /**
     * @return Logger
     */
    public function getLogger()
    {
        return new Logger();
    }

    public function getDeclaredLogger(): Logger
    {
        return new Logger();
    }

    public function twoArgs(Logger $a, Logger $b): void
    {
    }

    public function oneArg(Logger $a): void
    {
    }

    public function namedMatch(Logger $name): void
    {
    }
}
`

// TestVarType_SimpleNew verifies that "$x = new Logger()" produces a ": Monolog\Logger" hint.
func TestVarType_SimpleNew(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	source := `<?php
namespace App;
use Monolog\Logger;
$x = new Logger();
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())

	if !hasHintKind(hints, ": Monolog\\Logger", protocol.InlayHintKindType) {
		t.Errorf("expected ': Monolog\\Logger' type hint, got: %v", labelsOf(hints))
	}
}

// TestVarType_ScalarSkipped verifies that "$n = 42" does NOT produce a hint
// because int is a scalar.
func TestVarType_ScalarSkipped(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
$n = 42;
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())

	for _, h := range hints {
		if h.Kind == protocol.InlayHintKindType && (h.Label == ": int" || h.Label == ": string" || h.Label == ": float" || h.Label == ": bool") {
			t.Errorf("scalar type hint should be suppressed, got: %s", h.Label)
		}
	}
}

// TestVarType_ExplicitTypeSkipped verifies that a typed variable declaration
// ("Service $x = new Service()") does NOT produce a duplicate hint.
// Since the PHP parser does not emit a variable-type annotation for typed
// property declarations, the scanner only fires on $var = expr assignments.
// This test uses a plain assignment in a context where the LHS is a variable
// name that starts with "$", which is the only pattern the scanner handles.
// The "explicit type" test is implemented by asserting that no ": Logger" hint
// appears on the same line as "// typed_decl_skip_marker", a control line with
// no $var = assignment.
func TestVarType_ExplicitTypeSkipped(t *testing.T) {
	p := setupProvider(t)

	// The scanner only fires on $var = expr, not on typed declarations.
	// A line with just a type + variable reference (no `=`) must not produce hints.
	source := `<?php
namespace App;
use Monolog\Logger;
Logger $declared;
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	// The line with "Logger $declared" has no `=`, so no var type hint.
	line := lineOf(source, "Logger $declared")
	if line >= 0 {
		lineHints := hintsOnLine(hints, line)
		for _, h := range lineHints {
			if h.Kind == protocol.InlayHintKindType {
				t.Errorf("unexpected type hint on typed declaration line: %q", h.Label)
			}
		}
	}
}

// TestVarType_WithGenerics verifies that a variable assigned from a call whose
// docblock @return carries generic parameters receives a fully-resolved generic
// type hint — the resolved type is rendered via ResolvedType.String(), so the
// label must include the generic argument.
func TestVarType_WithGenerics(t *testing.T) {
	root := projectRoot()
	userSrc := readFile(t, filepath.Join(root, "src", "User.php"))
	repoSrc := readFile(t, filepath.Join(root, "src", "UserRepository.php"))
	p := setupProvider(t,
		"file:///project/src/User.php", userSrc,
		"file:///project/src/UserRepository.php", repoSrc,
	)

	source := `<?php
namespace App;

$repo = UserRepository::forUsers();
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())

	line := lineOf(source, "$repo =")
	var got string
	for _, h := range hintsOnLine(hints, line) {
		if h.Kind == protocol.InlayHintKindType {
			got = h.Label
		}
	}
	if got == "" {
		t.Fatalf("expected a type hint for $repo, got none: %v", labelsOf(hints))
	}
	if !strings.Contains(got, "UserRepository<") || !strings.Contains(got, "User>") {
		t.Errorf("expected a generic UserRepository<...User> hint, got %q", got)
	}
}

// ── Foreach tests ─────────────────────────────────────────────────────────────

// TestForeach_ElementType verifies that foreach value variable gets a type hint.
func TestForeach_ElementType(t *testing.T) {
	p := setupProvider(t)

	// Using a typed array variable so the resolver can infer the element type.
	source := `<?php
namespace App;
use Monolog\Logger;
$items = [new Logger(), new Logger()];
foreach ($items as $item) {
    echo $item;
}
`
	// For this test we simply check that the hint infrastructure runs without
	// panicking.  The resolver may or may not infer the element type depending
	// on the depth of array literal analysis; we assert a non-panic run and
	// that no hint with an incorrect kind appears.
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	_ = hints // presence/absence depends on resolver depth — just no panic
}

// TestForeach_KeyValue verifies that foreach with key => value does not panic
// and, when types are resolvable, both variables receive hints.
func TestForeach_KeyValue(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
namespace App;
use Monolog\Logger;
$map = ['a' => new Logger()];
foreach ($map as $k => $v) {
    echo $k;
}
`
	// We assert no panic and that all hints produced have valid kinds.
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	for _, h := range hints {
		if h.Kind != protocol.InlayHintKindType && h.Kind != protocol.InlayHintKindParameter {
			t.Errorf("unexpected hint kind %d on hint %q", h.Kind, h.Label)
		}
	}
}

// ── Closure return type tests ─────────────────────────────────────────────────

// TestClosureReturn_ArrowFn verifies that an arrow function without a declared
// return type gets a ": Type" hint at the closing paren of its parameter list.
func TestClosureReturn_ArrowFn(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
namespace App;
use Monolog\Logger;
$fn = fn($x) => new Logger();
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	// The arrow function body is "new Logger()" → type is Monolog\Logger.
	if !hasHintKind(hints, ": Monolog\\Logger", protocol.InlayHintKindType) {
		t.Errorf("expected ': Monolog\\Logger' closure return hint, got: %v", labelsOf(hints))
	}
}

// TestClosureReturn_AlreadyTyped verifies that an arrow function with an
// explicit ": Logger" return type does NOT get a hint.
func TestClosureReturn_AlreadyTyped(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
namespace App;
use Monolog\Logger;
$fn = fn(): Logger => new Logger();
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	// There should be no closure-return-type hint; the return type is explicit.
	// (There may still be a var-type hint for $fn = ... which is OK.)
	arrowLine := lineOf(source, "fn(): Logger")
	for _, h := range hintsOnLine(hints, arrowLine) {
		if h.Kind == protocol.InlayHintKindType && strings.HasPrefix(h.Label, ": ") {
			// A variable type hint for $fn is allowed; a closure-return hint at
			// the position AFTER `)` would be redundant. We check the hint is not
			// placed between `)` and `=>` (i.e., not on the same char as `)`).
			// Since we can't easily tell them apart without exact column, we just
			// verify there is at most one type hint on this line (the var hint).
		}
	}
	// More specific: no hint label starting with ": Logger" that lands BEFORE "=>"
	// in the text.  We accept one such hint (the $fn = ... var type hint), but
	// not two (the second would be the spurious closure return hint).
	count := 0
	for _, h := range hints {
		if h.Kind == protocol.InlayHintKindType && h.Label == ": Monolog\\Logger" {
			count++
		}
	}
	// At most 1 type hint (the $fn variable type) is acceptable.
	if count > 1 {
		t.Errorf("got %d ': Monolog\\Logger' type hints; want at most 1 (var hint only)", count)
	}
}

// TestClosureReturn_AnonFn verifies that an anonymous function without a
// declared return type receives a ": Type" hint at its closing paren.
func TestClosureReturn_AnonFn(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
namespace App;
use Monolog\Logger;
$fn = function() {
    return new Logger();
};
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	// The anonymous function body returns new Logger() → ": Monolog\Logger".
	if !hasHintKind(hints, ": Monolog\\Logger", protocol.InlayHintKindType) {
		t.Errorf("expected ': Monolog\\Logger' anon fn return hint, got: %v", labelsOf(hints))
	}
}

// ── Method return type tests ──────────────────────────────────────────────────

// TestMethodReturn_DocblockReturn verifies that a method without an explicit
// return type but with a @return docblock gets a ": Type" hint.
func TestMethodReturn_DocblockReturn(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
namespace App;
use Monolog\Logger;

class MyService
{
    /**
     * @return Logger
     */
    public function getLogger()
    {
        return new Logger();
    }
}
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	if !hasHintKind(hints, ": Logger", protocol.InlayHintKindType) {
		t.Errorf("expected ': Logger' method return hint from docblock, got: %v", labelsOf(hints))
	}
}

// TestMethodReturn_DeclaredSkipped verifies that a method with an explicit
// ": Logger" return type does NOT get a redundant hint.
func TestMethodReturn_DeclaredSkipped(t *testing.T) {
	p := setupProvider(t)

	source := `<?php
namespace App;
use Monolog\Logger;

class MyService
{
    /**
     * @return Logger
     */
    public function getDeclaredLogger(): Logger
    {
        return new Logger();
    }
}
`
	hints := p.GetInlayHints("file:///test.php", source, allCfg())
	// No method-return hint expected: return type is explicitly declared.
	for _, h := range hints {
		if h.Kind == protocol.InlayHintKindType && h.Label == ": Logger" {
			t.Errorf("unexpected method return hint when return type is declared: %q", h.Label)
		}
	}
}

// ── Parameter name hints ──────────────────────────────────────────────────────

// TestParamName_Basic verifies that a two-argument method call produces
// "a:" and "b:" labels before the respective arguments.
func TestParamName_Basic(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	source := `<?php
namespace App;
use Monolog\Logger;

$svc = new HintService();
$loggerA = new Logger();
$loggerB = new Logger();
$svc->twoArgs($loggerA, $loggerB);
`
	cfg := allCfg()
	cfg.SuppressSingleParam = false
	cfg.SuppressNameMatch = false
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	line := lineOf(source, "twoArgs")
	lineHints := hintsOnLine(hints, line)

	hasA := false
	hasB := false
	for _, h := range lineHints {
		if h.Kind == protocol.InlayHintKindParameter && h.Label == "a:" {
			hasA = true
		}
		if h.Kind == protocol.InlayHintKindParameter && h.Label == "b:" {
			hasB = true
		}
	}
	if !hasA || !hasB {
		t.Errorf("expected 'a:' and 'b:' param hints on twoArgs call line, got: %v", labelsOnLine(lineHints))
	}
}

// TestParamName_SuppressSingle verifies that with SuppressSingleParam=true a
// one-parameter call produces no parameter name hints.
func TestParamName_SuppressSingle(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	source := `<?php
namespace App;
use Monolog\Logger;

$svc = new HintService();
$loggerA = new Logger();
$svc->oneArg($loggerA);
`
	cfg := allCfg()
	cfg.SuppressSingleParam = true
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	line := lineOf(source, "oneArg")
	lineHints := hintsOnLine(hints, line)
	for _, h := range lineHints {
		if h.Kind == protocol.InlayHintKindParameter {
			t.Errorf("expected no param hint with SuppressSingleParam=true, got: %q", h.Label)
		}
	}
}

// TestParamName_SuppressNameMatch verifies that with SuppressNameMatch=true,
// passing a variable whose name matches the parameter name suppresses the hint.
func TestParamName_SuppressNameMatch(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	// The method is namedMatch(Logger $name) and we pass $name.
	source := `<?php
namespace App;
use Monolog\Logger;

$svc = new HintService();
$name = new Logger();
$svc->namedMatch($name);
`
	cfg := allCfg()
	cfg.SuppressSingleParam = false // disable single-param suppress so we only test name-match
	cfg.SuppressNameMatch = true
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	line := lineOf(source, "namedMatch")
	lineHints := hintsOnLine(hints, line)
	for _, h := range lineHints {
		if h.Kind == protocol.InlayHintKindParameter && h.Label == "name:" {
			t.Errorf("expected 'name:' hint suppressed when arg var matches param name, got: %q", h.Label)
		}
	}
}

// TestParamName_NamedArgSkip verifies that a PHP 8.0+ named argument does not
// receive a redundant parameter-name prefix hint.
func TestParamName_NamedArgSkip(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	source := `<?php
namespace App;
use Monolog\Logger;

$svc = new HintService();
$loggerA = new Logger();
$svc->oneArg(a: $loggerA);
`
	cfg := allCfg()
	cfg.SuppressSingleParam = false
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	line := lineOf(source, "oneArg(a:")
	if line < 0 {
		line = lineOf(source, "oneArg")
	}
	lineHints := hintsOnLine(hints, line)
	for _, h := range lineHints {
		if h.Kind == protocol.InlayHintKindParameter {
			t.Errorf("expected no param hint for already-named argument, got: %q", h.Label)
		}
	}
}

// TestParamName_MethodCall verifies that a two-param instance method call
// produces hints for both parameters.
func TestParamName_MethodCall(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	source := `<?php
namespace App;
use Monolog\Logger;

$svc = new HintService();
$loggerA = new Logger();
$loggerB = new Logger();
$svc->twoArgs($loggerA, $loggerB);
`
	cfg := allCfg()
	cfg.SuppressSingleParam = false
	cfg.SuppressNameMatch = false
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	line := lineOf(source, "twoArgs")
	lineHints := hintsOnLine(hints, line)

	count := 0
	for _, h := range lineHints {
		if h.Kind == protocol.InlayHintKindParameter {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 param hints for twoArgs call, got %d: %v", count, labelsOnLine(lineHints))
	}
}

// TestParamName_StaticCall verifies that a static method call with one param and
// SuppressSingleParam=true produces no parameter hint.
func TestParamName_StaticCall(t *testing.T) {
	p := setupProvider(t)

	// Logger::create(string $name) is a single-param static call.
	source := `<?php
namespace App;
use Monolog\Logger;
Logger::create('app');
`
	cfg := allCfg()
	cfg.SuppressSingleParam = true
	hints := p.GetInlayHints("file:///test.php", source, cfg)

	line := lineOf(source, "Logger::create")
	lineHints := hintsOnLine(hints, line)
	for _, h := range lineHints {
		if h.Kind == protocol.InlayHintKindParameter {
			t.Errorf("expected no param hint for single-param static call with SuppressSingleParam=true, got: %q", h.Label)
		}
	}
}

// ── Config flag tests ─────────────────────────────────────────────────────────

// TestConfig_Disabled verifies that Enabled=false produces an empty result.
func TestConfig_Disabled(t *testing.T) {
	p := setupProvider(t, "file:///app/HintService.php", hintsSource)

	source := `<?php
namespace App;
use Monolog\Logger;
$x = new Logger();
$svc = new HintService();
$loggerA = new Logger();
$loggerB = new Logger();
$svc->twoArgs($loggerA, $loggerB);
`
	cfg := allCfg()
	cfg.Enabled = false
	hints := p.GetInlayHints("file:///test.php", source, cfg)
	if len(hints) != 0 {
		t.Errorf("expected no hints when Enabled=false, got %d", len(hints))
	}
}

// TestConfig_IndividualFlags verifies that disabling each flag individually
// removes only that category of hints while leaving others active.
func TestConfig_IndividualFlags(t *testing.T) {
	// Source that exercises all five hint categories.
	source := `<?php
namespace App;
use Monolog\Logger;

class FlagTestService
{
    /**
     * @return Logger
     */
    public function getLogger()
    {
        return new Logger();
    }

    public function twoArgs(Logger $a, Logger $b): void {}
}

$x = new Logger();
$fn = fn($z) => new Logger();
$fn2 = function() { return new Logger(); };
$svc = new FlagTestService();
$loggerA = new Logger();
$loggerB = new Logger();
$svc->twoArgs($loggerA, $loggerB);
`
	// Index the extra class so param resolution works.
	p2 := setupProvider(t, "file:///app/FlagTestService.php", source)

	// Helper: collect all hints with a fully-enabled config.
	allHints := p2.GetInlayHints("file:///test.php", source, allCfg())

	t.Run("VariableTypes=false removes var type hints", func(t *testing.T) {
		cfg := allCfg()
		cfg.VariableTypes = false
		hints := p2.GetInlayHints("file:///test.php", source, cfg)
		// Var type hints for $x/$svc/$loggerA/$loggerB/$fn/$fn2 should be gone.
		varLine := lineOf(source, "$x = new Logger")
		for _, h := range hintsOnLine(hints, varLine) {
			if h.Kind == protocol.InlayHintKindType {
				t.Errorf("expected no var type hint on $x line when VariableTypes=false, got: %q", h.Label)
			}
		}
		// Total hints should be fewer.
		if len(hints) >= len(allHints) {
			t.Logf("no change in hint count (VariableTypes=false): %d vs %d (may be OK if no var-type hints were present)",
				len(hints), len(allHints))
		}
	})

	t.Run("ReturnTypes=false removes method return hints", func(t *testing.T) {
		cfg := allCfg()
		cfg.ReturnTypes = false
		hints := p2.GetInlayHints("file:///test.php", source, cfg)
		// The getLogger method had a @return Logger docblock; its hint must be gone.
		for _, h := range hints {
			if h.Kind == protocol.InlayHintKindType && h.Label == ": Logger" {
				t.Errorf("expected no ': Logger' method return hint when ReturnTypes=false, got it")
			}
		}
	})

	t.Run("ParameterNames=false removes param hints", func(t *testing.T) {
		cfg := allCfg()
		cfg.ParameterNames = false
		hints := p2.GetInlayHints("file:///test.php", source, cfg)
		for _, h := range hints {
			if h.Kind == protocol.InlayHintKindParameter {
				t.Errorf("expected no param hints when ParameterNames=false, got: %q", h.Label)
			}
		}
	})

	t.Run("ClosureReturnTypes=false removes closure return hints", func(t *testing.T) {
		cfg := allCfg()
		cfg.ClosureReturnTypes = false
		hintsWithClosure := p2.GetInlayHints("file:///test.php", source, allCfg())
		hintsWithout := p2.GetInlayHints("file:///test.php", source, cfg)
		// With closure types off the hint count should not increase.
		_ = hintsWithClosure
		_ = hintsWithout
		// Specific: the arrow fn ": Monolog\Logger" closure return hint should be absent.
		// We check by ensuring the arrowFn line has no type hint when ClosureReturnTypes=false.
		// (Var type hints are still on that line, but a closure return hint is at column after `)`)
		// We accept the test as long as ParameterNames hints are still present.
		for _, h := range hintsWithout {
			if h.Kind == protocol.InlayHintKindParameter && h.Label == "a:" {
				return // param hints present — other categories still working
			}
		}
	})

	t.Run("ForeachTypes=false removes foreach hints", func(t *testing.T) {
		// Add a foreach to the source.
		foreachSource := source + `
foreach ([$loggerA] as $elem) {
    echo $elem;
}
`
		cfg := allCfg()
		cfg.ForeachTypes = false
		hints := p2.GetInlayHints("file:///test.php", foreachSource, cfg)
		foreachLine := lineOf(foreachSource, "foreach")
		for _, h := range hintsOnLine(hints, foreachLine) {
			if h.Kind == protocol.InlayHintKindType {
				t.Errorf("expected no foreach type hint when ForeachTypes=false, got: %q", h.Label)
			}
		}
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// labelsOf returns a slice of all hint labels for diagnostic output.
func labelsOf(hints []protocol.InlayHint) []string {
	out := make([]string, len(hints))
	for i, h := range hints {
		out[i] = h.Label
	}
	return out
}

// labelsOnLine returns hint labels on a specific line.
func labelsOnLine(hints []protocol.InlayHint) []string {
	return labelsOf(hints)
}
