package checks

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
)

func TestUnusedImportsRule(t *testing.T) {
	rule := &UnusedImportsRule{}

	t.Run("simple unused import detected", func(t *testing.T) {
		source := `<?php
use App\Models\User;
use App\Models\Post;

$user = new User();
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)

		assertFindingCodes(t, findings, []string{"unused-import"})
		if findings[0].Message != "Unused import 'App\\Models\\Post'" {
			t.Errorf("unexpected message: %s", findings[0].Message)
		}
	})

	t.Run("used import in new ClassName not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\User;

$user = new User();
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("used import in type hint not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\User;

function handle(User $user): void {}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("used import in docblock not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\User;

/**
 * @param User $user
 * @return void
 */
function handle($user) {}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("used import in ::class constant not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\User;

$class = User::class;
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("used import in catch not flagged", func(t *testing.T) {
		source := `<?php
use RuntimeException;

try {
    doSomething();
} catch (RuntimeException $e) {
    // handle
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("aliased import checks alias usage", func(t *testing.T) {
		source := `<?php
use App\Models\User as UserModel;

$user = new UserModel();
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("aliased import unused when only original name appears", func(t *testing.T) {
		source := `<?php
use App\Models\User as UserModel;

$user = new User();
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-import"})
	})

	t.Run("use function variant", func(t *testing.T) {
		source := `<?php
use function App\Helpers\formatName;

$name = formatName('John');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("use const variant", func(t *testing.T) {
		source := `<?php
use const App\Config\MAX_RETRIES;

$limit = MAX_RETRIES;
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("import used in PHP 8 attribute not flagged", func(t *testing.T) {
		source := `<?php
use Symfony\Component\Routing\Attribute\Route;

#[Route('/api/users')]
class UserController {}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("multiple unused imports", func(t *testing.T) {
		source := `<?php
use App\Models\User;
use App\Models\Post;
use App\Models\Comment;

echo "nothing";
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-import", "unused-import", "unused-import"})
	})

	t.Run("import used in instanceof not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\User;

if ($obj instanceof User) {
    // handle
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("import used in return type not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\User;

function getUser(): User {
    return new \App\Models\User();
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("severity is hint", func(t *testing.T) {
		source := `<?php
use App\Models\User;

echo "nothing";
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		if findings[0].Severity != SeverityHint {
			t.Errorf("expected SeverityHint, got %d", findings[0].Severity)
		}
	})
}

func assertNoFindings(t *testing.T, findings []Finding) {
	t.Helper()
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d: %v", len(findings), findings)
	}
}

func assertFindingCodes(t *testing.T, findings []Finding, codes []string) {
	t.Helper()
	if len(findings) != len(codes) {
		t.Fatalf("expected %d findings, got %d: %v", len(codes), len(findings), findings)
	}
	for i, code := range codes {
		if findings[i].Code != code {
			t.Errorf("finding[%d].Code = %q, want %q", i, findings[i].Code, code)
		}
	}
}
