package symbols

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func testdataPath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "project")
}

func readTestFile(t *testing.T, relPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(testdataPath(), relPath))
	if err != nil {
		t.Fatalf("failed to read %s: %v", relPath, err)
	}
	return string(content)
}

func setupIndex(t *testing.T) *Index {
	t.Helper()
	idx := NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///project/vendor/monolog/monolog/src/Monolog/Logger.php",
		readTestFile(t, "vendor/monolog/monolog/src/Monolog/Logger.php"))
	idx.IndexFile("file:///project/vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php",
		readTestFile(t, "vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php"))
	idx.IndexFile("file:///project/src/Service.php",
		readTestFile(t, "src/Service.php"))

	return idx
}

func TestIndexClassLookup(t *testing.T) {
	idx := setupIndex(t)

	tests := []struct {
		fqn  string
		kind SymbolKind
	}{
		{"Monolog\\Logger", KindClass},
		{"Monolog\\Handler\\StreamHandler", KindClass},
		{"App\\Service", KindClass},
	}

	for _, tt := range tests {
		t.Run(tt.fqn, func(t *testing.T) {
			sym := idx.Lookup(tt.fqn)
			if sym == nil {
				t.Fatalf("expected symbol %q, got nil", tt.fqn)
			}
			if sym.Kind != tt.kind {
				t.Errorf("expected kind %d, got %d", tt.kind, sym.Kind)
			}
		})
	}
}

func TestIndexMethodLookup(t *testing.T) {
	idx := setupIndex(t)

	tests := []struct {
		fqn        string
		returnType string
	}{
		{"Monolog\\Logger::info", "bool"},
		{"Monolog\\Logger::error", "bool"},
		{"Monolog\\Logger::create", "self"},
		{"Monolog\\Handler\\StreamHandler::getLogger", "Monolog\\Logger"},
		{"Monolog\\Handler\\StreamHandler::handle", "bool"},
		{"App\\Service::run", "void"},
		{"App\\Service::helper", "void"},
	}

	for _, tt := range tests {
		t.Run(tt.fqn, func(t *testing.T) {
			sym := idx.Lookup(tt.fqn)
			if sym == nil {
				t.Fatalf("expected symbol %q, got nil", tt.fqn)
			}
			if sym.Kind != KindMethod {
				t.Errorf("expected KindMethod, got %d", sym.Kind)
			}
			if sym.ReturnType != tt.returnType {
				t.Errorf("expected return type %q, got %q", tt.returnType, sym.ReturnType)
			}
		})
	}
}

func TestIndexTypeResolution(t *testing.T) {
	idx := setupIndex(t)

	t.Run("property type resolved to FQN", func(t *testing.T) {
		// App\Service has `private Logger $logger` with `use Monolog\Logger`
		sym := idx.Lookup("App\\Service::$logger")
		if sym == nil {
			t.Fatal("expected property symbol, got nil")
		}
		if sym.Type != "Monolog\\Logger" {
			t.Errorf("expected type %q, got %q", "Monolog\\Logger", sym.Type)
		}
	})

	t.Run("method return type resolved to FQN", func(t *testing.T) {
		// StreamHandler::getLogger returns Logger, resolved via use to Monolog\Logger
		sym := idx.Lookup("Monolog\\Handler\\StreamHandler::getLogger")
		if sym == nil {
			t.Fatal("expected method symbol, got nil")
		}
		if sym.ReturnType != "Monolog\\Logger" {
			t.Errorf("expected return type %q, got %q", "Monolog\\Logger", sym.ReturnType)
		}
	})

	t.Run("param type resolved to FQN", func(t *testing.T) {
		sym := idx.Lookup("App\\Service::__construct")
		if sym == nil {
			t.Fatal("expected method symbol, got nil")
		}
		if len(sym.Params) == 0 {
			t.Fatal("expected params, got none")
		}
		if sym.Params[0].Type != "Monolog\\Logger" {
			t.Errorf("expected param type %q, got %q", "Monolog\\Logger", sym.Params[0].Type)
		}
	})

	t.Run("builtin types not namespace-qualified", func(t *testing.T) {
		sym := idx.Lookup("Monolog\\Logger::info")
		if sym == nil {
			t.Fatal("expected method symbol, got nil")
		}
		if sym.ReturnType != "bool" {
			t.Errorf("expected return type %q, got %q", "bool", sym.ReturnType)
		}
		if len(sym.Params) < 1 {
			t.Fatal("expected params")
		}
		if sym.Params[0].Type != "string" {
			t.Errorf("expected param type %q, got %q", "string", sym.Params[0].Type)
		}
	})
}

func TestIndexLookupByName(t *testing.T) {
	idx := setupIndex(t)

	syms := idx.LookupByName("Logger")
	if len(syms) == 0 {
		t.Fatal("expected to find Logger by name")
	}
	found := false
	for _, sym := range syms {
		if sym.FQN == "Monolog\\Logger" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Monolog\\Logger in LookupByName results")
	}
}

func TestIndexInheritanceChain(t *testing.T) {
	idx := NewIndex()
	// Create a simple inheritance: Child extends Parent
	idx.IndexFile("file:///parent.php", `<?php
namespace App;
class BaseModel {
    public function save(): bool { return true; }
}`)
	idx.IndexFile("file:///child.php", `<?php
namespace App;
class User extends BaseModel {
    public string $name;
}`)

	chain := idx.GetInheritanceChain("App\\User")
	if len(chain) == 0 {
		t.Fatal("expected inheritance chain")
	}
	if chain[0] != "App\\BaseModel" {
		t.Errorf("expected parent %q, got %q", "App\\BaseModel", chain[0])
	}
}

func TestIndexGetClassMembers(t *testing.T) {
	idx := setupIndex(t)

	members := idx.GetClassMembers("Monolog\\Logger")
	if len(members) == 0 {
		t.Fatal("expected class members")
	}

	memberNames := make(map[string]bool)
	for _, m := range members {
		memberNames[m.Name] = true
	}

	for _, expected := range []string{"info", "error", "create"} {
		if !memberNames[expected] {
			t.Errorf("expected member %q in Monolog\\Logger", expected)
		}
	}
}

func TestIndexReindex(t *testing.T) {
	idx := NewIndex()
	uri := "file:///test.php"

	idx.IndexFile(uri, `<?php
class Foo {
    public function bar(): void {}
}`)

	if idx.Lookup("Foo") == nil {
		t.Fatal("Foo should exist")
	}
	if idx.Lookup("Foo::bar") == nil {
		t.Fatal("Foo::bar should exist")
	}

	// Reindex with different content
	idx.IndexFile(uri, `<?php
class Baz {
    public function qux(): void {}
}`)

	if idx.Lookup("Foo") != nil {
		t.Error("Foo should have been removed after reindex")
	}
	if idx.Lookup("Baz") == nil {
		t.Fatal("Baz should exist after reindex")
	}
	if idx.Lookup("Baz::qux") == nil {
		t.Fatal("Baz::qux should exist after reindex")
	}
}

func TestIndexSourcePropagation(t *testing.T) {
	idx := NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFileWithSource("file:///project.php", `<?php
namespace App;
class Service {
    public function run(): void {}
}
function helper(): string { return ""; }
`, SourceProject)

	idx.IndexFileWithSource("file:///vendor.php", `<?php
namespace Vendor;
class Logger {
    public function info(): void {}
}
`, SourceVendor)

	t.Run("project class source", func(t *testing.T) {
		sym := idx.Lookup("App\\Service")
		if sym == nil {
			t.Fatal("expected App\\Service")
		}
		if sym.Source != SourceProject {
			t.Errorf("expected SourceProject, got %d", sym.Source)
		}
	})

	t.Run("project method inherits source", func(t *testing.T) {
		sym := idx.Lookup("App\\Service::run")
		if sym == nil {
			t.Fatal("expected App\\Service::run")
		}
		if sym.Source != SourceProject {
			t.Errorf("expected SourceProject, got %d", sym.Source)
		}
	})

	t.Run("project function source", func(t *testing.T) {
		sym := idx.Lookup("App\\helper")
		if sym == nil {
			t.Fatal("expected App\\helper")
		}
		if sym.Source != SourceProject {
			t.Errorf("expected SourceProject, got %d", sym.Source)
		}
	})

	t.Run("vendor class source", func(t *testing.T) {
		sym := idx.Lookup("Vendor\\Logger")
		if sym == nil {
			t.Fatal("expected Vendor\\Logger")
		}
		if sym.Source != SourceVendor {
			t.Errorf("expected SourceVendor, got %d", sym.Source)
		}
	})

	t.Run("builtin source", func(t *testing.T) {
		sym := idx.Lookup("strlen")
		if sym == nil {
			t.Fatal("expected strlen builtin")
		}
		if sym.Source != SourceBuiltin {
			t.Errorf("expected SourceBuiltin, got %d", sym.Source)
		}
	})

	t.Run("IndexFile defaults to SourceProject", func(t *testing.T) {
		idx.IndexFile("file:///default.php", `<?php
class DefaultClass {}
`)
		sym := idx.Lookup("DefaultClass")
		if sym == nil {
			t.Fatal("expected DefaultClass")
		}
		if sym.Source != SourceProject {
			t.Errorf("expected SourceProject (default), got %d", sym.Source)
		}
	})
}

func TestIndexVirtualMembers(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///model.php", `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Collection;

/**
 * @property string $name
 * @property-read int $id
 * @property-write string $password
 * @method static Collection all()
 * @method bool save(array $options)
 */
class User {
    public string $email;
}
`)

	t.Run("virtual property from @property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$name")
		if sym == nil {
			t.Fatal("expected virtual property '$name'")
		}
		if sym.Kind != KindProperty {
			t.Errorf("expected KindProperty, got %d", sym.Kind)
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual to be true")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
		if sym.Visibility != "public" {
			t.Errorf("expected visibility 'public', got %q", sym.Visibility)
		}
	})

	t.Run("virtual property from @property-read", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$id")
		if sym == nil {
			t.Fatal("expected virtual property '$id'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual to be true")
		}
		if sym.Type != "int" {
			t.Errorf("expected type 'int', got %q", sym.Type)
		}
	})

	t.Run("virtual property from @property-write", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$password")
		if sym == nil {
			t.Fatal("expected virtual property '$password'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual to be true")
		}
	})

	t.Run("virtual method from @method", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::all")
		if sym == nil {
			t.Fatal("expected virtual method 'all'")
		}
		if sym.Kind != KindMethod {
			t.Errorf("expected KindMethod, got %d", sym.Kind)
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual to be true")
		}
		if sym.ReturnType != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected return type 'Illuminate\\Database\\Eloquent\\Collection', got %q", sym.ReturnType)
		}
	})

	t.Run("virtual method with params", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::save")
		if sym == nil {
			t.Fatal("expected virtual method 'save'")
		}
		if sym.ReturnType != "bool" {
			t.Errorf("expected return type 'bool', got %q", sym.ReturnType)
		}
		if len(sym.Params) != 1 {
			t.Fatalf("expected 1 param, got %d", len(sym.Params))
		}
		if sym.Params[0].Type != "array" {
			t.Errorf("expected param type 'array', got %q", sym.Params[0].Type)
		}
		if sym.Params[0].Name != "$options" {
			t.Errorf("expected param name '$options', got %q", sym.Params[0].Name)
		}
	})

	t.Run("real property not overridden", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$email")
		if sym == nil {
			t.Fatal("expected real property 'email'")
		}
		if sym.IsVirtual {
			t.Error("expected IsVirtual to be false for real property")
		}
	})

	t.Run("virtual members in GetClassMembers", func(t *testing.T) {
		members := idx.GetClassMembers("App\\Models\\User")
		names := make(map[string]bool)
		for _, m := range members {
			names[m.Name] = true
		}
		for _, expected := range []string{"$name", "$id", "$password", "all", "save", "$email"} {
			if !names[expected] {
				t.Errorf("expected member %q in GetClassMembers", expected)
			}
		}
	})
}

func TestIndexIDEHelperFileMerge(t *testing.T) {
	idx := NewIndex()

	// First, index the real model file
	idx.IndexFile("file:///app/Models/User.php", `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class User extends Model {
    public string $email;

    public function isAdmin(): bool {
        return false;
    }
}
`)

	// Then, index the IDE helper file (re-declares same class with @property tags)
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", `<?php
namespace App\Models;

/**
 * @property int $id
 * @property string $name
 * @property string $email
 * @property \DateTimeInterface $created_at
 * @method static \Illuminate\Database\Eloquent\Builder query()
 */
class User extends Model {
}
`)

	t.Run("real property preserved", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$email")
		if sym == nil {
			t.Fatal("expected real property '$email'")
		}
		if sym.IsVirtual {
			t.Error("real property should not be marked as virtual")
		}
		if sym.URI != "file:///app/Models/User.php" {
			t.Errorf("expected URI from model file, got %q", sym.URI)
		}
	})

	t.Run("real method preserved", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::isAdmin")
		if sym == nil {
			t.Fatal("expected real method 'isAdmin'")
		}
		if sym.IsVirtual {
			t.Error("real method should not be marked as virtual")
		}
	})

	t.Run("virtual properties from IDE helper merged", func(t *testing.T) {
		for _, name := range []string{"$id", "$name", "$created_at"} {
			sym := idx.Lookup("App\\Models\\User::" + name)
			if sym == nil {
				t.Errorf("expected virtual property %q from IDE helper", name)
				continue
			}
			if !sym.IsVirtual {
				t.Errorf("property %q should be virtual", name)
			}
			if sym.URI != "file:///_ide_helper_models.php" {
				t.Errorf("expected URI from IDE helper, got %q", sym.URI)
			}
		}
	})

	t.Run("virtual method from IDE helper merged", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::query")
		if sym == nil {
			t.Fatal("expected virtual method 'query' from IDE helper")
		}
		if !sym.IsVirtual {
			t.Error("method 'query' should be virtual")
		}
	})

	t.Run("IDE helper does not duplicate existing property", func(t *testing.T) {
		// $email exists as real — IDE helper's @property string $email should be skipped
		members := idx.GetClassMembers("App\\Models\\User")
		emailCount := 0
		for _, m := range members {
			if m.Name == "$email" {
				emailCount++
			}
		}
		if emailCount != 1 {
			t.Errorf("expected exactly 1 '$email' member, got %d", emailCount)
		}
	})

	t.Run("class symbol not overwritten", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User")
		if sym == nil {
			t.Fatal("expected class symbol")
		}
		// Class should still point to the original model file
		if sym.URI != "file:///app/Models/User.php" {
			t.Errorf("expected URI from model file, got %q", sym.URI)
		}
	})
}

func TestIndexIDEHelperFileStandalone(t *testing.T) {
	idx := NewIndex()

	// Index IDE helper file when model hasn't been indexed yet
	idx.IndexIDEHelperFile("file:///_ide_helper_models.php", `<?php
namespace App\Models;

/**
 * @property int $id
 * @property string $name
 */
class Post {
}
`)

	t.Run("class created when not existing", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\Post")
		if sym == nil {
			t.Fatal("expected class symbol created from IDE helper")
		}
	})

	t.Run("virtual properties created", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\Post::$id")
		if sym == nil {
			t.Fatal("expected virtual property '$id'")
		}
		if !sym.IsVirtual {
			t.Error("expected virtual flag")
		}
	})
}
