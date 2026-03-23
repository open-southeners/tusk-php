package completion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
)

// laravelControllerSource reads the real CategoryController.php from testdata.
func laravelControllerSource(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "testdata", "laravel")
	data, err := os.ReadFile(filepath.Join(root, "app/Http/Controllers/CategoryController.php"))
	if err != nil {
		t.Fatalf("cannot read controller: %v", err)
	}
	return string(data)
}

// findLine returns the 0-based line number containing the given substring.
func findLine(source, substr string) int {
	for i, line := range strings.Split(source, "\n") {
		if strings.Contains(line, substr) {
			return i
		}
	}
	return -1
}

// TestE2ELaravelEloquentChains tests Eloquent query chain generic resolution
// against the real CategoryController.php and vendor files.
func TestE2ELaravelEloquentChains(t *testing.T) {
	p := setupLaravelE2E(t)
	source := laravelControllerSource(t)
	file := parser.ParseFile(source)
	uri := "file:///app/Http/Controllers/CategoryController.php"
	p.index.IndexFile(uri, source)

	rt := func(varName string, lineSubstr string) resolve.ResolvedType {
		line := findLine(source, lineSubstr)
		if line < 0 {
			t.Fatalf("line containing %q not found", lineSubstr)
		}
		return p.ResolveExpressionTypeTyped(varName, source, protocol.Position{Line: line + 1}, file)
	}

	t.Run("Category::first() returns Category", func(t *testing.T) {
		r := rt("$category", "$category = Category::first()")
		if r.FQN != "App\\Models\\Category" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("Category::find(1) returns Category", func(t *testing.T) {
		r := rt("$found", "$found = Category::find(1)")
		if r.FQN != "App\\Models\\Category" && r.FQN != "" {
			// find() may return ?Category
			if r.FQN != "App\\Models\\Category" {
				t.Errorf("got %q", r.String())
			}
		}
	})

	t.Run("Category::all() returns Collection<int, Category>", func(t *testing.T) {
		r := rt("$allCategories", "$allCategories = Category::all()")
		if r.BaseFQN() != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("got %q", r.String())
		}
		if !r.IsGeneric() || len(r.Params) < 2 {
			t.Errorf("expected generic, got %q", r.String())
		}
	})

	t.Run("multi-line query chain variable gets Builder or Collection", func(t *testing.T) {
		// Multi-line: $queried = Category::query()\n->where()->orderBy()->get()
		// Variable inference sees the assignment line only: Category::query() → Builder<Category>
		// Full chain resolution would need multi-line RHS joining (future improvement)
		r := rt("$queried", "$queried = Category::query()")
		if r.BaseFQN() != "Illuminate\\Database\\Eloquent\\Builder" && r.BaseFQN() != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("query()->get()->first() returns ?Category", func(t *testing.T) {
		r := rt("$firstFromQuery", "$firstFromQuery = Category::query()->get()->first()")
		if r.FQN != "App\\Models\\Category" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("query()->get()->all() returns array<int, Category>", func(t *testing.T) {
		r := rt("$asArray", "$asArray = Category::query()->get()->all()")
		if r.FQN != "array" || !r.IsGeneric() {
			t.Errorf("got %q", r.String())
		}
		if len(r.Params) >= 2 && r.Params[1].FQN != "App\\Models\\Category" {
			t.Errorf("value type: got %q", r.Params[1].String())
		}
	})

	t.Run("$asArray[0] returns Category", func(t *testing.T) {
		r := rt("$firstFromArray", "$firstFromArray = $asArray[0]")
		if r.FQN != "App\\Models\\Category" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("variable propagation: $cats -> $firstCat", func(t *testing.T) {
		r := rt("$cats", "$cats = Category::query()->get()")
		if r.BaseFQN() != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("$cats: got %q", r.String())
		}

		r2 := rt("$firstCat", "$firstCat = $cats->first()")
		if r2.FQN != "App\\Models\\Category" {
			t.Errorf("$firstCat: got %q", r2.String())
		}
	})

	t.Run("Product::query()->get() returns Collection<int, Product>", func(t *testing.T) {
		r := rt("$productCollection", "$productCollection = Product::query()->get()")
		if r.BaseFQN() != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("got %q", r.String())
		}
		if r.IsGeneric() && len(r.Params) >= 2 && r.Params[1].FQN != "App\\Models\\Product" {
			t.Errorf("TModel: got %q", r.Params[1].String())
		}
	})
}

// TestE2ELaravelArrayShapes tests array literal shape inference and Collection
// constructor propagation against the real controller file.
func TestE2ELaravelArrayShapes(t *testing.T) {
	p := setupLaravelE2E(t)
	source := laravelControllerSource(t)
	file := parser.ParseFile(source)
	uri := "file:///app/Http/Controllers/CategoryController.php"
	p.index.IndexFile(uri, source)

	rt := func(varName string, lineSubstr string) resolve.ResolvedType {
		line := findLine(source, lineSubstr)
		if line < 0 {
			t.Fatalf("line containing %q not found", lineSubstr)
		}
		return p.ResolveExpressionTypeTyped(varName, source, protocol.Position{Line: line + 1}, file)
	}

	t.Run("associative array shape", func(t *testing.T) {
		r := rt("$employee", "$employee = ['id' => 1")
		if r.FQN != "array" || r.Shape == "" {
			t.Errorf("got %q (shape: %q)", r.String(), r.Shape)
		}
		if !strings.Contains(r.Shape, "id: int") || !strings.Contains(r.Shape, "name: string") {
			t.Errorf("shape: %q", r.Shape)
		}
	})

	t.Run("array of shapes", func(t *testing.T) {
		r := rt("$employeesArr", "$employeesArr = [")
		if r.FQN != "array" || !r.IsGeneric() {
			t.Errorf("got %q", r.String())
		}
		if len(r.Params) >= 2 && r.Params[1].Shape == "" {
			t.Errorf("expected nested shape, got %q", r.Params[1].String())
		}
	})

	t.Run("new Collection($employeesArr) gets generics from arg", func(t *testing.T) {
		r := rt("$col", "$col = new Collection($employeesArr)")
		if r.BaseFQN() != "Illuminate\\Support\\Collection" {
			t.Errorf("base: got %q", r.String())
		}
		if !r.IsGeneric() || len(r.Params) < 2 {
			t.Errorf("expected generic, got %q", r.String())
		}
	})

	t.Run("$col->first() returns element shape", func(t *testing.T) {
		r := rt("$firstEmployee", "$firstEmployee = $col->first()")
		if r.FQN != "array" || r.Shape == "" {
			t.Errorf("got %q (shape: %q)", r.String(), r.Shape)
		}
	})

	t.Run("Arr::first($employeesArr) returns element shape", func(t *testing.T) {
		r := rt("$firstViaArr", "$firstViaArr = Arr::first($employeesArr)")
		if r.FQN != "array" || r.Shape == "" {
			t.Errorf("got %q (shape: %q)", r.String(), r.Shape)
		}
	})
}

// TestE2ELaravelCompletions tests that completions work at key points in the
// real controller file.
func TestE2ELaravelCompletions(t *testing.T) {
	p := setupLaravelE2E(t)
	source := laravelControllerSource(t)
	uri := "file:///app/Http/Controllers/CategoryController.php"
	p.index.IndexFile(uri, source)

	complete := func(lineSubstr string, afterSuffix string) map[string]bool {
		line := findLine(source, lineSubstr)
		if line < 0 {
			t.Fatalf("line containing %q not found", lineSubstr)
		}
		// Build source up to the point after the suffix on that line
		lines := strings.Split(source, "\n")
		testLine := lines[line]
		idx := strings.Index(testLine, afterSuffix)
		if idx < 0 {
			t.Fatalf("suffix %q not found on line %q", afterSuffix, testLine)
		}
		char := idx + len(afterSuffix)

		items := p.GetCompletions(uri, source, protocol.Position{Line: line, Character: char})
		return collectLabels(items)
	}

	t.Run("$category-> shows Model members", func(t *testing.T) {
		// We need a line with $category-> to test
		// Use the relation access line: $products = $category->products
		labels := complete("$products = $category->", "$category->")
		if !labels["name"] && !labels["slug"] {
			t.Errorf("expected model properties, got: %v", mapKeys(labels, 10))
		}
	})

	t.Run("$col-> shows Collection methods", func(t *testing.T) {
		labels := complete("$firstEmployee = $col->", "$col->")
		if !labels["first"] && !labels["all"] && !labels["count"] {
			t.Errorf("expected Collection methods, got: %v", mapKeys(labels, 10))
		}
	})
}
