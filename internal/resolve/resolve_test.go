package resolve

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupResolver() *Resolver {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Logger.php", `<?php
namespace Monolog;
class Logger {
    public function info(string $message): void {}
    public function error(string $message): void {}
}
`)
	idx.IndexFile("file:///app/Service.php", `<?php
namespace App;
use Monolog\Logger;
class Service {
    private Logger $logger;
    public function run(): string { return ""; }
    /** @return static */
    public function fluent() {}
    /** @return $this */
    public function chain() {}
    /** @var string */
    public $name;
    /** @return \Monolog\Logger|null */
    public function getLogger() {}
    /** @return Builder<static> */
    public function query() {}
}
`)
	return NewResolver(idx)
}

func TestNewResolver(t *testing.T) {
	idx := symbols.NewIndex()
	r := NewResolver(idx)
	if r == nil || r.Index != idx {
		t.Fatal("NewResolver should create resolver with index")
	}
}

func TestResolveClassName(t *testing.T) {
	r := setupResolver()
	file := parser.ParseFile(`<?php
namespace App;
use Monolog\Logger;
class Foo {}
`)

	tests := []struct {
		name, input, want string
	}{
		{"empty", "", ""},
		{"via use statement", "Logger", "Monolog\\Logger"},
		{"FQN with backslash", "\\Monolog\\Logger", "Monolog\\Logger"},
		{"nullable stripped", "?Logger", "Monolog\\Logger"},
		{"in current namespace", "Service", "App\\Service"},
		{"unknown stays as-is", "Unknown", "Unknown"},
		{"nil file returns name", "Foo", "Foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := file
			if tt.name == "nil file returns name" {
				f = nil
			}
			got := r.ResolveClassName(tt.input, f)
			if got != tt.want {
				t.Errorf("ResolveClassName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindEnclosingClass(t *testing.T) {
	file := parser.ParseFile(`<?php
namespace App;
class Foo {
    public function bar() {}
}
class Baz {}
`)

	t.Run("inside class", func(t *testing.T) {
		fqn := FindEnclosingClass(file, protocol.Position{Line: 3})
		if fqn != "App\\Foo" {
			t.Errorf("got %q, want App\\Foo", fqn)
		}
	})

	t.Run("nil file", func(t *testing.T) {
		if FindEnclosingClass(nil, protocol.Position{Line: 0}) != "" {
			t.Error("nil file should return empty")
		}
	})

	t.Run("before any class", func(t *testing.T) {
		fqn := FindEnclosingClass(file, protocol.Position{Line: 0})
		if fqn != "" {
			t.Errorf("before class should be empty, got %q", fqn)
		}
	})
}

func TestFindEnclosingMethod(t *testing.T) {
	file := parser.ParseFile(`<?php
class Foo {
    public function bar() {
        echo "hello";
    }
    public function baz() {}
}
`)

	t.Run("inside method", func(t *testing.T) {
		m := FindEnclosingMethod(file, protocol.Position{Line: 3})
		if m == nil || m.Name != "bar" {
			t.Errorf("expected bar, got %v", m)
		}
	})

	t.Run("nil file", func(t *testing.T) {
		if FindEnclosingMethod(nil, protocol.Position{Line: 0}) != nil {
			t.Error("nil file should return nil")
		}
	})
}

func TestFindMember(t *testing.T) {
	r := setupResolver()

	t.Run("method lookup", func(t *testing.T) {
		m := r.FindMember("App\\Service", "run")
		if m == nil || m.Name != "run" {
			t.Errorf("expected run, got %v", m)
		}
	})

	t.Run("property lookup with $", func(t *testing.T) {
		m := r.FindMember("App\\Service", "$logger")
		if m == nil {
			t.Error("expected $logger member")
		}
	})

	t.Run("property lookup without $", func(t *testing.T) {
		m := r.FindMember("App\\Service", "logger")
		if m == nil {
			t.Error("expected logger member (without $)")
		}
	})

	t.Run("missing member", func(t *testing.T) {
		if r.FindMember("App\\Service", "nonexistent") != nil {
			t.Error("expected nil for missing member")
		}
	})

	t.Run("inherited member", func(t *testing.T) {
		m := r.FindMember("Monolog\\Logger", "info")
		if m == nil {
			t.Error("expected info method")
		}
	})
}

func TestMemberType(t *testing.T) {
	r := setupResolver()
	file := parser.ParseFile(`<?php
namespace App;
use Monolog\Logger;
`)

	t.Run("return type from hint", func(t *testing.T) {
		m := r.FindMember("App\\Service", "run")
		typ := r.MemberType(m, file)
		if typ != "string" {
			t.Errorf("expected string, got %q", typ)
		}
	})

	t.Run("static resolves to parent", func(t *testing.T) {
		m := r.FindMember("App\\Service", "fluent")
		typ := r.MemberType(m, file)
		if typ != "App\\Service" {
			t.Errorf("expected App\\Service, got %q", typ)
		}
	})

	t.Run("$this resolves to parent", func(t *testing.T) {
		m := r.FindMember("App\\Service", "chain")
		typ := r.MemberType(m, file)
		if typ != "App\\Service" {
			t.Errorf("expected App\\Service, got %q", typ)
		}
	})

	t.Run("property type from @var docblock", func(t *testing.T) {
		m := r.FindMember("App\\Service", "name")
		typ := r.MemberType(m, file)
		if typ != "string" {
			t.Errorf("expected string, got %q", typ)
		}
	})

	t.Run("union type takes first non-null", func(t *testing.T) {
		m := r.FindMember("App\\Service", "getLogger")
		typ := r.MemberType(m, file)
		if typ != "Monolog\\Logger" {
			t.Errorf("expected Monolog\\Logger, got %q", typ)
		}
	})

	t.Run("generic stripped", func(t *testing.T) {
		m := r.FindMember("App\\Service", "query")
		typ := r.MemberType(m, file)
		if typ != "Builder" {
			t.Errorf("expected Builder (generic stripped), got %q", typ)
		}
	})

	t.Run("void returns empty", func(t *testing.T) {
		m := r.FindMember("Monolog\\Logger", "info")
		typ := r.MemberType(m, file)
		if typ != "" {
			t.Errorf("expected empty for void, got %q", typ)
		}
	})

	t.Run("constant kind returns empty", func(t *testing.T) {
		typ := r.MemberType(&symbols.Symbol{Kind: symbols.KindConstant}, file)
		if typ != "" {
			t.Errorf("expected empty for constant, got %q", typ)
		}
	})
}

func TestResolveVariableType(t *testing.T) {
	r := setupResolver()

	t.Run("$this resolves to enclosing class", func(t *testing.T) {
		file := parser.ParseFile(`<?php
namespace App;
class Service {
    public function foo() { $this->bar(); }
}
`)
		lines := SplitLines(`<?php
namespace App;
class Service {
    public function foo() { $this->bar(); }
}
`)
		typ := r.ResolveVariableType("$this", lines, protocol.Position{Line: 3}, file)
		if typ != "App\\Service" {
			t.Errorf("expected App\\Service, got %q", typ)
		}
	})

	t.Run("parameter type", func(t *testing.T) {
		source := `<?php
namespace App;
use Monolog\Logger;
class Foo {
    public function bar(Logger $log) {
        $log->info("x");
    }
}
`
		file := parser.ParseFile(source)
		lines := SplitLines(source)
		typ := r.ResolveVariableType("$log", lines, protocol.Position{Line: 5}, file)
		if typ != "Monolog\\Logger" {
			t.Errorf("expected Monolog\\Logger, got %q", typ)
		}
	})

	t.Run("$var = new ClassName()", func(t *testing.T) {
		source := `<?php
use Monolog\Logger;
$x = new Logger();
$x->info("test");
`
		file := parser.ParseFile(source)
		lines := SplitLines(source)
		typ := r.ResolveVariableType("$x", lines, protocol.Position{Line: 3}, file)
		if typ != "Monolog\\Logger" {
			t.Errorf("expected Monolog\\Logger, got %q", typ)
		}
	})

	t.Run("nil file", func(t *testing.T) {
		if r.ResolveVariableType("$x", nil, protocol.Position{}, nil) != "" {
			t.Error("nil file should return empty")
		}
	})
}

func TestInferLiteralType(t *testing.T) {
	tests := []struct {
		expr, want string
	}{
		{"'hello'", "string"},
		{`"hello"`, "string"},
		{"true", "bool"},
		{"false", "bool"},
		{"null", "null"},
		{"[1, 2]", "array"},
		{"array(1)", "array"},
		{"42", "int"},
		{"-5", "int"},
		{"3.14", "float"},
		{"1e10", "float"},
		{"", ""},
		{"$var", ""},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			if got := InferLiteralType(tt.expr); got != tt.want {
				t.Errorf("InferLiteralType(%q) = %q, want %q", tt.expr, got, tt.want)
			}
		})
	}
}

func TestJoinChainLines(t *testing.T) {
	t.Run("single line no-op", func(t *testing.T) {
		lines := []string{"Category::query()->get()"}
		joined, offset := JoinChainLines(lines, 0, 24)
		if joined != "Category::query()->get()" || offset != 24 {
			t.Errorf("got %q offset %d", joined, offset)
		}
	})

	t.Run("two-line chain", func(t *testing.T) {
		lines := []string{"Category::query()", "    ->get()"}
		joined, _ := JoinChainLines(lines, 1, 6)
		if joined != "Category::query()    ->get()" {
			t.Errorf("got %q", joined)
		}
	})

	t.Run("three-line chain", func(t *testing.T) {
		lines := []string{"Category::query()", "    ->with('rel')", "    ->get()"}
		joined, _ := JoinChainLines(lines, 2, 6)
		if joined != "Category::query()    ->with('rel')    ->get()" {
			t.Errorf("got %q", joined)
		}
	})

	t.Run("non-chain line not joined", func(t *testing.T) {
		lines := []string{"$x = 1;", "$y = 2;"}
		joined, offset := JoinChainLines(lines, 1, 3)
		if joined != "$y = 2;" || offset != 3 {
			t.Errorf("got %q offset %d", joined, offset)
		}
	})

	t.Run("out of bounds", func(t *testing.T) {
		lines := []string{"hello"}
		joined, offset := JoinChainLines(lines, 5, 0)
		if joined != "" || offset != 0 {
			t.Errorf("got %q offset %d", joined, offset)
		}
	})

	t.Run("nullsafe chain", func(t *testing.T) {
		lines := []string{"$user", "    ?->getName()"}
		joined, _ := JoinChainLines(lines, 1, 6)
		if joined != "$user    ?->getName()" {
			t.Errorf("got %q", joined)
		}
	})
}

func TestBuildFQN(t *testing.T) {
	if got := BuildFQN("App", "Service"); got != "App\\Service" {
		t.Errorf("got %q", got)
	}
	if got := BuildFQN("", "Service"); got != "Service" {
		t.Errorf("got %q", got)
	}
}

func TestWordAt(t *testing.T) {
	lines := []string{"$user->getName()"}

	t.Run("variable", func(t *testing.T) {
		w := WordAt(lines, protocol.Position{Line: 0, Character: 0})
		if w != "$user" {
			t.Errorf("got %q", w)
		}
	})

	t.Run("method name", func(t *testing.T) {
		w := WordAt(lines, protocol.Position{Line: 0, Character: 7})
		if w != "getName" {
			t.Errorf("got %q", w)
		}
	})

	t.Run("out of bounds line", func(t *testing.T) {
		if WordAt(lines, protocol.Position{Line: 5, Character: 0}) != "" {
			t.Error("expected empty for OOB line")
		}
	})

	t.Run("out of bounds character", func(t *testing.T) {
		if WordAt(lines, protocol.Position{Line: 0, Character: 100}) != "" {
			t.Error("expected empty for OOB character")
		}
	})
}

func TestIsWordChar(t *testing.T) {
	for _, ch := range "abcABC019_\\" {
		if !IsWordChar(byte(ch)) {
			t.Errorf("expected true for %c", ch)
		}
	}
	for _, ch := range " ->:()'" {
		if IsWordChar(byte(ch)) {
			t.Errorf("expected false for %c", ch)
		}
	}
}

func TestExtractContainerCallArg(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"app('request')", "request"},
		{`app("cache")`, "cache"},
		{"resolve('log')", "log"},
		{"$container->get('mailer')", "mailer"},
		{"$this->app->make('auth')", "auth"},
		{"app('request', 'extra')", "request"},
		{"someFunc('x')", ""},
		{"no parens", ""},
		{"app(Request::class)", "Request::class"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ExtractContainerCallArg(tt.input); got != tt.want {
				t.Errorf("ExtractContainerCallArg(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetLineAt(t *testing.T) {
	source := "line0\nline1\nline2"
	if got := GetLineAt(source, 1); got != "line1" {
		t.Errorf("got %q", got)
	}
	if got := GetLineAt(source, 5); got != "" {
		t.Errorf("expected empty for OOB, got %q", got)
	}
}

func TestGetWordAt(t *testing.T) {
	source := "$user->getName()"
	w := GetWordAt(source, protocol.Position{Line: 0, Character: 0})
	if w != "$user" {
		t.Errorf("got %q", w)
	}
}
