package analyzer

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupRenameAnalyzer(sources map[string]string) (*Analyzer, func(string) string) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range sources {
		idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	}
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)
	reader := func(uri string) string { return sources[uri] }
	return a, reader
}

func TestPrepareRenameVariable(t *testing.T) {
	source := `<?php
namespace App;
class Foo {
    public function bar() {
        $count = 0;
        $count++;
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	// Cursor on $count
	result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 4, Character: 9})
	if result == nil {
		t.Fatal("expected PrepareRenameResult for $count")
	}
	if result.Placeholder != "$count" {
		t.Errorf("placeholder = %q, want $count", result.Placeholder)
	}
}

func TestPrepareRenameRejectsThis(t *testing.T) {
	source := `<?php
namespace App;
class Foo {
    public function bar() {
        $this->baz();
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 4, Character: 9})
	if result != nil {
		t.Error("should NOT allow renaming $this")
	}
}

func TestPrepareRenameRejectsBuiltin(t *testing.T) {
	source := `<?php
strlen("hello");
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 1, Character: 2})
	if result != nil {
		t.Error("should NOT allow renaming built-in strlen")
	}
}

func TestPrepareRenameClass(t *testing.T) {
	source := `<?php
namespace App;
class UserService {
    public function find(): void {}
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 2, Character: 8})
	if result == nil {
		t.Fatal("expected PrepareRenameResult for UserService")
	}
	if result.Placeholder != "UserService" {
		t.Errorf("placeholder = %q, want UserService", result.Placeholder)
	}
}

func TestRenameVariableLocalScope(t *testing.T) {
	source := `<?php
namespace App;
class Foo {
    public function bar() {
        $count = 0;
        echo $count;
        $count++;
    }
    public function other() {
        $count = 99;
    }
}
`
	a, reader := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	edit := a.Rename("file:///test.php", source, protocol.Position{Line: 4, Character: 9}, "$total", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for variable rename")
	}
	edits := edit.Changes["file:///test.php"]
	// Should rename $count in bar() only (lines 4,5,6), not in other() (line 9)
	for _, e := range edits {
		if e.Range.Start.Line == 9 {
			t.Error("should NOT rename $count in other() method")
		}
		if e.NewText != "$total" {
			t.Errorf("NewText = %q, want $total", e.NewText)
		}
	}
	// Should have at least 3 edits (declaration + 2 usages in bar)
	if len(edits) < 3 {
		t.Errorf("expected at least 3 edits, got %d", len(edits))
	}
}

func TestRenameClassAcrossFiles(t *testing.T) {
	sources := map[string]string{
		"file:///service.php": `<?php
namespace App;
class UserService {
    public function find(): void {}
}
`,
		"file:///controller.php": `<?php
namespace App;
use App\UserService;
class Controller {
    public function index(UserService $svc): void {
        $x = new UserService();
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Rename UserService on its declaration
	edit := a.Rename("file:///service.php", sources["file:///service.php"], protocol.Position{Line: 2, Character: 8}, "AccountService", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for class rename")
	}

	// Should have edits in both files
	if len(edit.Changes) < 2 {
		t.Errorf("expected edits in at least 2 files, got %d", len(edit.Changes))
	}

	// Check controller file has renamed references
	ctrlEdits := edit.Changes["file:///controller.php"]
	found := false
	for _, e := range ctrlEdits {
		if e.NewText == "AccountService" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected AccountService replacement in controller.php")
	}
}

func TestRenameMethodAcrossFiles(t *testing.T) {
	sources := map[string]string{
		"file:///service.php": `<?php
namespace App;
class Service {
    public function process(): void {}
}
`,
		"file:///caller.php": `<?php
namespace App;
use App\Service;
class Caller {
    public function run(Service $s): void {
        $s->process();
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Rename process method from its declaration
	edit := a.Rename("file:///service.php", sources["file:///service.php"], protocol.Position{Line: 3, Character: 23}, "execute", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for method rename")
	}

	// Check caller file
	callerEdits := edit.Changes["file:///caller.php"]
	found := false
	for _, e := range callerEdits {
		if e.NewText == "execute" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'execute' replacement in caller.php")
	}
}

func TestPrepareRenameRejectsVendor(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file:///vendor/logger.php", `<?php
namespace Monolog;
class Logger {
    public function info(string $msg): void {}
}
`, symbols.SourceVendor)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	source := `<?php
use Monolog\Logger;
$l = new Logger();
$l->info("hello");
`
	idx.IndexFileWithSource("file:///test.php", source, symbols.SourceProject)

	// Trying to rename Logger (vendor symbol) should be rejected
	result := a.PrepareRename("file:///test.php", source, protocol.Position{Line: 2, Character: 12})
	if result != nil {
		t.Error("should NOT allow renaming vendor class Logger")
	}
}

func TestRenamePropertyFromDeclaration(t *testing.T) {
	sources := map[string]string{
		"file:///model.php": `<?php
namespace App;
class User {
    public string $name;
    public function getName(): string {
        return $this->name;
    }
}
`,
		"file:///use.php": `<?php
namespace App;
use App\User;
class Test {
    public function run(User $u): void {
        echo $u->name;
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Rename $name property from its declaration (cursor on $name)
	edit := a.Rename("file:///model.php", sources["file:///model.php"],
		protocol.Position{Line: 3, Character: 20}, "fullName", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for property rename from declaration")
	}

	// Check model.php has both $fullName (declaration) and ->fullName (this access)
	modelEdits := edit.Changes["file:///model.php"]
	declFound := false
	accessFound := false
	for _, e := range modelEdits {
		if strings.Contains(e.NewText, "$") {
			declFound = true
		}
		if e.NewText == "fullName" {
			accessFound = true
		}
	}
	if !declFound {
		t.Error("expected $fullName declaration edit in model.php")
	}
	if !accessFound {
		t.Error("expected ->fullName access edit in model.php")
	}

	// Check use.php has ->fullName
	useEdits := edit.Changes["file:///use.php"]
	found := false
	for _, e := range useEdits {
		if e.NewText == "fullName" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ->fullName access edit in use.php")
	}
}

func TestRenamePropertyFromAccess(t *testing.T) {
	sources := map[string]string{
		"file:///model.php": `<?php
namespace App;
class User {
    public string $email;
}
`,
		"file:///use.php": `<?php
namespace App;
use App\User;
class Test {
    public function run(User $u): void {
        echo $u->email;
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Rename from ->email access site
	edit := a.Rename("file:///use.php", sources["file:///use.php"],
		protocol.Position{Line: 5, Character: 18}, "address", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for property rename from access site")
	}

	// Should update declaration in model.php
	modelEdits := edit.Changes["file:///model.php"]
	found := false
	for _, e := range modelEdits {
		if strings.Contains(e.NewText, "address") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected address replacement in model.php declaration")
	}
}

func TestRenameInterfaceAcrossFiles(t *testing.T) {
	sources := map[string]string{
		"file:///iface.php": `<?php
namespace App\Contracts;
interface Cacheable {
    public function getCacheKey(): string;
}
`,
		"file:///impl.php": `<?php
namespace App\Models;
use App\Contracts\Cacheable;
class Product implements Cacheable {
    public function getCacheKey(): string { return 'product'; }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	edit := a.Rename("file:///iface.php", sources["file:///iface.php"],
		protocol.Position{Line: 2, Character: 12}, "Storable", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for interface rename")
	}

	// Check impl.php has the rename in use and implements
	implEdits := edit.Changes["file:///impl.php"]
	count := 0
	for _, e := range implEdits {
		if e.NewText == "Storable" {
			count++
		}
	}
	if count < 2 {
		t.Errorf("expected at least 2 replacements in impl.php (use + implements), got %d", count)
	}
}

func TestRenameVariableWithDollarPrefix(t *testing.T) {
	source := `<?php
function test() {
    $items = [];
    foreach ($items as $item) {
        echo $item;
    }
    return $items;
}
`
	a, reader := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	// Rename $items — user provides "entries" without $
	edit := a.Rename("file:///test.php", source, protocol.Position{Line: 2, Character: 5}, "entries", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for variable rename")
	}

	edits := edit.Changes["file:///test.php"]
	for _, e := range edits {
		if !strings.HasPrefix(e.NewText, "$") {
			t.Errorf("expected $ prefix in variable rename, got %q", e.NewText)
		}
	}
	// Should have 3 occurrences of $items (lines 2, 3, 6)
	if len(edits) < 3 {
		t.Errorf("expected at least 3 edits for $items, got %d", len(edits))
	}
}

func TestRenameEnumAcrossFiles(t *testing.T) {
	sources := map[string]string{
		"file:///status.php": `<?php
namespace App\Enums;
enum Status: string {
    case Active = 'active';
    case Inactive = 'inactive';
}
`,
		"file:///use.php": `<?php
namespace App;
use App\Enums\Status;
class Order {
    public Status $status;
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	edit := a.Rename("file:///status.php", sources["file:///status.php"],
		protocol.Position{Line: 2, Character: 6}, "OrderStatus", reader)
	if edit == nil {
		t.Fatal("expected WorkspaceEdit for enum rename")
	}

	useEdits := edit.Changes["file:///use.php"]
	found := false
	for _, e := range useEdits {
		if e.NewText == "OrderStatus" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected OrderStatus replacement in use.php")
	}
}

func TestGetCodeActionsCopyNamespace(t *testing.T) {
	source := `<?php
namespace App\Models;
class User {}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range:        protocol.Range{},
		Context:      protocol.CodeActionContext{},
	})

	if len(actions) == 0 {
		t.Fatal("expected at least one code action")
	}
	found := false
	for _, action := range actions {
		if strings.Contains(action.Title, "App\\Models\\User") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected code action with FQN App\\Models\\User")
	}
}

func TestGetFileNamespace(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			"class with namespace",
			"<?php\nnamespace App\\Models;\nclass User {}",
			"App\\Models\\User",
		},
		{
			"interface with namespace",
			"<?php\nnamespace App\\Contracts;\ninterface Payable {}",
			"App\\Contracts\\Payable",
		},
		{
			"namespace only",
			"<?php\nnamespace App\\Helpers;\nfunction foo() {}",
			"App\\Helpers",
		},
		{
			"no namespace",
			"<?php\nclass GlobalClass {}",
			"GlobalClass",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": tt.source})
			got := a.GetFileNamespace("file:///test.php", tt.source)
			if got != tt.want {
				t.Errorf("GetFileNamespace() = %q, want %q", got, tt.want)
			}
		})
	}
}
