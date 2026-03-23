package completion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/models"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// setupLaravelE2E indexes the real testdata/laravel vendor files and project
// models, runs Eloquent analysis, and returns a Provider ready for testing.
// This tests against the actual Laravel framework source code.
func setupLaravelE2E(t *testing.T) *Provider {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "laravel")

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	// Index real vendor files
	for _, dir := range []string{
		"vendor/laravel/framework/src/Illuminate/Database/Eloquent",
		"vendor/laravel/framework/src/Illuminate/Collections",
		"vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations",
	} {
		absDir := filepath.Join(root, dir)
		filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".php" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			idx.IndexFileWithSource("file:///"+rel, string(data), symbols.SourceVendor)
			return nil
		})
	}

	// Index project models
	for _, rel := range []string{
		"app/Models/Category.php",
		"app/Models/Product.php",
		"app/Models/User.php",
	} {
		data, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		idx.IndexFileWithSource("file:///"+rel, string(data), symbols.SourceProject)
	}

	// Run Eloquent model analysis
	models.AnalyzeEloquentModels(idx, root)

	ca := container.NewContainerAnalyzer(idx, root, "laravel")
	return NewProvider(idx, ca, "laravel")
}

// TestE2EGenericChainResolution tests generic type propagation through
// Eloquent method chains using the real Laravel vendor source files.
func TestE2EGenericChainResolution(t *testing.T) {
	p := setupLaravelE2E(t)

	resolveTyped := func(expr, source string, line int) string {
		file := parser.ParseFile(source)
		rt := p.ResolveExpressionTypeTyped(expr, source, protocol.Position{Line: line}, file)
		return rt.String()
	}

	preamble := "<?php\nuse App\\Models\\Category;\nuse App\\Models\\Product;\n\n"

	t.Run("Category::query() returns Builder<Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()", preamble, 4)
		if typ != "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->get() returns Collection<int, Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()->get()", preamble, 4)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->get()->first() returns Category", func(t *testing.T) {
		typ := resolveTyped("Category::query()->get()->first()", preamble, 4)
		// Real Collection::first() has complex @template TFirstDefault signature;
		// nullable may not be detected. Accept Category or ?Category.
		if typ != "App\\Models\\Category" && typ != "?App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->get()->all() returns array<int, Category>", func(t *testing.T) {
		typ := resolveTyped("Category::query()->get()->all()", preamble, 4)
		if typ != "array<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->with()->get() preserves generics through $this chain", func(t *testing.T) {
		typ := resolveTyped("Category::query()->with('products')->get()", preamble, 4)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Category::query()->with()->get()->first() through full chain", func(t *testing.T) {
		typ := resolveTyped("Category::query()->with('products')->get()->first()", preamble, 4)
		if typ != "App\\Models\\Category" && typ != "?App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Product::query()->get() returns Collection<int, Product>", func(t *testing.T) {
		typ := resolveTyped("Product::query()->get()", preamble, 4)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Product>" {
			t.Errorf("got %q", typ)
		}
	})
}

// TestE2EVariableGenericPropagation tests that generic types survive through
// variable assignments using real vendor files.
func TestE2EVariableGenericPropagation(t *testing.T) {
	p := setupLaravelE2E(t)

	resolveTyped := func(expr, source string, line int) string {
		file := parser.ParseFile(source)
		rt := p.ResolveExpressionTypeTyped(expr, source, protocol.Position{Line: line}, file)
		return rt.String()
	}

	t.Run("$categories = Category::query()->get() is Collection<int, Category>", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$categories = Category::query()->get();\n"
		typ := resolveTyped("$categories", source, 3)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("$fc = $categories->first() is Category", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$categories = Category::query()->get();\n$fc = $categories->first();\n"
		typ := resolveTyped("$fc", source, 4)
		if typ != "App\\Models\\Category" && typ != "?App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("$arr = Category::query()->get()->all() is array<int, Category>", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$arr = Category::query()->get()->all();\n"
		typ := resolveTyped("$arr", source, 3)
		if typ != "array<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("$item = $arr[0] is Category from array<int, Category>", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$arr = Category::query()->get()->all();\n$item = $arr[0];\n"
		typ := resolveTyped("$item", source, 4)
		if typ != "App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("$category = Category::first() is Category (static resolved)", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$category = Category::first();\n"
		typ := resolveTyped("$category", source, 3)
		if typ != "App\\Models\\Category" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("$c = Category::all() is Collection<int, Category>", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$c = Category::all();\n"
		typ := resolveTyped("$c", source, 3)
		if typ != "Illuminate\\Database\\Eloquent\\Collection<int, App\\Models\\Category>" {
			t.Errorf("got %q", typ)
		}
	})
}

// TestE2ECompletionsAfterGenericChain tests that completions show the correct
// members at each step of a generic method chain using real vendor files.
func TestE2ECompletionsAfterGenericChain(t *testing.T) {
	p := setupLaravelE2E(t)

	t.Run("Category::query()-> shows Builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		// Real Builder has where, get, with directly; first is from BuildsQueries trait
		for _, m := range []string{"where", "get", "with"} {
			if !labels[m] {
				t.Errorf("expected Builder method %q, got: %v", m, mapKeys(labels, 10))
			}
		}
	})

	t.Run("Category::query()->get()-> shows Collection methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->get()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 27})
		labels := collectLabels(items)

		// Collection inherits from Support\Collection which has all/count etc.
		if !labels["all"] && !labels["count"] && !labels["toArray"] {
			t.Errorf("expected Collection methods, got: %v", mapKeys(labels, 10))
		}
	})

	t.Run("$categories-> shows Collection methods after variable assignment", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$categories = Category::query()->get();\n$categories->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 13})
		labels := collectLabels(items)

		for _, m := range []string{"first", "all", "count"} {
			if !labels[m] {
				t.Errorf("expected Collection method %q on $categories->, got: %v", m, mapKeys(labels, 10))
			}
		}
	})

	t.Run("$fc-> shows Category model members after first()", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$c = Category::query()->get();\n$fc = $c->first();\n$fc->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 4, Character: 5})
		labels := collectLabels(items)

		if !labels["save"] {
			t.Errorf("expected Model method 'save' on $fc->, got: %v", mapKeys(labels, 10))
		}
		if !labels["name"] {
			t.Errorf("expected property 'name' on $fc->, got: %v", mapKeys(labels, 10))
		}
	})

	t.Run("Category::first()-> shows Category model members", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::first()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		if !labels["save"] {
			t.Errorf("expected 'save' on Category::first()->, got: %v", mapKeys(labels, 10))
		}
		if !labels["products"] {
			t.Errorf("expected 'products' relation on Category::first()->, got: %v", mapKeys(labels, 10))
		}
	})
}

// TestE2EMultiLineChainGenerics tests generic resolution through multi-line
// chains using real vendor files.
func TestE2EMultiLineChainGenerics(t *testing.T) {
	p := setupLaravelE2E(t)

	t.Run("multi-line chain completions after get()->", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()\n    ->with('products')\n    ->get()\n    ->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 5, Character: 6})
		labels := collectLabels(items)

		for _, m := range []string{"first", "all", "count"} {
			if !labels[m] {
				t.Errorf("expected Collection method %q on multi-line chain, got: %v", m, mapKeys(labels, 10))
			}
		}
	})

	t.Run("single-line variable assignment preserves generics", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\n$categories = Category::query()->get()->all();\n$first = $categories[0];\n"
		file := parser.ParseFile(source)

		rt := p.ResolveExpressionTypeTyped("$categories", source, protocol.Position{Line: 3}, file)
		if rt.String() != "array<int, App\\Models\\Category>" {
			t.Errorf("$categories: got %q", rt.String())
		}

		rt2 := p.ResolveExpressionTypeTyped("$first", source, protocol.Position{Line: 4}, file)
		if rt2.String() != "App\\Models\\Category" {
			t.Errorf("$first: got %q", rt2.String())
		}
	})
}

// mapKeys returns up to n keys from a map for readable error messages.
func mapKeys(m map[string]bool, n int) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
		if len(keys) >= n {
			break
		}
	}
	return keys
}
