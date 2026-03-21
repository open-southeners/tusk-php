package checks

import (
	"testing"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

func TestRedundantNullsafeRule(t *testing.T) {
	t.Run("nullsafe on non-nullable typed parameter flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(User $user): void {
        $user?->name;
    }
}
`
		file := parser.ParseFile(source)
		rule := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line int, file *parser.FileNode) string {
				if expr == "$user" {
					return "User"
				}
				return ""
			},
		}
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"redundant-nullsafe"})
	})

	t.Run("nullsafe on nullable parameter not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(?User $user): void {
        $user?->name;
    }
}
`
		file := parser.ParseFile(source)
		rule := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line int, file *parser.FileNode) string {
				if expr == "$user" {
					return "?User"
				}
				return ""
			},
		}
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nullsafe on mixed type not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(mixed $x): void {
        $x?->name;
    }
}
`
		file := parser.ParseFile(source)
		rule := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line int, file *parser.FileNode) string {
				return "mixed"
			},
		}
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nullsafe on union with null not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(User|null $user): void {
        $user?->name;
    }
}
`
		file := parser.ParseFile(source)
		rule := &RedundantNullsafeRule{
			TypeResolver: func(expr, source string, line int, file *parser.FileNode) string {
				return "User|null"
			},
		}
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("no type resolver is no-op", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        $x?->name;
    }
}
`
		file := parser.ParseFile(source)
		rule := &RedundantNullsafeRule{}
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})
}

func TestRedundantUnionRule(t *testing.T) {
	rule := &RedundantUnionRule{}

	t.Run("duplicate union member flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(string|string $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "Duplicate type")
	})

	t.Run("nullable shorthand with null flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(?string|null $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "already makes")
	})

	t.Run("object with class name flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(object|DateTime $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "covered by 'object'")
	})

	t.Run("mixed with string flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(mixed|string $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "covered by 'mixed'")
	})

	t.Run("parent child in union flagged with index", func(t *testing.T) {
		idx := symbols.NewIndex()
		idx.IndexFile("file:///parent.php", `<?php
class Animal {}
`)
		idx.IndexFile("file:///child.php", `<?php
class Dog extends Animal {}
`)
		source := `<?php
class Foo {
    public function bar(Animal|Dog $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, idx)
		hasFinding(t, findings, "redundant-union-member", "covered by parent 'Animal'")
	})

	t.Run("legitimate union not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(string|int $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("iterable and array flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(iterable|array $x): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		hasFinding(t, findings, "redundant-union-member", "covered by 'iterable'")
	})
}

func hasFinding(t *testing.T, findings []Finding, code, msgSubstr string) {
	t.Helper()
	for _, f := range findings {
		if f.Code == code && (msgSubstr == "" || contains(f.Message, msgSubstr)) {
			return
		}
	}
	t.Errorf("expected finding with code=%q containing %q, got: %v", code, msgSubstr, findings)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
