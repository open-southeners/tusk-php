package hover

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
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

func setupProvider(t *testing.T) (*Provider, string) {
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
	provider := NewProvider(idx, ca, "none")

	source := readTestFile(t, "src/Service.php")
	return provider, source
}

// charPosOf returns the line and character position of the last occurrence
// of target on the line containing lineHint. Using LastIndex avoids matching
// substrings inside variable names (e.g. "handle" inside "$handler").
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

func TestHoverInstanceMethodViaProperty(t *testing.T) {
	p, src := setupProvider(t)
	// $this->logger->info('hello') — hover on "info"
	pos := charPosOf(t, src, "info", "$this->logger->info")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function info") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
	if !strings.Contains(val, "Monolog\\Logger") {
		t.Errorf("expected parent class FQN, got:\n%s", val)
	}
}

func TestHoverMethodChainThroughReturnType(t *testing.T) {
	p, src := setupProvider(t)
	// $handler->getLogger()->info('via handler') — hover on second "info"
	pos := charPosOf(t, src, "info", "getLogger()->info")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function info") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
	if !strings.Contains(val, "Monolog\\Logger") {
		t.Errorf("expected parent class FQN, got:\n%s", val)
	}
}

func TestHoverVendorMethod(t *testing.T) {
	p, src := setupProvider(t)
	// $handler->handle(['message' => 'test']) — hover on "handle"
	pos := charPosOf(t, src, "handle", "$handler->handle")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function handle") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
	if !strings.Contains(val, "Monolog\\Handler\\StreamHandler") {
		t.Errorf("expected parent class FQN, got:\n%s", val)
	}
}

func TestHoverStaticMethod(t *testing.T) {
	p, src := setupProvider(t)
	// Logger::create('app') — hover on "create"
	pos := charPosOf(t, src, "create", "Logger::create")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "static") {
		t.Errorf("expected static keyword, got:\n%s", val)
	}
	if !strings.Contains(val, "function create") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
}

func TestHoverSelfMethod(t *testing.T) {
	p, src := setupProvider(t)
	// self::helper() — hover on "helper"
	pos := charPosOf(t, src, "helper", "self::helper")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function helper") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
	if !strings.Contains(val, "App\\Service") {
		t.Errorf("expected parent class FQN, got:\n%s", val)
	}
}

func TestHoverPropertyAccess(t *testing.T) {
	p, src := setupProvider(t)
	// $this->logger in assignment context — hover on "logger"
	pos := charPosOf(t, src, "logger", "$this->logger = $logger")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "$logger") {
		t.Errorf("expected property name, got:\n%s", val)
	}
	if !strings.Contains(val, "Logger") {
		t.Errorf("expected Logger type, got:\n%s", val)
	}
}

func TestHoverVariableWithTypeHint(t *testing.T) {
	p, src := setupProvider(t)
	// public function __construct(Logger $logger) — hover on "$logger"
	pos := charPosOf(t, src, "$logger", "__construct(Logger $logger")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**$logger**") {
		t.Errorf("expected bold variable header, got:\n%s", val)
	}
	if !strings.Contains(val, "Logger $logger") {
		t.Errorf("expected type and variable in code block, got:\n%s", val)
	}
}

func TestHoverClassInUseStatement(t *testing.T) {
	p, src := setupProvider(t)
	// use Monolog\Logger — hover on "Logger" part (which is the whole FQN via getWordAt)
	pos := charPosOf(t, src, "Logger", "use Monolog\\Logger")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**Monolog\\Logger**") {
		t.Errorf("expected class FQN header, got:\n%s", val)
	}
}

func TestHoverClassNameInTypeDecl(t *testing.T) {
	p, src := setupProvider(t)
	// private Logger $logger — hover on "Logger"
	pos := charPosOf(t, src, "Logger", "private Logger $logger")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**Monolog\\Logger**") {
		t.Errorf("expected class FQN header, got:\n%s", val)
	}
}

func TestHoverDocBlockTags(t *testing.T) {
	p, src := setupProvider(t)
	// $handler->handle() has @param and @return in its docblock
	pos := charPosOf(t, src, "handle", "$handler->handle")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**Params**") {
		t.Errorf("expected @param info, got:\n%s", val)
	}
	if !strings.Contains(val, "**Returns**") {
		t.Errorf("expected @return info, got:\n%s", val)
	}
}

func TestHoverNewExpression(t *testing.T) {
	p, src := setupProvider(t)
	// $handler = new StreamHandler() — hover on "StreamHandler"
	pos := charPosOf(t, src, "StreamHandler", "new StreamHandler")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**Monolog\\Handler\\StreamHandler**") {
		t.Errorf("expected class FQN header, got:\n%s", val)
	}
}

func TestHoverNoResult(t *testing.T) {
	p, src := setupProvider(t)
	// Hover on a keyword that isn't a symbol
	pos := charPosOf(t, src, "void", "public function run")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	// void is not indexed, should return nil
	if hover != nil {
		t.Errorf("expected nil hover for keyword, got: %s", hover.Contents.Value)
	}
}

func TestHoverRichCardStructure(t *testing.T) {
	p, src := setupProvider(t)
	pos := charPosOf(t, src, "info", "$this->logger->info")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	// Should have FQN header
	if !strings.Contains(val, "**Monolog\\Logger::info**") {
		t.Errorf("expected FQN header, got:\n%s", val)
	}
	// Should have code block with function signature
	if !strings.Contains(val, "```php") {
		t.Errorf("expected code block, got:\n%s", val)
	}
	// Should have structured params
	if !strings.Contains(val, "**Params**") {
		t.Errorf("expected structured params, got:\n%s", val)
	}
	// Should have structured return
	if !strings.Contains(val, "**Returns** `bool`") {
		t.Errorf("expected structured return, got:\n%s", val)
	}
}

func TestHoverSelfKeyword(t *testing.T) {
	p, _ := setupProvider(t)
	// Create source with self keyword usage
	source := `<?php
namespace App;

use Monolog\Logger;

class Service {
    private Logger $logger;
    public function run(): void {
        self::helper();
    }
    private static function helper(): void {}
}
`
	pos := charPosOf(t, source, "self", "self::helper")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover for self keyword")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**App\\Service**") {
		t.Errorf("expected App\\Service FQN header, got:\n%s", val)
	}
}

func TestHoverVariableRichCard(t *testing.T) {
	p, src := setupProvider(t)
	pos := charPosOf(t, src, "$logger", "__construct(Logger $logger")
	hover := p.GetHover("file:///project/src/Service.php", src, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	// Variable hover should have bold variable name header
	if !strings.Contains(val, "**$logger**") {
		t.Errorf("expected bold variable name header, got:\n%s", val)
	}
}

func TestHoverBuiltinFunctionManualLink(t *testing.T) {
	p, _ := setupProvider(t)
	source := `<?php
strlen("hello");
`
	pos := charPosOf(t, source, "strlen", "strlen")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover for strlen")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "**strlen**") {
		t.Errorf("expected bold FQN header, got:\n%s", val)
	}
	if !strings.Contains(val, "php.net") {
		t.Errorf("expected PHP manual link, got:\n%s", val)
	}
}

func TestHoverDeprecatedTag(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///test.php", `<?php
class Foo {
    /**
     * Old method.
     * @deprecated Use newMethod() instead.
     * @return void
     */
    public function oldMethod(): void {}
}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
use Foo;
$f = new Foo();
$f->oldMethod();
`
	pos := charPosOf(t, source, "oldMethod", "oldMethod")
	hover := p.GetHover("file:///test2.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover for deprecated method")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "Deprecated") {
		t.Errorf("expected deprecated warning, got:\n%s", val)
	}
}

func TestHoverEnumWithBackedType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///test.php", `<?php
namespace App;

enum Status: string {
    case Active = 'active';
    case Inactive = 'inactive';
}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	sym := idx.Lookup("App\\Status")
	if sym == nil {
		t.Fatal("expected enum to be indexed")
	}
	content := p.formatHover(sym)
	if !strings.Contains(content, "**App\\Status**") {
		t.Errorf("expected bold FQN header, got:\n%s", content)
	}
	if !strings.Contains(content, ": string") {
		t.Errorf("expected backed type in declaration, got:\n%s", content)
	}
}

func TestHoverConstantWithValue(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///test.php", `<?php
namespace App;

class Config {
    const VERSION = '1.0.0';
}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	sym := idx.Lookup("App\\Config::VERSION")
	if sym == nil {
		t.Fatal("expected constant to be indexed")
	}
	content := p.formatHover(sym)
	if !strings.Contains(content, "**App\\Config::VERSION**") {
		t.Errorf("expected bold FQN header, got:\n%s", content)
	}
	if !strings.Contains(content, "'1.0.0'") {
		t.Errorf("expected constant value, got:\n%s", content)
	}
}

func TestHoverStandaloneFunctionPreferredOverMethod(t *testing.T) {
	idx := symbols.NewIndex()
	// Index a class with a "request" method
	idx.IndexFile("file:///class.php", `<?php
namespace Illuminate\Http;
class Request {
    public function request(): mixed { return null; }
}
`)
	// Index a global "request" function (like Laravel helpers)
	idx.IndexFile("file:///helpers.php", `<?php
/**
 * Get an instance of the current request.
 *
 * @param string|null $key
 * @return mixed
 */
function request(?string $key = null) { return null; }
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "laravel")

	// Hovering over standalone request() in a controller method body
	source := `<?php
namespace App\Http\Controllers;

class UserController {
    public function index() {
        $name = request('name');
    }
}
`
	pos := charPosOf(t, source, "request", "request('name')")
	hover := p.GetHover("file:///controller.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for request()")
	}
	val := hover.Contents.Value
	// Should show the global function, not the class method
	// The FQN for a global function is just the name, while a method would be Class::method
	if !strings.Contains(val, "**request**") {
		t.Errorf("expected bold function name header, got:\n%s", val)
	}
	if strings.Contains(val, "Illuminate\\Http\\Request::request") {
		t.Errorf("should NOT show method hover for standalone request(), got:\n%s", val)
	}
}

func TestHoverCaseSensitivePreference(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", `<?php
function Request(): object { return new \stdClass(); }
`)
	idx.IndexFile("file:///b.php", `<?php
function request(): mixed { return null; }
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
request();
`
	pos := charPosOf(t, source, "request", "request()")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result")
	}
	val := hover.Contents.Value
	// Should prefer the exact case match "request" over "Request"
	if !strings.Contains(val, "**request**") {
		t.Errorf("expected exact case match 'request' in header, got:\n%s", val)
	}
}

func TestHoverClassModifiers(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///test.php", `<?php
namespace App;

abstract class BaseService {
    abstract public function handle(): void;
}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	sym := idx.Lookup("App\\BaseService")
	if sym == nil {
		t.Fatal("expected class to be indexed")
	}
	content := p.formatHover(sym)
	if !strings.Contains(content, "**App\\BaseService**") {
		t.Errorf("expected bold FQN header, got:\n%s", content)
	}
	if !strings.Contains(content, "abstract class") {
		t.Errorf("expected abstract modifier in code block, got:\n%s", content)
	}
}

func TestHoverVariableWithBuiltinType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
namespace App;

class Foo {
    public function bar(string $name, int $count, ?array $items): void {
        $name;
        $count;
        $items;
    }
}
`
	tests := []struct {
		varName  string
		lineHint string
		wantType string
	}{
		{"$name", "$name;", "string"},
		{"$count", "$count;", "int"},
		{"$items", "$items;", "array"},
	}
	for _, tt := range tests {
		t.Run(tt.varName, func(t *testing.T) {
			pos := charPosOf(t, source, tt.varName, tt.lineHint)
			hover := p.GetHover("file:///test.php", source, pos)
			if hover == nil {
				t.Fatalf("expected hover for %s", tt.varName)
			}
			val := hover.Contents.Value
			if !strings.Contains(val, "**"+tt.varName+"**") {
				t.Errorf("expected bold variable header, got:\n%s", val)
			}
			if !strings.Contains(val, tt.wantType+" "+tt.varName) {
				t.Errorf("expected '%s %s' in code block, got:\n%s", tt.wantType, tt.varName, val)
			}
		})
	}
}

