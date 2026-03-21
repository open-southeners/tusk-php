package parser

import (
	"testing"
)

func TestParseIncompleteClass(t *testing.T) {
	// Missing closing brace — should still parse the class and subsequent code
	source := `<?php
namespace App;

class Foo {
    public string $name;
    public function bar(): void {}

class Baz {
    public function qux(): int {}
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result for incomplete class")
	}
	if len(result.Classes) < 2 {
		t.Fatalf("expected 2 classes, got %d", len(result.Classes))
	}
	if result.Classes[0].Name != "Foo" {
		t.Errorf("expected first class 'Foo', got %q", result.Classes[0].Name)
	}
	if result.Classes[1].Name != "Baz" {
		t.Errorf("expected second class 'Baz', got %q", result.Classes[1].Name)
	}
	if len(result.Errors) == 0 {
		t.Error("expected parse errors for missing brace")
	}
}

func TestParseIncompleteParams(t *testing.T) {
	// Missing closing paren — should recover and parse the rest
	source := `<?php
class Foo {
    public function bar(int $x, string $y {
        return $x;
    }
    public function baz(): void {}
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Classes) == 0 {
		t.Fatal("expected at least 1 class")
	}
	cls := result.Classes[0]
	if len(cls.Methods) < 1 {
		t.Fatal("expected at least 1 method")
	}
	// The first method should still have the params that were parsed before the bail
	m := cls.Methods[0]
	if m.Name != "bar" {
		t.Errorf("expected method 'bar', got %q", m.Name)
	}
	if len(m.Params) < 2 {
		t.Errorf("expected at least 2 params, got %d", len(m.Params))
	}
	if len(result.Errors) == 0 {
		t.Error("expected parse errors for missing paren")
	}
}

func TestParseUnterminatedString(t *testing.T) {
	source := `<?php
$x = "hello world
class Foo {
    public function bar(): void {}
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("expected parse error for unterminated string")
	}
}

func TestParseUnterminatedComment(t *testing.T) {
	source := `<?php
/* this comment is never closed
class Foo {
    public function bar(): void {}
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("expected parse error for unterminated comment")
	}
}

func TestParseUnterminatedDocComment(t *testing.T) {
	source := `<?php
/** this doc comment is never closed
class Foo {}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Errors) == 0 {
		t.Error("expected parse error for unterminated doc comment")
	}
}

func TestParseEmptySource(t *testing.T) {
	result := New().Parse("")
	if result == nil {
		t.Fatal("expected non-nil result for empty source")
	}
}

func TestParseValidCodeNoErrors(t *testing.T) {
	source := `<?php
namespace App;

use Monolog\Logger;

class Service {
    private Logger $logger;

    public function __construct(Logger $logger) {
        $this->logger = $logger;
    }

    public function run(): void {
        $this->logger->info("running");
    }
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors for valid code, got %d: %v", len(result.Errors), result.Errors)
	}
	if result.Namespace != "App" {
		t.Errorf("expected namespace 'App', got %q", result.Namespace)
	}
	if len(result.Classes) != 1 || result.Classes[0].Name != "Service" {
		t.Errorf("expected class 'Service'")
	}
}

func TestParseFilePartialResultOnPanic(t *testing.T) {
	// Valid code should always return a non-nil FileNode
	source := `<?php
namespace App;

class Foo {
    public string $name;
}
`
	file := ParseFile(source)
	if file == nil {
		t.Fatal("expected non-nil FileNode")
	}
	if file.Namespace != "App" {
		t.Errorf("expected namespace 'App', got %q", file.Namespace)
	}
	if len(file.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(file.Classes))
	}
}

func TestParseMissingBraceBeforeInterface(t *testing.T) {
	// Missing closing brace before interface — should recover
	source := `<?php
class Foo {
    public function bar(): void {}

interface Bar {
    public function baz(): string;
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Classes) < 1 {
		t.Fatal("expected at least 1 class")
	}
	if len(result.Interfaces) < 1 {
		t.Fatal("expected at least 1 interface — recovery should have detected the missing brace")
	}
	if result.Interfaces[0].Name != "Bar" {
		t.Errorf("expected interface 'Bar', got %q", result.Interfaces[0].Name)
	}
}

func TestParseAnonymousClassNotFalseRecovery(t *testing.T) {
	// 'new class' inside a method body should NOT trigger recovery
	source := `<?php
class Foo {
    public function bar(): object {
        return new class {
            public function inner(): void {}
        };
    }
    public function baz(): void {}
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Classes) != 1 {
		t.Errorf("expected 1 top-level class, got %d", len(result.Classes))
	}
}

func TestParseProgressGuardPreventsHang(t *testing.T) {
	// Garbage input should not cause infinite loop — parser should terminate
	source := `<?php @@@ $$$ %%% ^^^ &&& *** ))) ((( !!! ~~~`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result for garbage input")
	}
}

func TestParseDocBlockStructuredParams(t *testing.T) {
	raw := `/**
	 * Process the input data.
	 *
	 * @param string $name The user name
	 * @param int $age The user age
	 * @return bool Whether processing succeeded
	 * @throws ValidationException When input is invalid
	 */`
	doc := ParseDocBlock(raw)
	if doc == nil {
		t.Fatal("expected non-nil docblock")
	}
	if doc.Summary != "Process the input data." {
		t.Errorf("unexpected summary: %q", doc.Summary)
	}
	if len(doc.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(doc.Params))
	}
	if doc.Params[0].Type != "string" || doc.Params[0].Name != "$name" {
		t.Errorf("unexpected param 0: %+v", doc.Params[0])
	}
	if doc.Params[0].Description != "The user name" {
		t.Errorf("unexpected param 0 desc: %q", doc.Params[0].Description)
	}
	if doc.Params[1].Type != "int" || doc.Params[1].Name != "$age" {
		t.Errorf("unexpected param 1: %+v", doc.Params[1])
	}
	if doc.Return.Type != "bool" {
		t.Errorf("unexpected return type: %q", doc.Return.Type)
	}
	if doc.Return.Description != "Whether processing succeeded" {
		t.Errorf("unexpected return desc: %q", doc.Return.Description)
	}
	if len(doc.Throws) != 1 {
		t.Fatalf("expected 1 throw, got %d", len(doc.Throws))
	}
	if doc.Throws[0].Type != "ValidationException" {
		t.Errorf("unexpected throw type: %q", doc.Throws[0].Type)
	}
}

func TestParseDocBlockDeprecated(t *testing.T) {
	raw := `/**
	 * Old method.
	 * @deprecated Use newMethod() instead.
	 */`
	doc := ParseDocBlock(raw)
	if doc == nil {
		t.Fatal("expected non-nil docblock")
	}
	if !doc.Deprecated {
		t.Error("expected Deprecated to be true")
	}
	if doc.DeprecatedMsg != "Use newMethod() instead." {
		t.Errorf("unexpected deprecated msg: %q", doc.DeprecatedMsg)
	}
}

func TestParseDocBlockTemplateMixinSee(t *testing.T) {
	raw := `/**
	 * A generic collection.
	 * @template T
	 * @mixin \Illuminate\Support\Collection
	 * @see https://example.com/docs
	 * @property-read int $count
	 */`
	doc := ParseDocBlock(raw)
	if doc == nil {
		t.Fatal("expected non-nil docblock")
	}
	if vals, ok := doc.Tags["template"]; !ok || len(vals) == 0 || vals[0] != "T" {
		t.Errorf("unexpected template tag: %v", doc.Tags["template"])
	}
	if vals, ok := doc.Tags["mixin"]; !ok || len(vals) == 0 {
		t.Errorf("unexpected mixin tag: %v", doc.Tags["mixin"])
	}
	if vals, ok := doc.Tags["see"]; !ok || len(vals) == 0 {
		t.Errorf("unexpected see tag: %v", doc.Tags["see"])
	}
	if vals, ok := doc.Tags["property-read"]; !ok || len(vals) == 0 {
		t.Errorf("unexpected property-read tag: %v", doc.Tags["property-read"])
	}
}

func TestParseDocBlockEmptyDeprecated(t *testing.T) {
	raw := `/**
	 * @deprecated
	 */`
	doc := ParseDocBlock(raw)
	if doc == nil {
		t.Fatal("expected non-nil docblock")
	}
	if !doc.Deprecated {
		t.Error("expected Deprecated to be true")
	}
	if doc.DeprecatedMsg != "" {
		t.Errorf("expected empty deprecated msg, got: %q", doc.DeprecatedMsg)
	}
}

func TestParseReservedWordMethodNames(t *testing.T) {
	source := `<?php
namespace App;

class Foo {
    public function class(): string { return ''; }
    public function match(): bool { return true; }
    public function new(): self { return new self(); }
    public function return(): void {}
    public function normal(): int { return 0; }
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(result.Classes))
	}
	cls := result.Classes[0]
	if len(cls.Methods) != 5 {
		t.Fatalf("expected 5 methods, got %d", len(cls.Methods))
	}
	expected := []string{"class", "match", "new", "return", "normal"}
	for i, name := range expected {
		if cls.Methods[i].Name != name {
			t.Errorf("method %d: expected %q, got %q", i, name, cls.Methods[i].Name)
		}
	}
	// No methods should leak as top-level functions
	if len(result.Functions) != 0 {
		t.Errorf("expected 0 top-level functions, got %d", len(result.Functions))
	}
}

func TestParseTraitUseWithConflictResolution(t *testing.T) {
	source := `<?php
namespace Illuminate\Database\Eloquent;

class Builder {
    use BuildsQueries, ForwardsCalls, QueriesRelationships {
        BuildsQueries::sole as baseSole;
    }

    protected $query;
    protected $model;

    public function with($relations, $callback = null) {
        return $this;
    }

    public function where($column, $operator = null, $value = null) {
        return $this;
    }

    public function first($columns = ['*']) {
        return null;
    }
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(result.Classes))
	}
	cls := result.Classes[0]
	if cls.Name != "Builder" {
		t.Errorf("expected class 'Builder', got %q", cls.Name)
	}
	if len(cls.Methods) < 3 {
		t.Errorf("expected at least 3 methods (with, where, first), got %d", len(cls.Methods))
		for _, m := range cls.Methods {
			t.Logf("  method: %s", m.Name)
		}
	}
}
