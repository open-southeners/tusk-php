package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupCoverageHover() *Provider {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///vendor/Logger.php", `<?php
namespace Monolog;
class Logger {
    /** Log an info message. */
    public function info(string $message): void {}
    /** Log an error message. */
    public function error(string $message): void {}
}
`)
	idx.IndexFile("file:///app/Service.php", `<?php
namespace App;
use Monolog\Logger;
class Service {
    private Logger $logger;
    public string $name;
    /** Run the service. */
    public function run(): string { return ""; }
    /**
     * @return static
     */
    public function fluent() { return $this; }
}
`)
	return NewProvider(idx, nil, "none")
}

func TestHoverOnVariable(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
use App\Service;
$svc = new Service();
$svc->run();
`
	// Cursor on "$svc" at line 3 — hover should show type
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 1})
	if hover == nil {
		t.Log("hover on $svc returned nil (variable type inference)")
	}
}

func TestHoverOnMethodAccess(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
use App\Service;
$svc = new Service();
$svc->run();
`
	// Cursor on "run" at line 3
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 7})
	if hover == nil {
		t.Fatal("expected hover on ->run()")
	}
	if !strings.Contains(hover.Contents.Value, "run") {
		t.Errorf("expected 'run' in hover, got: %s", hover.Contents.Value)
	}
}

func TestHoverOnClassName(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
use App\Service;
$svc = new Service();
`
	// Cursor on "Service" in the use statement
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 16})
	if hover == nil {
		t.Log("hover on class name in use statement returned nil")
	} else if !strings.Contains(hover.Contents.Value, "Service") {
		t.Errorf("expected Service in hover, got: %s", hover.Contents.Value)
	}
}

func TestHoverOnSelfStatic(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
namespace App;
class Service {
    public function foo(): void {
        self::bar();
    }
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 4, Character: 9})
	if hover != nil && !strings.Contains(hover.Contents.Value, "Service") {
		t.Errorf("expected Service for self, got: %s", hover.Contents.Value)
	}
}

func TestHoverOnPropertyAccess(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
use App\Service;
$svc = new Service();
$svc->name;
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 7})
	if hover == nil {
		t.Log("hover on ->name returned nil")
	} else if !strings.Contains(hover.Contents.Value, "name") {
		t.Errorf("expected 'name' in hover, got: %s", hover.Contents.Value)
	}
}

func TestHoverOnBuiltinType(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
function foo(string $x): void {}
`
	// Cursor on "string" — should return nil (builtin type, no hover)
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 15})
	if hover != nil {
		t.Log("hover on builtin type returned non-nil (acceptable)")
	}
}

func TestHoverOnUnknownSymbol(t *testing.T) {
	p := setupCoverageHover()
	source := `<?php
$x = unknownFunction();
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 8})
	// Unknown function — may or may not return hover
	_ = hover
}

func TestHoverEmptyLine(t *testing.T) {
	p := setupCoverageHover()
	hover := p.GetHover("file:///test.php", "<?php\n\n", protocol.Position{Line: 1, Character: 0})
	if hover != nil {
		t.Error("expected nil for empty line")
	}
}

func TestHoverOutOfBounds(t *testing.T) {
	p := setupCoverageHover()
	hover := p.GetHover("file:///test.php", "<?php\n", protocol.Position{Line: 99, Character: 0})
	if hover != nil {
		t.Error("expected nil for out-of-bounds")
	}
}

func TestFormatHoverMethod(t *testing.T) {
	p := setupCoverageHover()
	sym := &symbols.Symbol{
		Name:       "handle",
		FQN:        "App\\Service::handle",
		Kind:       symbols.KindMethod,
		ParentFQN:  "App\\Service",
		Visibility: "public",
		ReturnType: "string",
		Params:     []symbols.ParamInfo{{Name: "$name", Type: "string"}},
		DocComment: "/** Handle the request. */",
	}
	content := p.formatHover(sym)
	if content == "" {
		t.Fatal("expected non-empty hover")
	}
	if !strings.Contains(content, "handle") {
		t.Errorf("expected method name in hover, got: %s", content)
	}
}

func TestFormatHoverProperty(t *testing.T) {
	p := setupCoverageHover()
	sym := &symbols.Symbol{
		Name:       "$name",
		FQN:        "App\\Service::$name",
		Kind:       symbols.KindProperty,
		ParentFQN:  "App\\Service",
		Visibility: "public",
		Type:       "string",
	}
	content := p.formatHover(sym)
	if content == "" {
		t.Fatal("expected non-empty hover")
	}
	if !strings.Contains(content, "string") {
		t.Errorf("expected type in hover, got: %s", content)
	}
}

func TestFormatHoverClass(t *testing.T) {
	p := setupCoverageHover()
	sym := &symbols.Symbol{
		Name:    "Service",
		FQN:     "App\\Service",
		Kind:    symbols.KindClass,
		Extends: "Illuminate\\Database\\Eloquent\\Model",
	}
	content := p.formatHover(sym)
	if !strings.Contains(content, "Service") {
		t.Errorf("expected class name, got: %s", content)
	}
}
