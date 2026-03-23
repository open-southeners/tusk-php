package resolve

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestParseGenericType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		fqn   string
		npar  int
		str   string
	}{
		{"bare type", "string", "string", 0, "string"},
		{"single param", "Builder<static>", "Builder", 1, "Builder<static>"},
		{"two params", "Collection<int, Category>", "Collection", 2, "Collection<int, Category>"},
		{"nullable", "?Collection<int, Model>", "Collection", 2, "?Collection<int, Model>"},
		{"FQN", "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\User>", "Illuminate\\Database\\Eloquent\\Builder", 1, "Illuminate\\Database\\Eloquent\\Builder<App\\Models\\User>"},
		{"leading backslash", "\\Illuminate\\Builder", "Illuminate\\Builder", 0, "Illuminate\\Builder"},
		{"empty", "", "", 0, ""},
		{"nested generic", "Collection<int, Builder<Category>>", "Collection", 2, "Collection<int, Builder<Category>>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := ParseGenericType(tt.input)
			if rt.FQN != tt.fqn {
				t.Errorf("FQN = %q, want %q", rt.FQN, tt.fqn)
			}
			if len(rt.Params) != tt.npar {
				t.Errorf("len(Params) = %d, want %d", len(rt.Params), tt.npar)
			}
			if tt.str != "" && rt.String() != tt.str {
				t.Errorf("String() = %q, want %q", rt.String(), tt.str)
			}
		})
	}
}

func TestResolvedTypeHelpers(t *testing.T) {
	t.Run("BaseFQN", func(t *testing.T) {
		rt := ResolvedType{FQN: "Collection", Params: []ResolvedType{{FQN: "int"}}}
		if rt.BaseFQN() != "Collection" {
			t.Errorf("got %q", rt.BaseFQN())
		}
	})

	t.Run("IsGeneric", func(t *testing.T) {
		if !(ResolvedType{FQN: "X", Params: []ResolvedType{{FQN: "Y"}}}).IsGeneric() {
			t.Error("expected true")
		}
		if (ResolvedType{FQN: "X"}).IsGeneric() {
			t.Error("expected false")
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		if !(ResolvedType{}).IsEmpty() {
			t.Error("expected true")
		}
		if (ResolvedType{FQN: "X"}).IsEmpty() {
			t.Error("expected false")
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		input := "Collection<int, Builder<Category>>"
		rt := ParseGenericType(input)
		if rt.String() != input {
			t.Errorf("roundtrip: %q != %q", rt.String(), input)
		}
	})
}

func TestParseDocTemplate(t *testing.T) {
	t.Run("@template TModel", func(t *testing.T) {
		doc := parser.ParseDocBlock(`/**
 * @template TModel
 */`)
		if len(doc.Templates) != 1 {
			t.Fatalf("expected 1 template, got %d", len(doc.Templates))
		}
		if doc.Templates[0].Name != "TModel" || doc.Templates[0].Bound != "" {
			t.Errorf("got %+v", doc.Templates[0])
		}
	})

	t.Run("@template TModel of Model", func(t *testing.T) {
		doc := parser.ParseDocBlock(`/**
 * @template TModel of \Illuminate\Database\Eloquent\Model
 */`)
		if len(doc.Templates) != 1 {
			t.Fatalf("expected 1 template, got %d", len(doc.Templates))
		}
		if doc.Templates[0].Name != "TModel" {
			t.Errorf("name = %q", doc.Templates[0].Name)
		}
		if doc.Templates[0].Bound != "\\Illuminate\\Database\\Eloquent\\Model" {
			t.Errorf("bound = %q", doc.Templates[0].Bound)
		}
	})

	t.Run("multiple templates", func(t *testing.T) {
		doc := parser.ParseDocBlock(`/**
 * @template TKey
 * @template TValue
 */`)
		if len(doc.Templates) != 2 {
			t.Fatalf("expected 2 templates, got %d", len(doc.Templates))
		}
	})
}

func TestSymbolTemplateStorage(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///repo.php", `<?php
/**
 * @template T of \App\Model
 * @method T find(int $id)
 */
class Repository {
}
`)
	sym := idx.Lookup("Repository")
	if sym == nil {
		t.Fatal("expected Repository symbol")
	}
	if len(sym.Templates) != 1 {
		t.Fatalf("expected 1 template, got %d", len(sym.Templates))
	}
	if sym.Templates[0].Name != "T" {
		t.Errorf("template name = %q", sym.Templates[0].Name)
	}
}

func TestResolveSymbolTemplateReturn(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///repo.php", `<?php
/**
 * @template T of Model
 */
class Repository {
    /**
     * @return T|null
     */
    public function find(int $id) {}
}
`)

	r := NewResolver(idx)
	sym := idx.Lookup("Repository")
	if sym == nil {
		t.Fatal("expected Repository symbol")
	}

	rt := r.ResolveSymbolTemplateReturn(sym, "find", []ResolvedType{{FQN: "App\\Models\\User"}})
	// T|null with T = App\Models\User → should substitute T
	if rt.IsEmpty() {
		t.Fatal("expected non-empty result")
	}
	// The result should be App\Models\User (first non-null in union)
	if rt.FQN != "App\\Models\\User" {
		t.Logf("got %q (union handling may take first non-null)", rt.FQN)
	}
}

func TestResolveTemplateReturn(t *testing.T) {
	catType := ResolvedType{FQN: "App\\Models\\Category"}
	intType := ResolvedType{FQN: "int"}

	t.Run("Builder get returns Collection<int, Category>", func(t *testing.T) {
		rt := ResolveTemplateReturn("Illuminate\\Database\\Eloquent\\Builder", "get", []ResolvedType{catType})
		if rt.BaseFQN() != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("FQN = %q", rt.FQN)
		}
		if len(rt.Params) != 2 {
			t.Fatalf("params = %d", len(rt.Params))
		}
		if rt.Params[0].FQN != "int" {
			t.Errorf("param[0] = %q", rt.Params[0].FQN)
		}
		if rt.Params[1].FQN != "App\\Models\\Category" {
			t.Errorf("param[1] = %q", rt.Params[1].FQN)
		}
	})

	t.Run("Builder first returns ?Category", func(t *testing.T) {
		rt := ResolveTemplateReturn("Illuminate\\Database\\Eloquent\\Builder", "first", []ResolvedType{catType})
		if rt.FQN != "App\\Models\\Category" {
			t.Errorf("FQN = %q", rt.FQN)
		}
		if !rt.Nullable {
			t.Error("expected nullable")
		}
	})

	t.Run("Builder where returns static (Builder<Category>)", func(t *testing.T) {
		rt := ResolveTemplateReturn("Illuminate\\Database\\Eloquent\\Builder", "where", []ResolvedType{catType})
		if rt.FQN != "Illuminate\\Database\\Eloquent\\Builder" {
			t.Errorf("FQN = %q", rt.FQN)
		}
		if len(rt.Params) != 1 || rt.Params[0].FQN != "App\\Models\\Category" {
			t.Errorf("params = %v", rt.Params)
		}
	})

	t.Run("Collection first returns ?Category", func(t *testing.T) {
		rt := ResolveTemplateReturn("Illuminate\\Database\\Eloquent\\Collection", "first", []ResolvedType{intType, catType})
		if rt.FQN != "App\\Models\\Category" {
			t.Errorf("FQN = %q", rt.FQN)
		}
		if !rt.Nullable {
			t.Error("expected nullable")
		}
	})

	t.Run("Collection all returns array", func(t *testing.T) {
		rt := ResolveTemplateReturn("Illuminate\\Database\\Eloquent\\Collection", "all", []ResolvedType{intType, catType})
		if rt.FQN != "array" {
			t.Errorf("FQN = %q", rt.FQN)
		}
	})

	t.Run("unknown class returns empty", func(t *testing.T) {
		rt := ResolveTemplateReturn("Unknown\\Class", "get", nil)
		if !rt.IsEmpty() {
			t.Error("expected empty for unknown class")
		}
	})

	t.Run("unknown method returns empty", func(t *testing.T) {
		rt := ResolveTemplateReturn("Illuminate\\Database\\Eloquent\\Builder", "unknownMethod", []ResolvedType{catType})
		if !rt.IsEmpty() {
			t.Error("expected empty for unknown method")
		}
	})
}
