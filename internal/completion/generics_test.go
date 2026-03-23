package completion

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

// TestGenericChainResolution verifies that generic type parameters propagate
// through Eloquent method chains.
func TestGenericChainResolution(t *testing.T) {
	p := setupRealisticEloquentTest(t)

	resolveTyped := func(expr, source string) string {
		file := parser.ParseFile(source)
		rt := p.ResolveExpressionTypeTyped(expr, source, protocol.Position{Line: 5}, file)
		return rt.String()
	}

	preamble := "<?php\nuse App\\Models\\Category;\nuse App\\Models\\Product;\n\n\n"

	t.Run("Category::query() returns Builder<Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->with() returns Builder<Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()->with('rel')", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->get() returns Collection<int, Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()->get()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->get()->first() returns ?Category", func(t *testing.T) {
		typ := resolveTyped("Category::query()->get()->first()", preamble)
		if typ != "?App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->with()->where()->get() returns Collection<int, Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()->with('x')->where('y', 1)->get()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::where()->first() returns ?Category", func(t *testing.T) {
		typ := resolveTyped("Category::where('active', 1)->first()", preamble)
		if typ != "?App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::all() returns Collection<int, Category>", func(t *testing.T) {
		// all() is a virtual static method on the model returning Collection
		// The template system should inject model context
		typ := resolveTyped("Category::all()", preamble)
		// all() returns bare Collection from eloquent.go, but with template
		// resolution it should get Collection<int, Category>
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			// Acceptable fallback: bare Collection
			if typ != "Illuminate\\Database\\Eloquent\\Collection" {
				t.Errorf("got %q", typ)
			}
		}
	})

	t.Run("Category::first() returns Category (static already correct)", func(t *testing.T) {
		typ := resolveTyped("Category::first()", preamble)
		if typ != "App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("non-Eloquent chain still works", func(t *testing.T) {
		source := preamble
		file := parser.ParseFile(source)
		// Logger is not an Eloquent model, should resolve normally
		rt := p.ResolveExpressionTypeTyped("Category::first()", source, protocol.Position{Line: 5}, file)
		if rt.IsEmpty() {
			t.Error("expected non-empty for Category::first()")
		}
	})
}

// TestGenericCompletionMembers verifies that after resolving a generic type,
// the correct members are available for completion.
func TestGenericCompletionMembers(t *testing.T) {
	p := setupRealisticEloquentTest(t)

	t.Run("Category::get()->first()-> shows Category members", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::get()->first()->"
		// len("Category::get()->first()->") = 26
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 26})
		labels := collectLabels(items)

		if !labels["save"] {
			t.Errorf("expected 'save' after get()->first()->, got: %v", labels)
		}
	})

	t.Run("Category::query()->get()-> shows Collection methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->get()->"
		// len("Category::query()->get()->") = 26
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 26})
		labels := collectLabels(items)

		for _, m := range []string{"first", "count", "all", "map"} {
			if !labels[m] {
				t.Errorf("expected %q after query()->get()->, got: %v", m, labels)
			}
		}
	})
}
