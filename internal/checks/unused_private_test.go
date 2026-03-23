package checks

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
)

func TestUnusedPrivateRule(t *testing.T) {
	rule := &UnusedPrivateRule{}

	t.Run("unused private method detected", func(t *testing.T) {
		source := `<?php
class Foo {
    private function unusedMethod(): void {}
    public function bar(): void {
        echo "hello";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-private-method"})
		if findings[0].Message != "Private method 'Foo::unusedMethod' is never used" {
			t.Errorf("unexpected message: %s", findings[0].Message)
		}
	})

	t.Run("used private method via this not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private function helper(): string { return "ok"; }
    public function bar(): void {
        echo $this->helper();
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("used private method via self not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private static function helper(): string { return "ok"; }
    public function bar(): void {
        echo self::helper();
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unused private property detected", func(t *testing.T) {
		source := `<?php
class Foo {
    private string $unused = "hello";
    public function bar(): void {
        echo "world";
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unused-private-property"})
	})

	t.Run("used private property via this not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private string $name = "hello";
    public function bar(): string {
        return $this->name;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("magic methods never flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private function __construct() {}
    private function __toString(): string { return ""; }
    private function __clone(): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("private method used as callable string not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private function handler(): void {}
    public function register(): void {
        $cb = [$this, 'handler'];
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("protected and public methods never flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    protected function protectedUnused(): void {}
    public function publicUnused(): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("severity is info for methods hint for properties", func(t *testing.T) {
		source := `<?php
class Foo {
    private string $unused = "";
    private function unusedMethod(): void {}
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		if len(findings) != 2 {
			t.Fatalf("expected 2 findings, got %d", len(findings))
		}
		// Methods checked first, then properties
		methF := findings[0]
		propF := findings[1]
		if propF.Code != "unused-private-property" || propF.Severity != SeverityHint {
			t.Errorf("property finding: code=%s sev=%d", propF.Code, propF.Severity)
		}
		if methF.Code != "unused-private-method" || methF.Severity != SeverityInfo {
			t.Errorf("method finding: code=%s sev=%d", methF.Code, methF.Severity)
		}
	})

	t.Run("used private static property not flagged", func(t *testing.T) {
		source := `<?php
class Foo {
    private static int $count = 0;
    public function increment(): void {
        self::$count++;
    }
}
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})
}
