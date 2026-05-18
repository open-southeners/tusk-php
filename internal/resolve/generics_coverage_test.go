package resolve

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
	internaltypes "github.com/open-southeners/tusk-php/internal/types"
)

func setupGenericResolverCoverage(t *testing.T) (*Resolver, *symbols.Index, string, *parser.FileNode, []string) {
	t.Helper()

	source := `<?php
namespace App;

class Logger {}

/**
 * @template TKey
 * @template TValue
 */
class Collection {}

/**
 * @template TModel
 */
class Repository {
    /** @return TModel|null */
    public function first() {}

    /** @return Collection<int, TModel>|null */
    public function all() {}

    /** @return static|null */
    public function fluent() {}
}

class Service {
    public function run(Logger $logger) {
        $numbers = [1, 2, 3];
        $rows = [
            ['id' => 1],
            ['id' => 2],
        ];
        $firstNumber = $numbers[0];
        $firstRow = $rows[0];
        $collection = new Collection($numbers);
        $fromChain = Factory::make();
        $fromFallback = Factory::fallback();
        $logger;
        $this->run($logger);
    }
}
`

	idx := symbols.NewIndex()
	idx.IndexFile("file:///coverage.php", source)

	file := parser.ParseFile(source)
	if file == nil {
		t.Fatal("expected parsed file")
	}

	return NewResolver(idx), idx, source, file, SplitLines(source)
}

func lineContaining(t *testing.T, lines []string, needle string) int {
	t.Helper()
	for i, line := range lines {
		if strings.Contains(line, needle) {
			return i
		}
	}
	t.Fatalf("line containing %q not found", needle)
	return -1
}

func TestArrayLiteralCoverageHelpers(t *testing.T) {
	t.Run("shape arrays keep shape string", func(t *testing.T) {
		rt := InferArrayLiteralType(`['id' => 1, 'name' => 'desk']`)
		if rt.String() != "array{id: int, name: string}" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("indexed arrays infer scalar value type", func(t *testing.T) {
		rt := InferArrayLiteralType(`[1, 2, 3]`)
		if rt.String() != "array<int, int>" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("arrays of shapes preserve nested shape", func(t *testing.T) {
		rt := InferArrayLiteralType(`[['id' => 1], ['id' => 2]]`)
		if rt.String() != "array<int, array{id: int}>" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("mixed indexed arrays fall back to bare array", func(t *testing.T) {
		rt := InferArrayLiteralType(`[1, 'x']`)
		if rt.String() != "array" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("shape string builder and nullable shape serialization", func(t *testing.T) {
		shape := buildShapeString([]internaltypes.ShapeField{
			{Key: "id", Type: "int"},
			{Key: "name", Type: "string"},
		})
		if shape != "array{id: int, name: string}" {
			t.Fatalf("got %q", shape)
		}

		rt := ResolvedType{FQN: "array", Shape: shape, Nullable: true}
		if rt.String() != "?array{id: int, name: string}" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("splitArrayValues respects nesting and strings", func(t *testing.T) {
		values := splitArrayValues(`'a,b', [1, 2], foo(1, 2)`)
		if len(values) != 3 {
			t.Fatalf("expected 3 values, got %d (%v)", len(values), values)
		}
		if values[0] != "'a,b'" || values[1] != "[1, 2]" || values[2] != "foo(1, 2)" {
			t.Fatalf("unexpected split values: %v", values)
		}
	})

	t.Run("empty shape produces empty string", func(t *testing.T) {
		if buildShapeString(nil) != "" {
			t.Fatal("expected empty shape string")
		}
	})
}

func TestGenericResolverHelperCoverage(t *testing.T) {
	r, idx, _, file, lines := setupGenericResolverCoverage(t)

	repoFirst := idx.Lookup("App\\Repository::first")
	repoAll := idx.Lookup("App\\Repository::all")
	repoFluent := idx.Lookup("App\\Repository::fluent")
	if repoFirst == nil || repoAll == nil || repoFluent == nil {
		t.Fatal("expected repository methods to be indexed")
	}

	t.Run("member type resolved handles bare template unions", func(t *testing.T) {
		rt := r.MemberTypeResolved(repoFirst, file, "", []ResolvedType{{FQN: "App\\Logger"}})
		if rt.String() != "?App\\Logger" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("member type resolved handles generic unions", func(t *testing.T) {
		rt := r.MemberTypeResolved(repoAll, file, "", []ResolvedType{{FQN: "App\\Logger"}})
		if rt.String() != "?App\\Collection<int, App\\Logger>" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("member type resolved preserves static caller and context", func(t *testing.T) {
		rt := r.MemberTypeResolved(repoFluent, file, "App\\Repository", []ResolvedType{{FQN: "App\\Logger"}})
		if rt.FQN != "App\\Repository" {
			t.Fatalf("FQN = %q", rt.FQN)
		}
		if !rt.Nullable {
			t.Fatal("expected nullable result")
		}
		if len(rt.Params) != 1 || rt.Params[0].FQN != "App\\Logger" {
			t.Fatalf("unexpected params: %#v", rt.Params)
		}
	})

	t.Run("member type resolved falls back to known template registry", func(t *testing.T) {
		member := &symbols.Symbol{
			Kind:      symbols.KindMethod,
			ParentFQN: "Illuminate\\Database\\Eloquent\\Builder",
			Name:      "first",
		}
		rt := r.MemberTypeResolved(member, file, "", []ResolvedType{{FQN: "App\\Logger"}})
		if rt.String() != "?App\\Logger" {
			t.Fatalf("got %q", rt.String())
		}
	})

	t.Run("template substitution helpers resolve names", func(t *testing.T) {
		subst := buildTemplateSubst(idx, "App\\Repository", []ResolvedType{{FQN: "App\\Logger"}})
		if subst["TModel"].FQN != "App\\Logger" {
			t.Fatalf("unexpected substitution map: %#v", subst)
		}

		if got := resolveTypeParam(r, "TModel", "App\\Repository", subst, file); got != "App\\Logger" {
			t.Fatalf("got %q", got)
		}
		if got := resolveTypeParam(r, "static", "App\\Repository", subst, file); got != "App\\Repository" {
			t.Fatalf("got %q", got)
		}
		if got := resolveStaticOrClassName(r, "Logger", "App\\Repository", file); got != "App\\Logger" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("union helpers respect generic branches and null", func(t *testing.T) {
		union := `Collection<int, App\Logger>|App\Logger|null`
		if got := PickBestUnionPart(union); got != `Collection<int, App\Logger>` {
			t.Fatalf("got %q", got)
		}
		if !HasNullInUnion(union) {
			t.Fatal("expected null in union")
		}
		if HasNullInUnion(`Collection<int, App\Logger>|App\Logger`) {
			t.Fatal("did not expect null in union")
		}
	})

	t.Run("argument and constructor helpers infer generic arrays", func(t *testing.T) {
		pos := protocol.Position{Line: lineContaining(t, lines, "$collection =")}
		argRT := r.inferArgType(`$numbers, $ignored`, lines, pos, file, 0)
		if argRT.String() != "array<int, int>" {
			t.Fatalf("got %q", argRT.String())
		}

		literalRT := r.inferArgType(`[1, 2], $ignored`, lines, pos, file, 0)
		if literalRT.String() != "array<int, int>" {
			t.Fatalf("got %q", literalRT.String())
		}

		constructorRT := r.inferConstructorGenerics("App\\Collection", argRT)
		if constructorRT.String() != "App\\Collection<int, int>" {
			t.Fatalf("got %q", constructorRT.String())
		}
	})

	t.Run("misc helpers cover raw member return type and comma scan", func(t *testing.T) {
		if got := r.rawMemberReturnType(repoAll); got != "Collection<int, TModel>|null" {
			t.Fatalf("got %q", got)
		}

		input := `foo([1, 2], "a,b"), $rest`
		idx := IndexOutsideBrackets(input, ',')
		if idx < 0 || input[idx:] != ", $rest" {
			t.Fatalf("unexpected index %d for %q", idx, input)
		}
	})
}

func TestResolveVariableTypeTypedCoverage(t *testing.T) {
	r, _, _, file, lines := setupGenericResolverCoverage(t)

	r.TypedChainResolver = func(expr string, source string, pos protocol.Position, file *parser.FileNode) ResolvedType {
		if expr == "Factory::make()" {
			return ResolvedType{
				FQN:    "App\\Collection",
				Params: []ResolvedType{{FQN: "int"}, {FQN: "App\\Logger"}},
			}
		}
		return ResolvedType{}
	}
	r.ChainResolver = func(expr string, source string, pos protocol.Position, file *parser.FileNode) string {
		if expr == "Factory::fallback()" {
			return "App\\Logger"
		}
		return ""
	}

	assertType := func(varName, lineNeedle, want string) {
		t.Helper()
		pos := protocol.Position{Line: lineContaining(t, lines, lineNeedle)}
		rt := r.ResolveVariableTypeTyped(varName, lines, pos, file)
		got := rt.String()
		if got == "" {
			got = rt.FQN
		}
		if got != want {
			t.Fatalf("%s at %q = %q, want %q", varName, lineNeedle, got, want)
		}
	}

	assertType("$logger", "$logger;", "App\\Logger")
	assertType("$numbers", "$firstNumber =", "array<int, int>")
	assertType("$rows", "$firstRow =", "array<int, array{id: int}>")
	assertType("$firstNumber", "$firstNumber =", "int")
	assertType("$firstRow", "$firstRow =", "array{id: int}")
	assertType("$collection", "$collection =", "App\\Collection<int, int>")
	assertType("$fromChain", "$fromChain =", "App\\Collection<int, App\\Logger>")
	assertType("$fromFallback", "$fromFallback =", "App\\Logger")

	if got := r.ResolveVariableTypeTyped("$missing", nil, protocol.Position{}, nil); !got.IsEmpty() {
		t.Fatalf("expected empty type, got %#v", got)
	}
}

func TestResolveVariableTypeTypedThisCoverage(t *testing.T) {
	source := `<?php
namespace App;

class Service {
    public function run() {
        $this->run();
    }
}
`

	idx := symbols.NewIndex()
	idx.IndexFile("file:///this.php", source)

	r := NewResolver(idx)
	file := parser.ParseFile(source)
	if file == nil {
		t.Fatal("expected parsed file")
	}

	rt := r.ResolveVariableTypeTyped("$this", SplitLines(source), protocol.Position{Line: 5}, file)
	if rt.FQN != "App\\Service" {
		t.Fatalf("got %q", rt.FQN)
	}
}

func TestResolveVariableTypeTypedDocblockParamGeneric(t *testing.T) {
	source := `<?php
namespace App\Http\Controllers;

use App\Models\Category;
use OpenSoutheners\LaravelApiable\Http\JsonApiResponse;

class CategoryController {
    /**
     * @param JsonApiResponse<Category> $response
     */
    #[Route('/categories')]
    public function index(JsonApiResponse $response): mixed {
        return $response;
    }
}
`

	idx := symbols.NewIndex()
	idx.IndexFile("file:///JsonApiResponse.php", `<?php
namespace OpenSoutheners\LaravelApiable\Http;
/**
 * @template T
 */
class JsonApiResponse {}
`)
	idx.IndexFile("file:///Category.php", `<?php
namespace App\Models;
class Category {}
`)
	idx.IndexFile("file:///controller.php", source)

	r := NewResolver(idx)
	file := parser.ParseFile(source)
	if file == nil {
		t.Fatal("expected parsed file")
	}

	pos := protocol.Position{Line: 11}
	rt := r.ResolveVariableTypeTyped("$response", SplitLines(source), pos, file)
	if rt.String() != "OpenSoutheners\\LaravelApiable\\Http\\JsonApiResponse<App\\Models\\Category>" {
		t.Fatalf("got %q", rt.String())
	}

	if got := r.ResolveVariableType("$response", SplitLines(source), pos, file); got != "OpenSoutheners\\LaravelApiable\\Http\\JsonApiResponse" {
		t.Fatalf("plain type got %q", got)
	}
}
