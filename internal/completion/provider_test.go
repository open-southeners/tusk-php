package completion

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestSortPriority(t *testing.T) {
	tests := []struct {
		name    string
		sym     *symbols.Symbol
		ns      string
		wantPfx string
	}{
		{"same namespace project", &symbols.Symbol{Name: "Foo", FQN: "App\\Models\\Foo", Source: symbols.SourceProject}, "App\\Models", "1"},
		{"different namespace project", &symbols.Symbol{Name: "Bar", FQN: "App\\Services\\Bar", Source: symbols.SourceProject}, "App\\Models", "2"},
		{"builtin", &symbols.Symbol{Name: "strlen", FQN: "strlen", Source: symbols.SourceBuiltin}, "App\\Models", "3"},
		{"vendor", &symbols.Symbol{Name: "collect", FQN: "collect", Source: symbols.SourceVendor}, "App\\Models", "4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sortPriority(tt.sym, tt.ns)
			if got[0:1] != tt.wantPfx {
				t.Errorf("sortPriority() prefix = %q, want %q", got[0:1], tt.wantPfx)
			}
		})
	}
}

func TestSortOrdering(t *testing.T) {
	projectSame := sortPriority(&symbols.Symbol{Name: "Foo", FQN: "App\\Models\\Foo", Source: symbols.SourceProject}, "App\\Models")
	projectOther := sortPriority(&symbols.Symbol{Name: "Bar", FQN: "App\\Services\\Bar", Source: symbols.SourceProject}, "App\\Models")
	builtin := sortPriority(&symbols.Symbol{Name: "strlen", FQN: "strlen", Source: symbols.SourceBuiltin}, "App\\Models")
	vendor := sortPriority(&symbols.Symbol{Name: "collect", FQN: "collect", Source: symbols.SourceVendor}, "App\\Models")

	if projectSame >= projectOther {
		t.Error("same-namespace project should sort before other-namespace project")
	}
	if projectOther >= builtin {
		t.Error("project should sort before builtins")
	}
	if builtin >= vendor {
		t.Error("builtins should sort before vendor")
	}
}

func TestKeywordSortLast(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := "<?php\nnamespace App;\n"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 0})

	for _, item := range items {
		if item.Kind == protocol.CompletionItemKindKeyword {
			if item.SortText[0:1] != "5" {
				t.Errorf("keyword %q has SortText %q, expected prefix '5'", item.Label, item.SortText)
			}
		}
	}
}

func TestExtractNamespace(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"<?php\nnamespace App\\Models;\n", "App\\Models"},
		{"<?php\nnamespace App\\Services;\nclass Foo {}", "App\\Services"},
		{"<?php\n// no namespace\n", ""},
	}
	for _, tt := range tests {
		got := extractNamespace(tt.source)
		if got != tt.want {
			t.Errorf("extractNamespace() = %q, want %q", got, tt.want)
		}
	}
}

func TestCompletionSourceSorting(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	// Index a project function
	idx.IndexFileWithSource("file:///project.php", `<?php
namespace App;
function str_project(): string { return ""; }
`, symbols.SourceProject)

	// Index a vendor function
	idx.IndexFileWithSource("file:///vendor.php", `<?php
function str_vendor(): string { return ""; }
`, symbols.SourceVendor)

	p := NewProvider(idx, nil, "")
	source := "<?php\nnamespace App;\nstr"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 3})

	sortTexts := make(map[string]string)
	for _, item := range items {
		sortTexts[item.Label] = item.SortText
	}

	// Project symbol in same namespace should sort first
	if st, ok := sortTexts["str_project"]; ok {
		if st[0:1] != "1" {
			t.Errorf("str_project SortText = %q, expected prefix '1' (same namespace)", st)
		}
	}

	// Builtin str_contains should sort as builtin
	if st, ok := sortTexts["str_contains"]; ok {
		if st[0:1] != "3" {
			t.Errorf("str_contains SortText = %q, expected prefix '3' (builtin)", st)
		}
	}

	// Vendor function should sort last among symbols
	if st, ok := sortTexts["str_vendor"]; ok {
		if st[0:1] != "4" {
			t.Errorf("str_vendor SortText = %q, expected prefix '4' (vendor)", st)
		}
	}
}

func TestCompleteBackslashShowsRootNamespaces(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file:///a.php", `<?php
namespace App\Models;
class User {}
`, symbols.SourceProject)
	idx.IndexFileWithSource("file:///b.php", `<?php
namespace Illuminate\Support;
class Collection {}
`, symbols.SourceVendor)

	p := NewProvider(idx, nil, "")
	source := "<?php\nnamespace App;\n\\"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 1})

	labels := make(map[string]protocol.CompletionItemKind)
	for _, item := range items {
		labels[item.Label] = item.Kind
	}

	// Should show top-level namespace segments
	if _, ok := labels["App"]; !ok {
		t.Error("expected 'App' namespace segment in completions")
	}
	if _, ok := labels["Illuminate"]; !ok {
		t.Error("expected 'Illuminate' namespace segment in completions")
	}
	// Namespace segments should be Module kind
	if labels["App"] != protocol.CompletionItemKindModule {
		t.Errorf("expected Module kind for namespace segment, got %d", labels["App"])
	}
}

func TestCompleteNamespaceSegments(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///a.php", `<?php
namespace App\Models;
class User {}
class Post {}
`, symbols.SourceProject)
	idx.IndexFileWithSource("file:///b.php", `<?php
namespace App\Services;
class AuthService {}
`, symbols.SourceProject)

	p := NewProvider(idx, nil, "")
	source := "<?php\nuse App\\"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 8})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	// Should show sub-namespace segments
	if !labels["Models"] {
		t.Error("expected 'Models' namespace segment")
	}
	if !labels["Services"] {
		t.Error("expected 'Services' namespace segment")
	}
}

func TestCompleteNamespaceDirectSymbols(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///a.php", `<?php
namespace App\Models;
class User {}
class Post {}
`, symbols.SourceProject)

	p := NewProvider(idx, nil, "")
	source := "<?php\nuse App\\Models\\"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 15})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["User"] {
		t.Error("expected 'User' class in namespace completions")
	}
	if !labels["Post"] {
		t.Error("expected 'Post' class in namespace completions")
	}
}

func TestCompleteMemberAccess(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///cls.php", `<?php
namespace App;
class Service {
    public string $name;
    public static string $instance;
    public function run(): void {}
    public static function create(): self {}
    private function secret(): void {}
    const VERSION = '1.0';
}
`)
	p := NewProvider(idx, nil, "")

	t.Run("variable instance access", func(t *testing.T) {
		source := "<?php\nnamespace App;\nuse App\\Service;\n$svc = new Service();\n$svc->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 4, Character: 6})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["run"] {
			t.Error("expected 'run' instance method")
		}
		if !labels["name"] {
			t.Error("expected 'name' instance property")
		}
		if labels["create"] {
			t.Error("should NOT show static method 'create' via ->")
		}
		if labels["$instance"] {
			t.Error("should NOT show static property '$instance' via ->")
		}
		if labels["secret"] {
			t.Error("should NOT show private method 'secret'")
		}
	})

	t.Run("static access on class name", func(t *testing.T) {
		source := "<?php\nnamespace App;\nService::"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 9})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["create"] {
			t.Error("expected static method 'create' via ::")
		}
		if !labels["VERSION"] {
			t.Error("expected constant 'VERSION' via ::")
		}
		if !labels["$instance"] {
			t.Error("expected static property '$instance' via ::")
		}
		if labels["run"] {
			t.Error("should NOT show instance method 'run' via ::")
		}
		if labels["name"] {
			t.Error("should NOT show instance property '$name' via ::")
		}
	})

	t.Run("static access on variable type", func(t *testing.T) {
		source := "<?php\nnamespace App;\n$svc = new Service();\n$svc::"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 6})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["create"] {
			t.Error("expected static method 'create' via $var::")
		}
		if !labels["VERSION"] {
			t.Error("expected constant 'VERSION' via $var::")
		}
	})

	t.Run("new expression chained access", func(t *testing.T) {
		source := "<?php\nnamespace App;\n(new Service())->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 17})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["run"] {
			t.Error("expected 'run' from (new Service())->")
		}
	})

	t.Run("static method has snippet", func(t *testing.T) {
		source := "<?php\nnamespace App;\nService::"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 9})
		for _, item := range items {
			if item.Label == "create" {
				if item.InsertText != "create($0)" {
					t.Errorf("expected snippet 'create($0)', got %q", item.InsertText)
				}
				if item.InsertTextFormat != 2 {
					t.Error("expected InsertTextFormat=2 (snippet) for static method")
				}
				return
			}
		}
		t.Error("create method not found")
	})

	t.Run("self and this access", func(t *testing.T) {
		source := "<?php\nnamespace App;\nclass Service {\n    public function run(): void {}\n    public static function create(): self {}\n    const VERSION = '1.0';\n    public function test() {\n        self::\n    }\n}\n"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 14})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["create"] {
			t.Error("expected 'create' via self::")
		}
		if !labels["VERSION"] {
			t.Error("expected 'VERSION' via self::")
		}
	})

	t.Run("@var annotation resolves member access", func(t *testing.T) {
		source := "<?php\nnamespace App;\nuse App\\Service;\nfunction test(): void {\n    /** @var Service $svc */\n    $svc = app(Service::class);\n    $svc->\n}\n"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 6, Character: 10})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["run"] {
			t.Error("expected 'run' instance method via @var-resolved $svc->")
		}
		if !labels["name"] {
			t.Error("expected 'name' property via @var-resolved $svc->")
		}
	})
}

func TestCompleteFQNStaticAccess(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///cls.php", `<?php
namespace App\Models;
class User {
    public static function find(int $id): ?self {}
    const TABLE = 'users';
}
`)
	p := NewProvider(idx, nil, "")

	t.Run("FQN with leading backslash", func(t *testing.T) {
		source := `<?php
\App\Models\User::`
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 18})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["find"] {
			t.Error("expected 'find' static method via \\App\\Models\\User::")
		}
		if !labels["TABLE"] {
			t.Error("expected 'TABLE' constant via \\App\\Models\\User::")
		}
	})

	t.Run("FQN without leading backslash", func(t *testing.T) {
		source := `<?php
App\Models\User::`
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["find"] {
			t.Error("expected 'find' static method via App\\Models\\User::")
		}
	})

	t.Run("new FQN chained access", func(t *testing.T) {
		source := `<?php
(new \App\Models\User())->`
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 26})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["find"] {
			// find is static, shouldn't appear via ->
		}
		// At minimum the completion should not crash and should resolve the type
		if len(items) == 0 {
			t.Error("expected some completions from (new \\App\\Models\\User())->")
		}
	})
}

func TestCompleteFQNNewAssignmentMemberAccess(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///action.php", `<?php
namespace App\Actions\Category;
class GetDocumentPayloadFromCategory {
    public function handle(): array { return []; }
    public string $name;
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php
namespace App\Http\Controllers;

class CategoryController {
    public function index(): mixed {
        $a = new \App\Actions\Category\GetDocumentPayloadFromCategory();
        $a->
    }
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 6, Character: 12})
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}
	if !labels["handle"] {
		t.Errorf("expected 'handle' method via $a->, got labels: %v", labels)
	}
	if !labels["name"] {
		t.Errorf("expected 'name' property via $a->, got labels: %v", labels)
	}
}

func TestCompleteFQNNewAssignmentParamTypeHint(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///response.php", `<?php
namespace OpenSoutheners\LaravelApiable\Http;
class JsonApiResponse {
    public function using($query): self { return $this; }
    public function list(): array { return []; }
}
`)
	p := NewProvider(idx, nil, "")

	// Simulates the real CategoryController pattern: type-hinted param
	source := `<?php
namespace App\Http\Controllers;

use OpenSoutheners\LaravelApiable\Http\JsonApiResponse;

class CategoryController {
    public function index(JsonApiResponse $response): mixed {
        $response->
    }
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 19})
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}
	if !labels["using"] {
		t.Errorf("expected 'using' method via type-hinted $response->, got labels: %v", labels)
	}
	if !labels["list"] {
		t.Errorf("expected 'list' method via type-hinted $response->, got labels: %v", labels)
	}
}

func TestCompleteNewShortNameWithUseImport(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///category.php", `<?php
namespace App\Models;
class Category {
    public function devices(): array { return []; }
    public function form(): array { return []; }
    public string $name;
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php

namespace App\Http\Controllers;

use App\Models\Category;

class CategoryController
{
    public function index(): mixed
    {
        $newCategory = new Category();

        $newCategory->
    }
}
`
	// $newCategory-> is at line 12 (0-based), character 22
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 12, Character: 22})
	labels := map[string]bool{}
	for _, item := range items {
		labels[item.Label] = true
	}
	if !labels["devices"] {
		t.Errorf("expected 'devices' method via $newCategory->, got labels: %v", labels)
	}
	if !labels["form"] {
		t.Errorf("expected 'form' method via $newCategory->")
	}
	if !labels["name"] {
		t.Errorf("expected 'name' property via $newCategory->")
	}
}

func TestCompleteMemberAccessFiltering(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///cls.php", `<?php
namespace App;
class Service {
    public function doSomething(): void {}
    public function doOther(): void {}
    public function run(): void {}
    public string $description;
}
`)
	p := NewProvider(idx, nil, "")

	t.Run("typing after arrow filters to class members only", func(t *testing.T) {
		source := "<?php\nnamespace App;\n$svc = new Service();\n$svc->do"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 8})
		for _, item := range items {
			// Should only have class members matching "do", no globals
			if item.Kind == protocol.CompletionItemKindKeyword {
				t.Errorf("should NOT show keywords in member context, got %q", item.Label)
			}
		}
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["doSomething"] {
			t.Error("expected 'doSomething' matching 'do' prefix")
		}
		if !labels["doOther"] {
			t.Error("expected 'doOther' matching 'do' prefix")
		}
		if labels["run"] {
			t.Error("should NOT show 'run' (doesn't match 'do' prefix)")
		}
		if labels["description"] {
			t.Error("should NOT show 'description' (doesn't match 'do' prefix)")
		}
	})

	t.Run("typing after double colon filters to static members only", func(t *testing.T) {
		idx2 := symbols.NewIndex()
		idx2.RegisterBuiltins()
		idx2.IndexFile("file:///cls2.php", `<?php
namespace App;
class Foo {
    public static function create(): self {}
    public static function configure(): void {}
    public function callMe(): void {}
    const CONFIG = 'x';
}
`)
		p2 := NewProvider(idx2, nil, "")
		source := "<?php\nnamespace App;\nFoo::c"
		items := p2.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 6})
		labels := map[string]bool{}
		for _, item := range items {
			labels[item.Label] = true
		}
		if !labels["create"] {
			t.Error("expected 'create' matching 'c' prefix via ::")
		}
		if !labels["configure"] {
			t.Error("expected 'configure' matching 'c' prefix via ::")
		}
		if !labels["CONFIG"] {
			t.Error("expected 'CONFIG' matching 'c' prefix via ::")
		}
		if labels["callMe"] {
			t.Error("should NOT show non-static 'callMe' via ::")
		}
		// No globals/keywords
		for _, item := range items {
			if item.Kind == protocol.CompletionItemKindKeyword {
				t.Errorf("should NOT show keywords in static context, got %q", item.Label)
			}
		}
	})
}

func TestCompleteNamespaceFilterMidSegment(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///a.php", `<?php
namespace App\Models;
class User {}
class Post {}
`, symbols.SourceProject)
	idx.IndexFileWithSource("file:///b.php", `<?php
namespace App\Mail;
class WelcomeMail {}
`, symbols.SourceProject)

	p := NewProvider(idx, nil, "")
	source := "<?php\nuse App\\M"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 9})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	// Should match namespace segments starting with M
	if !labels["Models"] {
		t.Error("expected 'Models' namespace segment matching 'M'")
	}
	if !labels["Mail"] {
		t.Error("expected 'Mail' namespace segment matching 'M'")
	}
}
