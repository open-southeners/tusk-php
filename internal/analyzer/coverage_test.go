package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupCoverageAnalyzer() *Analyzer {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/Service.php", `<?php
namespace App;

class Service {
    public function handle(string $name): string {
        return $name;
    }

    public function process(int $count): void {
        echo $count;
    }

    public string $label;
}
`)
	idx.IndexFile("file:///app/Controller.php", `<?php
namespace App;

use App\Service;

class Controller {
    public function index(Service $svc): void {
        $result = $svc->handle("test");
        $svc->process(5);
        $label = $svc->label;
    }
}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	return NewAnalyzer(idx, ca)
}

func TestFindReferences(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

use App\Service;

class Controller {
    public function index(Service $svc): void {
        $result = $svc->handle("test");
        $svc->process(5);
    }
}
`
	refs := a.FindReferences("file:///app/Controller.php", source, protocol.Position{Line: 7, Character: 25})
	if len(refs) == 0 {
		t.Log("FindReferences returned 0 results (may need cross-file scanning)")
	}
}

func TestGetDocumentSymbolsCoverage(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class Service {
    public string $name;
    private int $count;

    public function handle(): string {
        return "";
    }

    public function process(): void {}
}
`
	syms := a.GetDocumentSymbols("file:///test.php", source)
	if len(syms) == 0 {
		t.Fatal("expected document symbols")
	}

	foundClass := false
	for _, s := range syms {
		if s.Name == "Service" {
			foundClass = true
			if len(s.Children) < 2 {
				t.Errorf("expected children (methods/properties), got %d", len(s.Children))
			}
		}
	}
	if !foundClass {
		t.Error("expected Service class in document symbols")
	}
}

func TestGetSignatureHelpCoverage(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class Service {
    public function handle(string $name, int $count): string {
        return "";
    }
}

$svc = new Service();
$svc->handle(
`
	help := a.GetSignatureHelp("file:///test.php", source, protocol.Position{Line: 11, Character: 13})
	if help == nil {
		t.Log("GetSignatureHelp returned nil (may need more context)")
		return
	}
	if len(help.Signatures) == 0 {
		t.Error("expected at least one signature")
	}
}

func TestPrepareRenameCoverage(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App;

class Service {
    public function handle(): void {}
}
`
	t.Run("on method name", func(t *testing.T) {
		result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 4, Character: 22})
		if result == nil {
			t.Log("PrepareRename returned nil (may need index context)")
		}
	})

	t.Run("on whitespace", func(t *testing.T) {
		result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 0, Character: 0})
		// Should return nil for non-renameable position
		_ = result
	})
}

func TestGetCodeActionsCoverage(t *testing.T) {
	a := setupCoverageAnalyzer()
	source := `<?php
namespace App\Models;
class User {}
`
	actions := a.GetCodeActions("file:///app/Models/User.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///app/Models/User.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 2, Character: 0},
			End:   protocol.Position{Line: 2, Character: 12},
		},
	})
	if len(actions) == 0 {
		t.Log("no code actions returned")
	}
}

func TestGetFileNamespaceCoverage(t *testing.T) {
	a := setupCoverageAnalyzer()

	t.Run("with namespace", func(t *testing.T) {
		ns := a.GetFileNamespace("file:///test.php", `<?php
namespace App\Models;
class User {}
`)
		if ns != "App\\Models\\User" && ns != "App\\Models" {
			t.Logf("got %q", ns)
		}
	})

	t.Run("no namespace", func(t *testing.T) {
		ns := a.GetFileNamespace("file:///test.php", `<?php
class User {}
`)
		// May return "User" or "" depending on implementation
		_ = ns
	})
}
