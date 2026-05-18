package resolve

// Regression tests for C1 — fatal stack overflow via infinite mutual recursion
// between ResolveVariableType / ResolveVariableTypeTyped and the ChainResolver /
// TypedChainResolver callbacks.
//
// The cycle (from CURRENT_ISSUES.md):
//   resolveExpressionType (hover/completion)
//     → resolveAccessChain
//       → Resolver.ResolveVariableType / ResolveVariableTypeTyped
//         → ChainResolver / TypedChainResolver  (calls back into resolveExpressionType)
//
// Inputs like "$x = $x->foo();" or mutual "$a = $b->x(); $b = $a->y();" used to
// overflow the goroutine stack — a fatal, unrecoverable runtime error. After the
// fix (per-goroutine recursion guard) these inputs must return normally.

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// buildRecursionResolver creates a Resolver whose ChainResolver and
// TypedChainResolver mimic the hover/completion behavior: they call back into
// ResolveVariableType / ResolveVariableTypeTyped for any variable found in the
// expression, faithfully reproducing the real cycle.
//
// The optional chainHook is called every time ChainResolver fires so tests can
// count invocations or record calls.
func buildRecursionResolver(t *testing.T, idx *symbols.Index, lines []string, file *parser.FileNode, chainHook func()) *Resolver {
	t.Helper()
	r := NewResolver(idx)

	r.ChainResolver = func(expr, source string, pos protocol.Position, f *parser.FileNode) string {
		if chainHook != nil {
			chainHook()
		}
		// Strip trailing semicolon (matches real resolveExpressionType usage)
		expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(expr), ";"))
		if expr == "" {
			return ""
		}
		// If the expression starts with a variable, resolve it — this is exactly
		// the re-entrant call that causes the cycle.
		if strings.HasPrefix(expr, "$") {
			// Extract just the variable name (up to the first non-identifier char)
			varName := expr
			for i := 1; i < len(expr); i++ {
				c := expr[i]
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
					varName = expr[:i]
					break
				}
			}
			return r.ResolveVariableType(varName, lines, pos, f)
		}
		return ""
	}

	r.TypedChainResolver = func(expr, source string, pos protocol.Position, f *parser.FileNode) ResolvedType {
		if chainHook != nil {
			chainHook()
		}
		expr = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(expr), ";"))
		if expr == "" {
			return ResolvedType{}
		}
		if strings.HasPrefix(expr, "$") {
			varName := expr
			for i := 1; i < len(expr); i++ {
				c := expr[i]
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
					varName = expr[:i]
					break
				}
			}
			rt := r.ResolveVariableTypeTyped(varName, lines, pos, f)
			return rt
		}
		return ResolvedType{}
	}

	return r
}

// TestRecursionGuard_SelfReferentialAssignment covers the basic case:
//
//	$x = $x->foo();
//
// Before the fix this caused a fatal goroutine stack overflow that recover()
// could not catch. After the fix it must return normally (the empty result is
// acceptable — we just need the call to terminate).
func TestRecursionGuard_SelfReferentialAssignment(t *testing.T) {
	source := `<?php
$x = $x->foo();
$y = $x->bar();
`
	lines := SplitLines(source)
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()

	var calls int
	r := buildRecursionResolver(t, idx, lines, file, func() { calls++ })

	// Position the cursor on the "$y = $x->bar();" line, resolving $x.
	// This will hit $x = $x->foo() on the line above and recurse.
	pos := protocol.Position{Line: 2}
	got := r.ResolveVariableType("$x", lines, pos, file)

	// We don't assert a specific type — any return (including "") is a pass.
	// The important thing is that we reach this line (no fatal crash).
	t.Logf("ResolveVariableType returned %q after %d chain calls", got, calls)
}

// TestRecursionGuard_SelfReferentialTyped covers the same case via
// ResolveVariableTypeTyped, which has its own depth counter guard.
func TestRecursionGuard_SelfReferentialTyped(t *testing.T) {
	source := `<?php
$x = $x->foo();
`
	lines := SplitLines(source)
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()

	var calls int
	r := buildRecursionResolver(t, idx, lines, file, func() { calls++ })

	pos := protocol.Position{Line: 1}
	got := r.ResolveVariableTypeTyped("$x", lines, pos, file)

	t.Logf("ResolveVariableTypeTyped returned %v after %d chain calls", got, calls)
}

// TestRecursionGuard_MutualAssignment covers two variables that reference each
// other:
//
//	$a = $b->x();
//	$b = $a->y();
//
// Resolving $a recurses into $b which recurses into $a, forming a cycle.
func TestRecursionGuard_MutualAssignment(t *testing.T) {
	source := `<?php
$a = $b->x();
$b = $a->y();
`
	lines := SplitLines(source)
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()

	var calls int
	r := buildRecursionResolver(t, idx, lines, file, func() { calls++ })

	pos := protocol.Position{Line: 2}
	gotA := r.ResolveVariableType("$a", lines, pos, file)
	gotB := r.ResolveVariableType("$b", lines, pos, file)

	t.Logf("$a → %q, $b → %q after %d chain calls", gotA, gotB, calls)
}

// TestRecursionGuard_MutualAssignmentTyped covers the mutual assignment case
// through the TypedChainResolver path.
func TestRecursionGuard_MutualAssignmentTyped(t *testing.T) {
	source := `<?php
$a = $b->x();
$b = $a->y();
`
	lines := SplitLines(source)
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()

	var calls int
	r := buildRecursionResolver(t, idx, lines, file, func() { calls++ })

	pos := protocol.Position{Line: 2}
	gotA := r.ResolveVariableTypeTyped("$a", lines, pos, file)
	gotB := r.ResolveVariableTypeTyped("$b", lines, pos, file)

	t.Logf("$a → %v, $b → %v after %d chain calls", gotA, gotB, calls)
}

// TestRecursionGuard_DeepChain covers a deeper cycle (three variables in a
// ring: $a → $b → $c → $a) that would previously stack-overflow before
// the guard triggers.
func TestRecursionGuard_DeepChain(t *testing.T) {
	source := `<?php
$a = $b->x();
$b = $c->y();
$c = $a->z();
`
	lines := SplitLines(source)
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()

	var calls int
	r := buildRecursionResolver(t, idx, lines, file, func() { calls++ })

	pos := protocol.Position{Line: 3}
	got := r.ResolveVariableType("$a", lines, pos, file)

	t.Logf("deep cycle $a → %q after %d chain calls", got, calls)
}

// TestRecursionGuard_NormalChainUnaffected verifies that the depth guard does
// NOT interfere with ordinary (non-recursive) resolution. A variable assigned
// from "new ClassName()" must still resolve to the correct FQN.
func TestRecursionGuard_NormalChainUnaffected(t *testing.T) {
	source := `<?php
namespace App;
use Monolog\Logger;
class Service {}

$logger = new Logger();
$x = $logger->info("hello");
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Logger.php", `<?php
namespace Monolog;
class Logger {
    public function info(string $msg): void {}
}
`)
	idx.IndexFile("file:///app.php", source)

	lines := SplitLines(source)
	file := parser.ParseFile(source)

	r := buildRecursionResolver(t, idx, lines, file, nil)

	// $logger is assigned from "new Logger()" — must resolve to Monolog\Logger.
	pos := protocol.Position{Line: 6}
	got := r.ResolveVariableType("$logger", lines, pos, file)
	if got != "Monolog\\Logger" {
		t.Errorf("expected Monolog\\Logger, got %q (regression: depth guard triggered prematurely)", got)
	}

	// TypedChainResolver path should also resolve correctly.
	gotTyped := r.ResolveVariableTypeTyped("$logger", lines, pos, file)
	if gotTyped.FQN != "Monolog\\Logger" {
		t.Errorf("TypedChainResolver: expected Monolog\\Logger, got %q", gotTyped.FQN)
	}
}

// TestRecursionGuard_StateClearsAfterGuard confirms that a guarded call clears
// the resolver state so later resolutions still behave normally.
func TestRecursionGuard_StateClearsAfterGuard(t *testing.T) {
	source := `<?php
$x = $x->foo();
$logger = new \Monolog\Logger();
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Logger.php", `<?php
namespace Monolog;
class Logger {}
`)

	lines := SplitLines(source)
	file := parser.ParseFile(source)

	r := buildRecursionResolver(t, idx, lines, file, nil)

	pos := protocol.Position{Line: 1}

	// First call: self-referential, guard should fire and return ""
	_ = r.ResolveVariableType("$x", lines, pos, file)

	r.stateMu.Lock()
	gotStates := len(r.states)
	r.stateMu.Unlock()
	if gotStates != 0 {
		t.Fatalf("resolver state count after guarded call = %d, want 0", gotStates)
	}

	// Second call: normal resolution must still work.
	pos2 := protocol.Position{Line: 2}
	got2 := r.ResolveVariableType("$logger", lines, pos2, file)
	if got2 != "Monolog\\Logger" {
		t.Errorf("after guarded call, normal resolution got %q, want Monolog\\Logger", got2)
	}
}

func TestRecursionGuard_ConcurrentNonRecursiveTypedResolutions(t *testing.T) {
	source := `<?php
$result = Factory::make();
`
	lines := SplitLines(source)
	file := parser.ParseFile(source)
	if file == nil {
		t.Fatal("expected parsed file")
	}

	r := NewResolver(symbols.NewIndex())
	r.TypedChainResolver = func(expr, source string, pos protocol.Position, f *parser.FileNode) ResolvedType {
		if expr == "Factory::make()" {
			time.Sleep(20 * time.Millisecond)
			return ResolvedType{FQN: "App\\Logger"}
		}
		return ResolvedType{}
	}

	const workers = 64
	var wg sync.WaitGroup
	results := make(chan string, workers)
	pos := protocol.Position{Line: 1}

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- r.ResolveVariableTypeTyped("$result", lines, pos, file).FQN
		}()
	}

	wg.Wait()
	close(results)

	for got := range results {
		if got != "App\\Logger" {
			t.Fatalf("concurrent typed resolution got %q, want App\\Logger", got)
		}
	}
}
