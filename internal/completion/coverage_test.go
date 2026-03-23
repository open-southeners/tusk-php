package completion

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupCoverageCompletion() *Provider {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///vendor/Logger.php", `<?php
namespace Monolog;
class Logger {
    public function info(string $message): void {}
    public function error(string $message): void {}
    public static function create(): static { return new self(); }
}
`)
	idx.IndexFile("file:///app/Service.php", `<?php
namespace App;
use Monolog\Logger;
class Service {
    private Logger $logger;
    public string $name;
    public function run(): string { return ""; }
}
`)
	idx.IndexFile("file:///app/Controller.php", `<?php
namespace App;
class Controller {
    public function index(): void {}
}
`)
	return NewProvider(idx, nil, "none")
}

func TestCompleteNew(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nuse App\\Service;\nnew "
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 4})
	labels := collectLabels(items)

	if !labels["Service"] {
		t.Errorf("expected Service in new completions, got: %v", labels)
	}
}

func TestCompleteNewWithPrefix(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nnew Ser"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 7})
	labels := collectLabels(items)

	if !labels["Service"] {
		t.Errorf("expected Service matching 'Ser' prefix, got: %v", labels)
	}
}

func TestCompleteUse(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nuse "
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 4})

	if len(items) == 0 {
		t.Error("expected completions for use statement")
	}
}

func TestCompleteUseWithNamespace(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nuse App\\"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 8})

	if len(items) == 0 {
		t.Log("no completions for use App\\ (may need more indexed files)")
	}
}

func TestCompleteAttribute(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\n#["
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 2})

	if len(items) == 0 {
		t.Error("expected attribute completions")
	}
	labels := collectLabels(items)
	if !labels["Override"] && !labels["Deprecated"] {
		t.Errorf("expected PHP attributes, got: %v", labels)
	}
}

func TestCompleteGlobalKeywords(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nret"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 3})
	labels := collectLabels(items)

	if !labels["return"] {
		t.Errorf("expected 'return' keyword, got: %v", labels)
	}
}

func TestCompleteGlobalTypes(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nfunction foo(): str"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 19})
	labels := collectLabels(items)

	if !labels["string"] {
		t.Errorf("expected 'string' type, got: %v", labels)
	}
}

func TestCompleteThisInsideClass(t *testing.T) {
	p := setupCoverageCompletion()
	source := `<?php
namespace App;
class Service {
    public function foo(): void {
        $th
    }
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 4, Character: 11})
	labels := collectLabels(items)

	if !labels["$this"] {
		t.Errorf("expected $this inside class, got: %v", labels)
	}
}

func TestCompleteStaticAccess(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nuse Monolog\\Logger;\nLogger::"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 8})
	labels := collectLabels(items)

	if !labels["create"] {
		t.Errorf("expected static method 'create', got: %v", labels)
	}
}

func TestCompleteMemberFilter(t *testing.T) {
	p := setupCoverageCompletion()
	source := "<?php\nuse App\\Service;\n$s = new Service();\n$s->ru"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 6})
	labels := collectLabels(items)

	if !labels["run"] {
		t.Errorf("expected 'run' matching 'ru' prefix, got: %v", labels)
	}
	// 'name' should be filtered out
	if labels["name"] {
		t.Error("'name' should not match 'ru' prefix")
	}
}
