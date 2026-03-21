package checks

import (
	"testing"

	"github.com/open-southeners/php-lsp/internal/parser"
)

func TestUnreachableCodeRule(t *testing.T) {
	rule := &UnreachableCodeRule{}

	t.Run("code after return in method body", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): string {
        return "hello";
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
		if findings[0].Severity != SeverityWarning {
			t.Errorf("expected SeverityWarning, got %d", findings[0].Severity)
		}
	})

	t.Run("code after throw", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        throw new \RuntimeException("error");
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("code after exit", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        exit(1);
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("code after die", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): void {
        die("fatal");
        echo "unreachable";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code"})
	})

	t.Run("return inside if block does not flag code after if", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(bool $x): string {
        if ($x) {
            return "yes";
        }
        return "no";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("closing brace after return not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(): string {
        return "hello";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("nested blocks - return in inner block does not affect outer", func(t *testing.T) {
		source := `<?php
class Foo {
    public function bar(bool $x): string {
        if ($x) {
            return "inner";
        }
        $y = "still reachable";
        return $y;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("no method body - abstract method", func(t *testing.T) {
		source := `<?php
abstract class Foo {
    abstract public function bar(): string;
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("multiple unreachable sections", func(t *testing.T) {
		source := `<?php
class Foo {
    public function a(): string {
        return "x";
        echo "dead1";
    }
    public function b(): void {
        throw new \Exception();
        echo "dead2";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unreachable-code", "unreachable-code"})
	})
}
