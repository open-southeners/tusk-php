package analyzer

import (
	"strings"
	"testing"

	"github.com/open-southeners/php-lsp/internal/composer"
	"github.com/open-southeners/php-lsp/internal/protocol"
)

func TestMoveToNamespaceUpdatesDeclaration(t *testing.T) {
	sources := map[string]string{
		"file:///app/Models/User.php": `<?php

namespace App\Models;

class User {
    public string $name;
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	edit := a.MoveToNamespace(
		"file:///app/Models/User.php",
		sources["file:///app/Models/User.php"],
		"App\\Domain",
		nil,
		reader,
	)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for move namespace")
	}

	// Check namespace declaration was updated
	edits := edit.Changes["file:///app/Models/User.php"]
	found := false
	for _, e := range edits {
		if e.NewText == "App\\Domain" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected namespace declaration updated to App\\Domain")
	}
}

func TestMoveToNamespaceUpdatesUseImports(t *testing.T) {
	sources := map[string]string{
		"file:///app/Models/User.php": `<?php

namespace App\Models;

class User {
    public string $name;
}
`,
		"file:///app/Http/Controllers/UserController.php": `<?php

namespace App\Http\Controllers;

use App\Models\User;

class UserController {
    public function show(User $user): User {
        return $user;
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	edit := a.MoveToNamespace(
		"file:///app/Models/User.php",
		sources["file:///app/Models/User.php"],
		"App\\Domain",
		nil,
		reader,
	)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit")
	}

	// Check use import was updated in controller
	ctrlEdits := edit.Changes["file:///app/Http/Controllers/UserController.php"]
	found := false
	for _, e := range ctrlEdits {
		if strings.Contains(e.NewText, "App\\Domain\\User") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected use import updated to App\\Domain\\User in controller")
		for _, e := range ctrlEdits {
			t.Logf("  edit: %q at %d:%d", e.NewText, e.Range.Start.Line, e.Range.Start.Character)
		}
	}
}

func TestMoveToNamespaceUpdatesFQNReferences(t *testing.T) {
	sources := map[string]string{
		"file:///app/Models/User.php": `<?php

namespace App\Models;

class User {}
`,
		"file:///app/Service.php": `<?php

namespace App;

class Service {
    public function create(): \App\Models\User {
        return new \App\Models\User();
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	edit := a.MoveToNamespace(
		"file:///app/Models/User.php",
		sources["file:///app/Models/User.php"],
		"App\\Domain",
		nil,
		reader,
	)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit")
	}

	svcEdits := edit.Changes["file:///app/Service.php"]
	count := 0
	for _, e := range svcEdits {
		if strings.Contains(e.NewText, "\\App\\Domain\\User") {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 FQN updates in Service.php, got %d", count)
	}
}

func TestMoveToNamespaceUpdatesPartialNamespaceReferences(t *testing.T) {
	sources := map[string]string{
		"file:///app/Http/Controllers/CategoryController.php": `<?php

namespace App\Http\Controllers;

class CategoryController {
    public function index(): void {}
}
`,
		"file:///routes/web.php": `<?php

use App\Http\Controllers;

Route::get('/categories', [Controllers\CategoryController::class, 'index']);
Route::get('/cats', [Controllers\CategoryController::class, 'show']);
`,
		"file:///routes/api.php": `<?php

use App\Http\Controllers\CategoryController;

Route::get('/api/categories', [CategoryController::class, 'index']);
`,
		"file:///app/Service.php": `<?php

namespace App;

use App\Http;

class Service {
    public function run(): void {
        $ctrl = new Http\Controllers\CategoryController();
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	edit := a.MoveToNamespace(
		"file:///app/Http/Controllers/CategoryController.php",
		sources["file:///app/Http/Controllers/CategoryController.php"],
		"App\\Http\\Controllers\\Api",
		nil,
		reader,
	)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit")
	}

	// Check routes/web.php: Controllers\CategoryController → Controllers\Api\CategoryController
	webEdits := edit.Changes["file:///routes/web.php"]
	partialCount := 0
	for _, e := range webEdits {
		if e.NewText == "Controllers\\Api\\CategoryController" {
			partialCount++
		}
	}
	if partialCount < 2 {
		t.Errorf("expected at least 2 partial namespace updates in web.php, got %d", partialCount)
		for _, e := range webEdits {
			t.Logf("  edit: %q at line %d col %d-%d", e.NewText, e.Range.Start.Line, e.Range.Start.Character, e.Range.End.Character)
		}
	}

	// Check routes/api.php: direct import should be updated
	apiEdits := edit.Changes["file:///routes/api.php"]
	directFound := false
	for _, e := range apiEdits {
		if e.NewText == "App\\Http\\Controllers\\Api\\CategoryController" {
			directFound = true
			break
		}
	}
	if !directFound {
		t.Error("expected direct use import updated in api.php")
	}

	// Check app/Service.php: Http\Controllers\CategoryController → Http\Controllers\Api\CategoryController
	svcEdits := edit.Changes["file:///app/Service.php"]
	deepPartialFound := false
	for _, e := range svcEdits {
		if e.NewText == "Http\\Controllers\\Api\\CategoryController" {
			deepPartialFound = true
			break
		}
	}
	if !deepPartialFound {
		t.Error("expected deep partial namespace update in Service.php")
		for _, e := range svcEdits {
			t.Logf("  edit: %q at line %d col %d-%d", e.NewText, e.Range.Start.Line, e.Range.Start.Character, e.Range.End.Character)
		}
	}
}

func TestMoveToNamespaceIncludesFileRename(t *testing.T) {
	sources := map[string]string{
		"file:///project/app/Models/User.php": `<?php

namespace App\Models;

class User {}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	autoload := []composer.AutoloadEntry{
		{Namespace: "App", Path: "/project/app", IsVendor: false},
	}
	edit := a.MoveToNamespace(
		"file:///project/app/Models/User.php",
		sources["file:///project/app/Models/User.php"],
		"App\\Domain",
		autoload,
		reader,
	)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit")
	}

	// Check DocumentChanges has a RenameFile operation
	foundRename := false
	for _, dc := range edit.DocumentChanges {
		if dc.RenameFile != nil {
			foundRename = true
			if !strings.Contains(dc.RenameFile.NewURI, "Domain") {
				t.Errorf("expected new URI to contain 'Domain', got %s", dc.RenameFile.NewURI)
			}
			if dc.RenameFile.Kind != "rename" {
				t.Errorf("expected kind 'rename', got %q", dc.RenameFile.Kind)
			}
			break
		}
	}
	if !foundRename {
		t.Error("expected RenameFile document change for PSR-4 file move")
	}
}

func TestMoveToNamespaceCodeAction(t *testing.T) {
	source := `<?php

namespace App\Models;

class User {}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	// Cursor on class declaration line (line 4)
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 4, Character: 0},
			End:   protocol.Position{Line: 4, Character: 0},
		},
		Context: protocol.CodeActionContext{},
	})

	foundMove := false
	for _, action := range actions {
		if action.Kind == "refactor.move" {
			foundMove = true
			break
		}
	}
	if !foundMove {
		t.Error("expected 'refactor.move' code action on class declaration line")
	}
}

func TestMoveToNamespaceCodeActionNotOnOtherLines(t *testing.T) {
	source := `<?php

namespace App\Models;

class User {
    public string $name;
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	// Cursor on property line (line 5) — should NOT offer move
	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 5, Character: 0},
			End:   protocol.Position{Line: 5, Character: 0},
		},
		Context: protocol.CodeActionContext{},
	})

	for _, action := range actions {
		if action.Kind == "refactor.move" {
			t.Error("should NOT offer 'refactor.move' on property line")
		}
	}
}
