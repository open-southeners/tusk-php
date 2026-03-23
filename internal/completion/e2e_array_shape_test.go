package completion

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

// TestE2EArrayShapeInference tests array literal shape inference using real
// Laravel vendor files from testdata/laravel.
func TestE2EArrayShapeInference(t *testing.T) {
	p := setupLaravelE2E(t)

	resolveTyped := func(expr, source string, line int) string {
		file := parser.ParseFile(source)
		rt := p.ResolveExpressionTypeTyped(expr, source, protocol.Position{Line: line}, file)
		return rt.String()
	}

	t.Run("associative array infers shape", func(t *testing.T) {
		source := "<?php\n$a = ['name' => 'John', 'age' => 30];\n"
		typ := resolveTyped("$a", source, 2)
		if typ != "array{name: string, age: int}" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("indexed int array", func(t *testing.T) {
		source := "<?php\n$b = [1, 2, 3];\n"
		typ := resolveTyped("$b", source, 2)
		if typ != "array<int, int>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("indexed string array", func(t *testing.T) {
		source := "<?php\n$c = ['a', 'b', 'c'];\n"
		typ := resolveTyped("$c", source, 2)
		if typ != "array<int, string>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("array of shapes", func(t *testing.T) {
		source := "<?php\n$d = [['id' => 1, 'name' => 'A'], ['id' => 2, 'name' => 'B']];\n"
		typ := resolveTyped("$d", source, 2)
		if typ != "array<int, array{id: int, name: string}>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		source := "<?php\n$e = [];\n"
		typ := resolveTyped("$e", source, 2)
		if typ != "array" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("multi-line array of shapes", func(t *testing.T) {
		source := `<?php
$employees = [
    ['id' => 1, 'name' => 'Ruben R', 'role' => 'admin'],
    ['id' => 2, 'name' => 'Jorge M', 'role' => 'admin'],
];
`
		typ := resolveTyped("$employees", source, 5)
		if typ != "array<int, array{id: int, name: string, role: string}>" {
			t.Errorf("got %q", typ)
		}
	})
}

// TestE2EConstructorGenericPropagation tests that new Collection($arr) infers
// generic params from the constructor argument using real vendor files.
func TestE2EConstructorGenericPropagation(t *testing.T) {
	p := setupLaravelE2E(t)

	resolveTyped := func(expr, source string, line int) string {
		file := parser.ParseFile(source)
		rt := p.ResolveExpressionTypeTyped(expr, source, protocol.Position{Line: line}, file)
		return rt.String()
	}

	t.Run("new Collection with indexed int array", func(t *testing.T) {
		source := "<?php\nuse Illuminate\\Support\\Collection;\n$col = new Collection([1, 2, 3]);\n"
		typ := resolveTyped("$col", source, 3)
		if typ != "Illuminate\\Support\\Collection<int, int>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("new Collection with variable arg", func(t *testing.T) {
		source := `<?php
use Illuminate\Support\Collection;
$employees = [
    ['id' => 1, 'name' => 'Ruben R', 'role' => 'admin'],
    ['id' => 2, 'name' => 'Jorge M', 'role' => 'admin'],
];
$col = new Collection($employees);
`
		typ := resolveTyped("$col", source, 7)
		if typ != "Illuminate\\Support\\Collection<int, array{id: int, name: string, role: string}>" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("$col->first() returns element type", func(t *testing.T) {
		source := `<?php
use Illuminate\Support\Collection;
$employees = [
    ['id' => 1, 'name' => 'Ruben'],
    ['id' => 2, 'name' => 'Jorge'],
];
$col = new Collection($employees);
$first = $col->first();
`
		typ := resolveTyped("$first", source, 8)
		// first() returns TValue which should be the array shape
		if typ != "array{id: int, name: string}" && typ != "?array{id: int, name: string}" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("new Collection with empty array", func(t *testing.T) {
		source := "<?php\nuse Illuminate\\Support\\Collection;\n$col = new Collection([]);\n"
		typ := resolveTyped("$col", source, 3)
		// Empty array can't infer generics
		if typ != "Illuminate\\Support\\Collection" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("completions after $col-> show Collection methods", func(t *testing.T) {
		source := "<?php\nuse Illuminate\\Support\\Collection;\n$col = new Collection([1, 2]);\n$col->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 6})
		labels := collectLabels(items)

		if !labels["first"] && !labels["all"] && !labels["count"] {
			t.Errorf("expected Collection methods on $col->, got: %v", mapKeys(labels, 10))
		}
	})
}

// TestE2EMethodTemplateInference tests that method-level @template params are
// inferred from call arguments (e.g., Arr::first($array) and Collection::first()).
func TestE2EMethodTemplateInference(t *testing.T) {
	p := setupLaravelE2E(t)

	resolveTyped := func(expr, source string, line int) string {
		file := parser.ParseFile(source)
		rt := p.ResolveExpressionTypeTyped(expr, source, protocol.Position{Line: line}, file)
		return rt.String()
	}

	t.Run("Arr::first() infers TValue from array arg", func(t *testing.T) {
		source := `<?php
use Illuminate\Support\Arr;
$employees = [['id' => 1, 'name' => 'Ruben'], ['id' => 2, 'name' => 'Jorge']];
$first = Arr::first($employees);
`
		typ := resolveTyped("$first", source, 4)
		if typ != "array{id: int, name: string}" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Arr::first() with simple int array", func(t *testing.T) {
		source := `<?php
use Illuminate\Support\Arr;
$nums = [10, 20, 30];
$first = Arr::first($nums);
`
		typ := resolveTyped("$first", source, 4)
		if typ != "int" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Collection->first() returns element type from constructor", func(t *testing.T) {
		source := `<?php
use Illuminate\Support\Collection;
$col = new Collection([['id' => 1, 'name' => 'Ruben'], ['id' => 2, 'name' => 'Jorge']]);
$first = $col->first();
`
		typ := resolveTyped("$first", source, 4)
		if typ != "array{id: int, name: string}" && typ != "?array{id: int, name: string}" {
			t.Errorf("got %q", typ)
		}
	})

	t.Run("Full chain: literal → Collection → first()", func(t *testing.T) {
		source := `<?php
use Illuminate\Support\Collection;
$employeesArr = [
    ['id' => 1, 'name' => 'Ruben R', 'role' => 'admin'],
    ['id' => 2, 'name' => 'Jorge M', 'role' => 'admin'],
];
$col = new Collection($employeesArr);
$firstEmployee = $col->first();
`
		arrTyp := resolveTyped("$employeesArr", source, 7)
		if arrTyp != "array<int, array{id: int, name: string, role: string}>" {
			t.Errorf("$employeesArr: got %q", arrTyp)
		}

		colTyp := resolveTyped("$col", source, 7)
		if colTyp != "Illuminate\\Support\\Collection<int, array{id: int, name: string, role: string}>" {
			t.Errorf("$col: got %q", colTyp)
		}

		firstTyp := resolveTyped("$firstEmployee", source, 8)
		if firstTyp != "array{id: int, name: string, role: string}" && firstTyp != "?array{id: int, name: string, role: string}" {
			t.Errorf("$firstEmployee: got %q", firstTyp)
		}
	})
}
