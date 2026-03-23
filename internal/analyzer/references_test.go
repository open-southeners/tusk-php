package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestFindAllReferencesClass(t *testing.T) {
	sources := map[string]string{
		"file:///user.php": `<?php
namespace App\Models;
class User {
    public string $name;
}
`,
		"file:///controller.php": `<?php
namespace App\Http;
use App\Models\User;
class UserController {
    public function show(User $user): User {
        $x = new User();
        return $user;
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Find references to User from its declaration
	locs := a.FindAllReferences("file:///user.php", sources["file:///user.php"],
		protocol.Position{Line: 2, Character: 8}, reader)

	if len(locs) < 4 {
		t.Errorf("expected at least 4 references to User (decl + use + 2 type hints + new), got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		}
	}

	// Verify references span both files
	fileCount := make(map[string]int)
	for _, loc := range locs {
		fileCount[loc.URI]++
	}
	if fileCount["file:///controller.php"] == 0 {
		t.Error("expected references in controller.php")
	}
	if fileCount["file:///user.php"] == 0 {
		t.Error("expected references in user.php (definition)")
	}
}

func TestFindAllReferencesMethod(t *testing.T) {
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
        $s->process();
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Find references to process from its declaration
	locs := a.FindAllReferences("file:///service.php", sources["file:///service.php"],
		protocol.Position{Line: 3, Character: 22}, reader)

	// Definition + 2 call sites
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to process(), got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		}
	}
}

func TestFindAllReferencesVariable(t *testing.T) {
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
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	locs := a.FindAllReferences("file:///test.php", source,
		protocol.Position{Line: 4, Character: 9}, nil)

	// Should find $count only in bar(), not in other()
	for _, loc := range locs {
		if loc.Range.Start.Line == 9 {
			t.Error("should NOT include $count from other() method")
		}
	}
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to $count in bar(), got %d", len(locs))
	}
}

func TestFindAllReferencesProperty(t *testing.T) {
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

	// Find references to $name property
	locs := a.FindAllReferences("file:///model.php", sources["file:///model.php"],
		protocol.Position{Line: 3, Character: 20}, reader)

	// Declaration ($name) + $this->name + $u->name
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to name property, got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d-%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
		}
	}

	// Verify cross-file
	fileCount := make(map[string]int)
	for _, loc := range locs {
		fileCount[loc.URI]++
	}
	if fileCount["file:///use.php"] == 0 {
		t.Error("expected property reference in use.php")
	}
}

func TestFindAllReferencesFunction(t *testing.T) {
	sources := map[string]string{
		"file:///helpers.php": `<?php
function formatName(string $name): string {
    return ucfirst($name);
}
`,
		"file:///caller.php": `<?php
$result = formatName("john");
echo formatName("jane");
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	locs := a.FindAllReferences("file:///helpers.php", sources["file:///helpers.php"],
		protocol.Position{Line: 1, Character: 10}, reader)

	// Definition + 2 call sites
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to formatName, got %d", len(locs))
	}
}

func TestFindAllReferencesDeduplicates(t *testing.T) {
	source := `<?php
namespace App;
class Foo {}
`
	a, reader := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	locs := a.FindAllReferences("file:///test.php", source,
		protocol.Position{Line: 2, Character: 8}, reader)

	// Definition should appear only once
	count := 0
	for _, loc := range locs {
		if loc.URI == "file:///test.php" && loc.Range.Start.Line == 2 {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected definition to appear once, got %d times", count)
	}
}
