package analyzer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
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

func setupAnalyzer(t *testing.T) (*Analyzer, string) {
	t.Helper()
	root := testdataPath()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///project/vendor/monolog/monolog/src/Monolog/Logger.php",
		readTestFile(t, "vendor/monolog/monolog/src/Monolog/Logger.php"))
	idx.IndexFile("file:///project/vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php",
		readTestFile(t, "vendor/monolog/monolog/src/Monolog/Handler/StreamHandler.php"))
	idx.IndexFile("file:///project/src/Service.php",
		readTestFile(t, "src/Service.php"))

	ca := container.NewContainerAnalyzer(idx, root, "none")
	a := NewAnalyzer(idx, ca)

	source := readTestFile(t, "src/Service.php")
	return a, source
}

// charPosOf finds the position of target on the line containing lineHint.
func charPosOf(t *testing.T, source, target, lineHint string) protocol.Position {
	t.Helper()
	for i, line := range strings.Split(source, "\n") {
		if lineHint != "" && !strings.Contains(line, lineHint) {
			continue
		}
		col := strings.LastIndex(line, target)
		if col >= 0 {
			return protocol.Position{Line: i, Character: col}
		}
	}
	t.Fatalf("could not find %q (hint: %q) in source", target, lineHint)
	return protocol.Position{}
}

func TestDefinitionMethodViaProperty(t *testing.T) {
	a, src := setupAnalyzer(t)
	// $this->logger->info('hello') — go to definition on "info"
	pos := charPosOf(t, src, "info", "$this->logger->info")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Logger.php") {
		t.Errorf("expected URI to contain Logger.php, got %s", loc.URI)
	}
}

func TestDefinitionStaticMethod(t *testing.T) {
	a, src := setupAnalyzer(t)
	// Logger::create('app') — go to definition on "create"
	pos := charPosOf(t, src, "create", "Logger::create")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Logger.php") {
		t.Errorf("expected URI to contain Logger.php, got %s", loc.URI)
	}
}

func TestDefinitionSelfMethod(t *testing.T) {
	a, src := setupAnalyzer(t)
	// self::helper() — go to definition on "helper"
	pos := charPosOf(t, src, "helper", "self::helper")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Service.php") {
		t.Errorf("expected URI to contain Service.php, got %s", loc.URI)
	}
}

func TestDefinitionPropertyAccess(t *testing.T) {
	a, src := setupAnalyzer(t)
	// $this->logger->info — go to definition on the first "logger" (the property)
	pos := charPosOf(t, src, "logger", "$this->logger->info")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Service.php") {
		t.Errorf("expected URI to contain Service.php, got %s", loc.URI)
	}
}

func TestDefinitionClassInUseStatement(t *testing.T) {
	a, src := setupAnalyzer(t)
	// use Monolog\Logger — go to definition on "Logger"
	pos := charPosOf(t, src, "Logger", "use Monolog\\Logger")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Logger.php") {
		t.Errorf("expected URI to contain Logger.php, got %s", loc.URI)
	}
}

func TestDefinitionClassInTypeHint(t *testing.T) {
	a, src := setupAnalyzer(t)
	// private Logger $logger — go to definition on "Logger"
	pos := charPosOf(t, src, "Logger", "private Logger $logger")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Logger.php") {
		t.Errorf("expected URI to contain Logger.php, got %s", loc.URI)
	}
}

func TestDefinitionNewExpression(t *testing.T) {
	a, src := setupAnalyzer(t)
	// new StreamHandler() — go to definition on "StreamHandler"
	pos := charPosOf(t, src, "StreamHandler", "new StreamHandler")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "StreamHandler.php") {
		t.Errorf("expected URI to contain StreamHandler.php, got %s", loc.URI)
	}
}

func TestDefinitionVariable(t *testing.T) {
	a, src := setupAnalyzer(t)
	// $handler = new StreamHandler() — go to definition on "$handler" in usage
	pos := charPosOf(t, src, "$handler", "$handler->getLogger")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location for variable")
	}
	if !strings.Contains(loc.URI, "StreamHandler.php") {
		t.Errorf("expected URI to contain StreamHandler.php, got %s", loc.URI)
	}
}

func TestDefinitionMethodChain(t *testing.T) {
	a, src := setupAnalyzer(t)
	// $handler->getLogger()->info('via handler') — go to definition on "info"
	pos := charPosOf(t, src, "info", "getLogger()->info")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "Logger.php") {
		t.Errorf("expected URI to contain Logger.php, got %s", loc.URI)
	}
}

func TestDefinitionStaticMethodChain(t *testing.T) {
	idx := symbols.NewIndex()
	// Model with query() that returns Builder (via @return, no type hint — like real Laravel)
	idx.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;

class Model {
    /**
     * @return \Illuminate\Database\Eloquent\Builder
     */
    public static function query() { return new Builder(); }
}
`)
	// Builder with with() method
	idx.IndexFile("file:///vendor/Builder.php", `<?php
namespace Illuminate\Database\Eloquent;

class Builder {
    public function with(string $relation): self { return $this; }
}
`)
	// Another class that also has a with() method (to prevent fallback by name)
	idx.IndexFile("file:///vendor/AQuery.php", `<?php
namespace App\Query;

class AQuery {
    public function with(): void {}
}
`)
	// Add a global function named "with" to be picked as best standalone
	idx.IndexFile("file:///vendor/helpers.php", `<?php
function with(mixed $value): mixed { return $value; }
`)
	// Category extends Model
	idx.IndexFile("file:///app/Category.php", `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class Category extends Model {}
`)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	source := `<?php
namespace App\Http\Controllers;

use App\Models\Category;

class CategoryController {
    public function index() {
        Category::query()->with('form');
    }
}
`
	// Go to definition on "with" — should find Builder::with via chain resolution,
	// NOT via fallback (helpers.php has a global with() function that would win fallback)
	pos := charPosOf(t, source, "with", "query()->with")
	loc := a.FindDefinition("file:///controller.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition location for with() on Builder")
	}
	if !strings.Contains(loc.URI, "Builder.php") {
		t.Errorf("expected URI to contain Builder.php, got %s", loc.URI)
	}
}

func TestDefinitionVendorMethod(t *testing.T) {
	a, src := setupAnalyzer(t)
	// $handler->handle(['message' => 'test']) — go to definition on "handle"
	pos := charPosOf(t, src, "handle", "$handler->handle")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc == nil {
		t.Fatal("expected definition location")
	}
	if !strings.Contains(loc.URI, "StreamHandler.php") {
		t.Errorf("expected URI to contain StreamHandler.php, got %s", loc.URI)
	}
}

func TestDefinitionStandaloneFunctionOverMethod(t *testing.T) {
	idx := symbols.NewIndex()
	// Class with a "request" method
	idx.IndexFile("file:///vendor/Request.php", `<?php
namespace Illuminate\Http;
class Request {
    public function request(): mixed { return null; }
}
`)
	// Global "request" function (Laravel helper)
	idx.IndexFile("file:///vendor/helpers.php", `<?php
function request(?string $key = null) { return null; }
`)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	source := `<?php
namespace App\Http\Controllers;

class UserController {
    public function index() {
        $name = request('name');
    }
}
`
	pos := charPosOf(t, source, "request", "request('name')")
	loc := a.FindDefinition("file:///controller.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition location for request()")
	}
	// Should go to the global function, not the class method
	if !strings.Contains(loc.URI, "helpers.php") {
		t.Errorf("expected URI to contain helpers.php (global function), got %s", loc.URI)
	}
}

func TestDefinitionNoResult(t *testing.T) {
	a, src := setupAnalyzer(t)
	// "void" keyword — should return nil
	pos := charPosOf(t, src, "void", "public function run")
	loc := a.FindDefinition("file:///project/src/Service.php", src, pos)
	if loc != nil {
		t.Errorf("expected nil for keyword, got %s", loc.URI)
	}
}
