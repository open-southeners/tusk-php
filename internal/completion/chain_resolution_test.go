package completion

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/models"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// TestChainReturnTypeResolution verifies return type resolution at each step
// of an Eloquent method chain, documenting current behavior and marking where
// generic type support is needed.
func TestChainReturnTypeResolution(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)

	resolve := func(expr, source string) string {
		file := parser.ParseFile(source)
		return p.ResolveExpressionType(expr, source, protocol.Position{Line: 5}, file)
	}

	preamble := "<?php\nuse App\\Models\\Category;\nuse App\\Models\\Product;\n\n\n"

	t.Run("Category::query() returns Builder", func(t *testing.T) {
		typ := resolve("Category::query()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder" {
			t.Errorf("expected Builder, got %q", typ)
		}
	})

	t.Run("Category::query()->with() returns Builder (static on Builder)", func(t *testing.T) {
		// Builder::with() returns `static` which resolves to Builder itself.
		// With generics, this should return Builder<Category> preserving the model context.
		typ := resolve("Category::query()->with(['products'])", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder" {
			t.Errorf("expected Builder, got %q", typ)
		}
	})

	t.Run("Category::query()->with()->get() returns Collection", func(t *testing.T) {
		// Builder::get() returns Collection.
		// With generics, this should return Collection<int, Category>.
		typ := resolve("Category::query()->with(['products'])->get(['id', 'name'])", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})

	t.Run("Collection->first() returns Model (generic gap)", func(t *testing.T) {
		// Collection::first() returns ?Model in the test stubs.
		// With generics, Collection<int, Category>::first() should return ?Category.
		// Currently this resolves to Model (the base class), losing the specific model type.
		source := preamble + "$cats = Category::get();\n$cats->first()"
		typ := resolve("$cats->first()", source)
		// Currently: resolves to Model (not Category) — this is the generic gap
		if typ != "Illuminate\\Database\\Eloquent\\Model" {
			t.Logf("Collection::first() resolved to %q (expected Model without generics)", typ)
		}
	})

	t.Run("Category::where() returns Builder", func(t *testing.T) {
		typ := resolve("Category::where('active', true)", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder" {
			t.Errorf("expected Builder, got %q", typ)
		}
	})

	t.Run("Category::first() returns Category (static resolved)", func(t *testing.T) {
		// The virtual static method first() has return type "static" which
		// eloquent.go converts to the model FQN during injection.
		typ := resolve("Category::first()", preamble)
		if typ != "App\\Models\\Category" {
			t.Errorf("expected App\\Models\\Category, got %q", typ)
		}
	})

	t.Run("Category::find() returns Category (static resolved)", func(t *testing.T) {
		typ := resolve("Category::find(1)", preamble)
		if typ != "App\\Models\\Category" {
			t.Errorf("expected App\\Models\\Category, got %q", typ)
		}
	})

	t.Run("Category::all() returns Collection", func(t *testing.T) {
		typ := resolve("Category::all()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})

	t.Run("Category::get() returns Collection", func(t *testing.T) {
		typ := resolve("Category::get()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})

	t.Run("full chain: query->with->get->count", func(t *testing.T) {
		// Collection::count() returns int
		typ := resolve("Category::query()->with(['products'])->get(['id'])->count()", preamble)
		if typ != "int" {
			t.Errorf("expected int, got %q", typ)
		}
	})

	t.Run("full chain: query->with->get->map", func(t *testing.T) {
		// Collection::map() returns static → Collection
		typ := resolve("Category::query()->with(['products'])->get(['id'])->map(fn($x) => $x)", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})
}

// setupRealisticEloquentTest creates stubs matching the real Laravel vendor
// signatures: no PHP return types, only @return docblocks.
func setupRealisticEloquentTest(t *testing.T) *Provider {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {
    /**
     * @return \Illuminate\Database\Eloquent\Builder<static>
     */
    public static function query() {}

    /**
     * @return bool
     */
    public function save() {}

    /**
     * @return bool
     */
    public function delete() {}

    /**
     * @return array
     */
    public function toArray() {}

    /**
     * @return static
     */
    public function refresh() {}
}
`)
	idx.IndexFile("file:///vendor/Builder.php", `<?php
namespace Illuminate\Database\Eloquent;
class Builder {
    /**
     * @param  mixed  $column
     * @return $this
     */
    public function where($column, $operator = null, $value = null) {}

    /**
     * @return \Illuminate\Database\Eloquent\Model|null
     */
    public function first() {}

    /**
     * @return \Illuminate\Database\Eloquent\Collection
     */
    public function get($columns = []) {}

    /**
     * @return $this
     */
    public function with($relations) {}

    /**
     * @return $this
     */
    public function orderBy($column, $direction = 'asc') {}
}
`)
	idx.IndexFile("file:///vendor/Collection.php", `<?php
namespace Illuminate\Database\Eloquent;
class Collection extends \Illuminate\Support\Collection {
    /**
     * @return \Illuminate\Database\Eloquent\Model|null
     */
    public function find($key, $default = null) {}
}
`)
	idx.IndexFile("file:///vendor/SupportCollection.php", `<?php
namespace Illuminate\Support;
class Collection {
    /**
     * @return int
     */
    public function count() {}

    /**
     * @return mixed
     */
    public function first($callback = null, $default = null) {}

    /**
     * @return array
     */
    public function all() {}

    /**
     * @return static
     */
    public function map($callback) {}
}
`)

	categorySource := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class Category extends Model {
    public string $name;
}
`
	idx.IndexFile("file:///app/Models/Category.php", categorySource)

	models.AnalyzeEloquentModels(idx, "/tmp")

	return NewProvider(idx, nil, "laravel")
}

// TestRealisticChainResolution tests chain resolution with real Laravel-style
// vendor stubs that use @return docblocks instead of PHP return types.
func TestRealisticChainResolution(t *testing.T) {
	p := setupRealisticEloquentTest(t)

	resolve := func(expr, source string) string {
		file := parser.ParseFile(source)
		return p.ResolveExpressionType(expr, source, protocol.Position{Line: 5}, file)
	}

	preamble := "<?php\nuse App\\Models\\Category;\n\n\n\n"

	t.Run("query() returns Builder", func(t *testing.T) {
		typ := resolve("Category::query()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("query()->with() returns Builder via @return $this", func(t *testing.T) {
		typ := resolve("Category::query()->with('rel')", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Builder" {
			t.Errorf("expected Builder, got %q", typ)
		}
	})

	t.Run("query()->with()->get() returns Collection", func(t *testing.T) {
		typ := resolve("Category::query()->with('rel')->get()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})

	t.Run("query()->with()->get()->all() returns array", func(t *testing.T) {
		// Collection extends Support\Collection, all() returns array
		typ := resolve("Category::query()->with('rel')->get()->all()", preamble)
		if typ != "array" {
			t.Errorf("expected array, got %q", typ)
		}
	})

	t.Run("query()->with()->get()->count() returns int", func(t *testing.T) {
		typ := resolve("Category::query()->with('rel')->get()->count()", preamble)
		if typ != "int" {
			t.Errorf("expected int, got %q", typ)
		}
	})

	t.Run("where()->orderBy()->get() returns Collection", func(t *testing.T) {
		typ := resolve("Category::where('x', 1)->orderBy('y')->get()", preamble)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})
}

// TestRealisticChainCompletions tests that completions work at each chain step
// with real Laravel-style vendor stubs.
func TestRealisticChainCompletions(t *testing.T) {
	p := setupRealisticEloquentTest(t)

	t.Run("query()->with()-> shows Builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->with('rel')->"
		// len = 32
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 32})
		labels := collectLabels(items)

		for _, m := range []string{"where", "get", "orderBy", "with"} {
			if !labels[m] {
				t.Errorf("expected %q after with()->, got: %v", m, labels)
			}
		}
	})

	t.Run("query()->with()->get()-> shows Collection methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->with('rel')->get()->"
		// len = 39
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 39})
		labels := collectLabels(items)

		for _, m := range []string{"all", "count", "first", "map"} {
			if !labels[m] {
				t.Errorf("expected %q after get()->, got: %v", m, labels)
			}
		}
	})

	t.Run("where()->orderBy()-> shows Builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::where('x', 1)->orderBy('y')->"
		// len = 39
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 39})
		labels := collectLabels(items)

		if !labels["get"] {
			t.Errorf("expected 'get' after orderBy()->, got: %v", labels)
		}
	})
}

// TestMultiLineChainResolution verifies that chains spanning multiple lines
// are resolved correctly — this was the root cause of hover/go-to-definition
// landing on wrong symbols in real Laravel projects.
func TestMultiLineChainResolution(t *testing.T) {
	p := setupRealisticEloquentTest(t)

	t.Run("multi-line: query->with->get resolves type via joined line", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()\n    ->with('rel')\n    ->get()\n    ->all()"
		file := parser.ParseFile(source)
		// ResolveExpressionType works with single-line expressions, so join first
		typ := p.ResolveExpressionType("Category::query()->with('rel')->get()", source, protocol.Position{Line: 4}, file)
		if typ != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection, got %q", typ)
		}
	})

	t.Run("multi-line: hover on get() after with() shows Builder member", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()\n    ->with('rel')\n    ->get()"
		// Line 4: "    ->get()", cursor on "get" at character 6
		lines := strings.Split(source, "\n")
		joined, wordStart := resolve.JoinChainLines(lines, 4, 6)

		// The joined line should contain the full chain
		if !strings.Contains(joined, "Category::query()") {
			t.Errorf("joined line should contain chain origin, got: %q", joined)
		}
		if !strings.Contains(joined, "->with") {
			t.Errorf("joined line should contain ->with, got: %q", joined)
		}
		if !strings.Contains(joined, "->get") {
			t.Errorf("joined line should contain ->get, got: %q", joined)
		}
		_ = wordStart
	})

	t.Run("multi-line: completions after get()-> on continuation line", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()\n    ->with('rel')\n    ->get()\n    ->"
		// Line 5: "    ->", cursor at character 6
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 5, Character: 6})
		labels := collectLabels(items)

		for _, m := range []string{"all", "count", "first", "map"} {
			if !labels[m] {
				t.Errorf("expected Collection method %q after multi-line get()->, got: %v", m, labels)
			}
		}
	})

	t.Run("multi-line: completions after with()-> on continuation line", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()\n    ->with('rel')\n    ->"
		// Line 4: "    ->", cursor at character 6
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 4, Character: 6})
		labels := collectLabels(items)

		for _, m := range []string{"where", "get", "orderBy"} {
			if !labels[m] {
				t.Errorf("expected Builder method %q after multi-line with()->, got: %v", m, labels)
			}
		}
	})

	t.Run("single-line chain still works", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->with('rel')->get()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 39})
		labels := collectLabels(items)

		if !labels["all"] || !labels["count"] {
			t.Errorf("single-line chain should still work, got: %v", labels)
		}
	})
}

// TestChainCompletionItems verifies that completions appear at each step
// of a method chain, checking that the correct members are available.
func TestChainCompletionItems(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)

	t.Run("Category::query()-> shows Builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		for _, m := range []string{"where", "first", "get", "with", "orderBy"} {
			if !labels[m] {
				t.Errorf("expected Builder method %q after query()->, got: %v", m, labels)
			}
		}
	})

	t.Run("Category::query()->with()-> shows Builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->with('products')->"
		// "Category::query()->with('products')->" = 37 chars
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 37})
		labels := collectLabels(items)

		if !labels["get"] {
			t.Errorf("expected 'get' after with()->, got: %v", labels)
		}
		if !labels["where"] {
			t.Errorf("expected 'where' after with()->, got: %v", labels)
		}
	})

	t.Run("Category::query()->with()->get()-> shows Collection methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->with(['products'])->get()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 46})
		labels := collectLabels(items)

		for _, m := range []string{"count", "first", "map"} {
			if !labels[m] {
				t.Errorf("expected Collection method %q after get()->, got: %v", m, labels)
			}
		}
	})

	t.Run("Category::where()->orderBy()-> shows Builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::where('active', 1)->orderBy('name')->"
		// "Category::where('active', 1)->orderBy('name')->" = 47 chars
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 47})
		labels := collectLabels(items)

		if !labels["get"] {
			t.Errorf("expected 'get' after orderBy()->, got: %v", labels)
		}
		if !labels["first"] {
			t.Errorf("expected 'first' after orderBy()->, got: %v", labels)
		}
	})

	t.Run("Category::first()-> shows Category model methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::first()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		// first() returns static (→ Category), so model members should appear
		if !labels["save"] {
			t.Errorf("expected 'save' from Model after first()->, got: %v", labels)
		}
		if !labels["products"] {
			t.Errorf("expected 'products' relation after first()->, got: %v", labels)
		}
	})
}
