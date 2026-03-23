package diagnostics

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/checks"
	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestAnalyzeOnSave(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	cfg := config.DefaultConfig()
	p := NewProvider(idx, "none", "/tmp", nil, cfg)

	// With no resolvers set, should still not panic
	p.AnalyzeOnSave("file:///test.php", `<?php class Foo {}`)

	// Results should be cached
	diags := p.Analyze("file:///test.php", `<?php class Foo {}`)
	_ = diags // may be empty, just verifying no panic
}

func TestAnalyzeOnSaveWithTypeResolver(t *testing.T) {
	idx := symbols.NewIndex()
	cfg := config.DefaultConfig()
	p := NewProvider(idx, "none", "/tmp", nil, cfg)
	p.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return "SomeClass" // non-nullable
	}

	source := `<?php
class Foo {
    public function bar(SomeClass $x): void {
        $x?->method();
    }
}
`
	p.AnalyzeOnSave("file:///test.php", source)
	diags := p.Analyze("file:///test.php", source)

	// Should have redundant-nullsafe finding
	found := false
	for _, d := range diags {
		if d.Code == "redundant-nullsafe" {
			found = true
		}
	}
	if !found {
		t.Error("expected redundant-nullsafe diagnostic from AnalyzeOnSave")
	}
}

func TestFilterByConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.DiagnosticRules = map[string]bool{
		"unused-import":  true,
		"unreachable-code": false,
	}
	p := NewProvider(symbols.NewIndex(), "none", "/tmp", nil, cfg)

	source := `<?php
use App\Models\User;
class Foo {
    public function bar(): string {
        return "x";
        echo "dead";
    }
}
`
	diags := p.Analyze("file:///test.php", source)

	for _, d := range diags {
		if d.Code == "unreachable-code" {
			t.Error("unreachable-code should be filtered out by config")
		}
	}
}

func TestNewIndexMemberChecker(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`)
	idx.IndexFile("file:///vendor/HasMany.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;
class HasMany {}
`)
	idx.IndexFile("file:///app/Category.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;
class Category extends Model {
    public string $name;
    public function products(): HasMany {
        return $this->hasMany(Product::class);
    }
}
`)

	// Run Eloquent analysis to inject virtual members
	idx.IndexFile("file:///app/Product.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Product extends Model {}
`)

	// Manually add a migration-derived virtual property
	idx.AddVirtualMember("App\\Models\\Category", &symbols.Symbol{
		Name:       "$slug",
		FQN:        "App\\Models\\Category::$slug",
		Kind:       symbols.KindProperty,
		IsVirtual:  true,
		Type:       "string",
		DocComment: "From migration",
	})

	checker := NewIndexMemberChecker(idx)

	t.Run("IsColumn finds declared property", func(t *testing.T) {
		if !checker.IsColumn("App\\Models\\Category", "name") {
			t.Error("expected name to be a column")
		}
	})

	t.Run("IsColumn finds migration property", func(t *testing.T) {
		if !checker.IsColumn("App\\Models\\Category", "slug") {
			t.Error("expected slug to be a column")
		}
	})

	t.Run("IsColumn returns false for unknown", func(t *testing.T) {
		if checker.IsColumn("App\\Models\\Category", "nonexistent") {
			t.Error("should not find nonexistent column")
		}
	})

	t.Run("IsDBColumn finds migration property", func(t *testing.T) {
		if !checker.IsDBColumn("App\\Models\\Category", "slug") {
			t.Error("expected slug from migration to be DB column")
		}
	})

	t.Run("IsDBColumn excludes declared property", func(t *testing.T) {
		// Declared properties are not virtual, so IsDBColumn should reject them
		if checker.IsDBColumn("App\\Models\\Category", "name") {
			t.Error("declared PHP property should not be a DB column")
		}
	})

	t.Run("IsRelation finds relation method", func(t *testing.T) {
		if !checker.IsRelation("App\\Models\\Category", "products") {
			t.Error("expected products to be a relation")
		}
	})

	t.Run("IsRelation returns false for non-relation", func(t *testing.T) {
		if checker.IsRelation("App\\Models\\Category", "name") {
			t.Error("name is not a relation")
		}
	})

	t.Run("RelatedModelFQN for singular relation", func(t *testing.T) {
		// Add a BelongsTo relation property manually
		idx.AddVirtualMember("App\\Models\\Category", &symbols.Symbol{
			Name:       "$parent",
			FQN:        "App\\Models\\Category::$parent",
			Kind:       symbols.KindProperty,
			IsVirtual:  true,
			Type:       "?App\\Models\\Category",
			DocComment: "BelongsTo relation",
		})
		fqn := checker.RelatedModelFQN("App\\Models\\Category", "parent")
		if fqn != "App\\Models\\Category" {
			t.Errorf("expected App\\Models\\Category, got %q", fqn)
		}
	})

	t.Run("RelatedModelFQN returns empty for unknown", func(t *testing.T) {
		if checker.RelatedModelFQN("App\\Models\\Category", "unknown") != "" {
			t.Error("expected empty for unknown relation")
		}
	})
}

func TestClearCache(t *testing.T) {
	cfg := config.DefaultConfig()
	p := NewProvider(symbols.NewIndex(), "none", "/tmp", nil, cfg)

	p.mu.Lock()
	p.toolResults["file:///a.php"] = nil
	p.saveResults["file:///a.php"] = nil
	p.mu.Unlock()

	p.ClearCache("file:///a.php")

	p.mu.RLock()
	_, hasTools := p.toolResults["file:///a.php"]
	_, hasSave := p.saveResults["file:///a.php"]
	p.mu.RUnlock()

	if hasTools || hasSave {
		t.Error("ClearCache should remove both tool and save results")
	}
}

func TestFindingsToDiagnosticsWithTags(t *testing.T) {
	findings := []checks.Finding{
		{
			StartLine: 1,
			Severity:  checks.SeverityHint,
			Code:      "unused-import",
			Message:   "test",
			Tags:      []checks.Tag{checks.TagUnnecessary},
		},
	}
	diags := findingsToDiagnostics(findings)
	if len(diags) != 1 {
		t.Fatalf("expected 1, got %d", len(diags))
	}
	if len(diags[0].Tags) != 1 {
		t.Error("expected Unnecessary tag")
	}
}
