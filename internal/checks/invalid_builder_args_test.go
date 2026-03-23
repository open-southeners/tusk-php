package checks

import (
	"strings"
	"testing"

	"github.com/open-southeners/php-lsp/internal/parser"
)

// mockMemberChecker implements MemberChecker for testing.
type mockMemberChecker struct {
	columns      map[string]map[string]bool // modelFQN → set of column names
	dbColumns    map[string]map[string]bool
	relations    map[string]map[string]bool
	relatedModel map[string]map[string]string // modelFQN → relationName → relatedModelFQN
}

func (m *mockMemberChecker) IsColumn(modelFQN, name string) bool {
	if cols, ok := m.columns[modelFQN]; ok {
		return cols[name]
	}
	return false
}

func (m *mockMemberChecker) IsDBColumn(modelFQN, name string) bool {
	if cols, ok := m.dbColumns[modelFQN]; ok {
		return cols[name]
	}
	return false
}

func (m *mockMemberChecker) IsRelation(modelFQN, name string) bool {
	if rels, ok := m.relations[modelFQN]; ok {
		return rels[name]
	}
	return false
}

func (m *mockMemberChecker) RelatedModelFQN(modelFQN, relationName string) string {
	if rels, ok := m.relatedModel[modelFQN]; ok {
		return rels[relationName]
	}
	return ""
}

func setupBuilderArgRule() *InvalidBuilderArgRule {
	members := &mockMemberChecker{
		columns: map[string]map[string]bool{
			"App\\Models\\Category": {"id": true, "name": true, "slug": true, "created_at": true},
			"App\\Models\\Product":  {"id": true, "category_id": true, "title": true, "price": true},
		},
		dbColumns: map[string]map[string]bool{
			"App\\Models\\Category": {"id": true, "name": true, "slug": true, "created_at": true},
			"App\\Models\\Product":  {"id": true, "category_id": true, "title": true, "price": true},
		},
		relations: map[string]map[string]bool{
			"App\\Models\\Category": {"products": true},
			"App\\Models\\Product":  {"category": true},
		},
		relatedModel: map[string]map[string]string{
			"App\\Models\\Category": {"products": "App\\Models\\Product"},
			"App\\Models\\Product":  {"category": "App\\Models\\Category"},
		},
	}

	resolver := func(prefix, method, source string, line int, file *parser.FileNode) string {
		if strings.Contains(prefix, "Category") {
			return "App\\Models\\Category"
		}
		if strings.Contains(prefix, "Product") {
			return "App\\Models\\Product"
		}
		return ""
	}

	return &InvalidBuilderArgRule{
		ModelResolver: resolver,
		Members:       members,
	}
}

func TestInvalidBuilderArgRule(t *testing.T) {
	rule := setupBuilderArgRule()

	t.Run("unknown column in where flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('nonexistent', 'value');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
		if !strings.Contains(findings[0].Message, "nonexistent") {
			t.Errorf("expected message to contain 'nonexistent', got: %s", findings[0].Message)
		}
	})

	t.Run("known column in where not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('name', 'value');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unknown relation in with flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('nonexistent');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-relation"})
	})

	t.Run("known relation in with not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('products');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("unknown DB column in get flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::get(['nonexistent']);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
	})

	t.Run("known columns in get not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::get(['id', 'name']);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("array with one bad relation flags only bad one", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with(['products', 'bad']);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-relation"})
		if !strings.Contains(findings[0].Message, "bad") {
			t.Errorf("expected message about 'bad', got: %s", findings[0].Message)
		}
	})

	t.Run("withCount with alias strips alias", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withCount(['products as cnt']);
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("incomplete string no closing quote not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('na
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("non-model receiver not flagged", func(t *testing.T) {
		source := `<?php
$container->get('service');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("Product orderBy known column not flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Product;
Product::orderBy('title');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("Product orderBy unknown column flagged", func(t *testing.T) {
		source := `<?php
use App\Models\Product;
Product::orderBy('nonexistent');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
	})

	t.Run("severity is warning", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::where('bad_col', 'x');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		if findings[0].Severity != SeverityWarning {
			t.Errorf("expected SeverityWarning, got %d", findings[0].Severity)
		}
	})

	t.Run("dot notation validates first segment", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('products.tags');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		// 'products' exists as relation, so first segment is valid
		assertNoFindings(t, findings)
	})

	t.Run("dot notation flags bad first segment", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::with('nonexistent.tags');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-relation"})
	})

	t.Run("withSum second arg validates column on related model", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withSum('products', 'price');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("withSum second arg flags unknown column on related model", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withSum('products', 'nonexistent');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertFindingCodes(t, findings, []string{"unknown-column"})
		if !strings.Contains(findings[0].Message, "Product") {
			t.Errorf("expected message to reference related model 'Product', got: %s", findings[0].Message)
		}
	})

	t.Run("withAvg second arg validates column on related model", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::withAvg('products', 'price');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		assertNoFindings(t, findings)
	})

	t.Run("orderBy with dot notation skipped", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
Category::orderBy('relation.column');
`
		file := parser.ParseFile(source)
		findings := rule.Check(file, source, nil)
		// Dot-notation in column methods is join-qualified — skip validation
		assertNoFindings(t, findings)
	})
}
