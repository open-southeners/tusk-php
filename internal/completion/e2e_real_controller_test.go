package completion

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
)

// TestE2ERealControllerFile tests type resolution on the actual
// testdata/laravel/app/Http/Controllers/CategoryController.php file,
// indexing the real vendor dependencies, exactly as the live LSP would.
func TestE2ERealControllerFile(t *testing.T) {
	p := setupLaravelE2E(t)
	source := laravelControllerSource(t)
	uri := "file:///app/Http/Controllers/CategoryController.php"
	p.index.IndexFile(uri, source)
	file := parser.ParseFile(source)

	rt := func(varName string, lineSubstr string) resolve.ResolvedType {
		line := findLine(source, lineSubstr)
		if line < 0 {
			t.Fatalf("line containing %q not found", lineSubstr)
		}
		return p.ResolveExpressionTypeTyped(varName, source, protocol.Position{Line: line + 1}, file)
	}

	t.Run("$category = Category::first()", func(t *testing.T) {
		r := rt("$category", "$category = Category::first()")
		if r.FQN != "App\\Models\\Category" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("$allCategories = Category::all()", func(t *testing.T) {
		r := rt("$allCategories", "$allCategories = Category::all()")
		if r.BaseFQN() != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("$categories = Category::query()->get()->all()", func(t *testing.T) {
		r := rt("$asArray", "$asArray = Category::query()->get()->all()")
		if r.FQN != "array" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("$firstFromArray = $asArray[0]", func(t *testing.T) {
		r := rt("$firstFromArray", "$firstFromArray = $asArray[0]")
		if r.FQN != "App\\Models\\Category" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("$employeesArr multi-line array shape", func(t *testing.T) {
		r := rt("$employeesArr", "$employeesArr = [")
		if r.FQN != "array" || (!r.IsGeneric() && r.Shape == "") {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("$col = new Collection($employeesArr)", func(t *testing.T) {
		r := rt("$col", "$col = new Collection($employeesArr)")
		if r.BaseFQN() != "Illuminate\\Support\\Collection" {
			t.Errorf("got %q", r.String())
		}
	})

	t.Run("$firstEmployee = $col->first()", func(t *testing.T) {
		r := rt("$firstEmployee", "$firstEmployee = $col->first()")
		if r.FQN != "array" || r.Shape == "" {
			t.Errorf("got %q (shape: %q)", r.String(), r.Shape)
		}
	})

	t.Run("$firstViaArr = Arr::first($employeesArr)", func(t *testing.T) {
		r := rt("$firstViaArr", "$firstViaArr = Arr::first($employeesArr)")
		if r.FQN != "array" || r.Shape == "" {
			t.Errorf("got %q (shape: %q)", r.String(), r.Shape)
		}
	})

	t.Run("completions after $col->", func(t *testing.T) {
		line := findLine(source, "$firstEmployee = $col->first()")
		lines := strings.Split(source, "\n")
		testLine := lines[line]
		col := strings.Index(testLine, "$col->") + 6
		items := p.GetCompletions(uri, source, protocol.Position{Line: line, Character: col})
		labels := collectLabels(items)

		if !labels["first"] && !labels["all"] && !labels["count"] {
			t.Errorf("expected Collection methods, got: %v", mapKeys(labels, 10))
		}
	})
}
