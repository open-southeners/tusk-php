package completion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/php-lsp/internal/models"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

func setupConfigProvider(t *testing.T) (*Provider, string) {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "config", "app.php"), []byte(`<?php
return [
    'name' => 'My App',
    'debug' => true,
    'url' => 'http://localhost',
];
`), 0644)
	os.WriteFile(filepath.Join(dir, "config", "database.php"), []byte(`<?php
return [
    'default' => 'mysql',
    'connections' => [
        'mysql' => [
            'host' => 'localhost',
            'port' => 3306,
        ],
        'sqlite' => [
            'driver' => 'sqlite',
            'database' => ':memory:',
        ],
    ],
];
`), 0644)
	os.WriteFile(filepath.Join(dir, "config", "cache.php"), []byte(`<?php
return [
    'default' => 'file',
    'stores' => [
        'file' => ['driver' => 'file', 'path' => '/tmp'],
    ],
];
`), 0644)

	idx := symbols.NewIndex()
	resolver := models.NewFrameworkArrayResolver(idx, dir, "laravel")
	p := NewProvider(idx, nil, "laravel")
	p.SetArrayResolver(resolver)
	return p, dir
}

func TestExtractConfigArgContext(t *testing.T) {
	tests := []struct {
		name       string
		trimmed    string
		wantPath   string
		wantPartial string
		wantQuote  string
		wantOk     bool
	}{
		{"empty single quote", "config('", "", "", "'", true},
		{"empty double quote", `config("`, "", "", "\"", true},
		{"partial top level", "config('da", "", "da", "'", true},
		{"top level complete dot", "config('database.", "database", "", "'", true},
		{"nested partial", "config('database.co", "database", "co", "'", true},
		{"deep nested", "config('database.connections.", "database.connections", "", "'", true},
		{"deep nested partial", "config('database.connections.my", "database.connections", "my", "'", true},
		{"closed paren", "config('database')", "", "", "", false},
		{"not config", "app('something", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, partial, quote, ok := extractConfigArgContext(tt.trimmed)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if path != tt.wantPath {
				t.Errorf("configPath = %q, want %q", path, tt.wantPath)
			}
			if partial != tt.wantPartial {
				t.Errorf("partial = %q, want %q", partial, tt.wantPartial)
			}
			if quote != tt.wantQuote {
				t.Errorf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestCompleteConfigTopLevel(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 8})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["app"] {
		t.Error("expected 'app' config file")
	}
	if !labels["database"] {
		t.Error("expected 'database' config file")
	}
	if !labels["cache"] {
		t.Error("expected 'cache' config file")
	}
}

func TestCompleteConfigTopLevelFilter(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('da
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 10})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["database"] {
		t.Error("expected 'database' matching 'da'")
	}
	if labels["app"] {
		t.Error("should NOT show 'app' (doesn't match 'da')")
	}
}

func TestCompleteConfigNestedKeys(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["default"] {
		t.Error("expected 'default' from database config")
	}
	if !labels["connections"] {
		t.Error("expected 'connections' from database config")
	}
}

func TestCompleteConfigDeepNested(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.connections.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 29})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' connection")
	}
	if !labels["sqlite"] {
		t.Error("expected 'sqlite' connection")
	}
}

func TestCompleteConfigDeepNestedPartial(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.connections.my
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 31})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["mysql"] {
		t.Error("expected 'mysql' matching 'my'")
	}
	if labels["sqlite"] {
		t.Error("should NOT show 'sqlite' (doesn't match 'my')")
	}
}

func TestCompleteConfigNestedAppendsTrailingDot(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	for _, item := range items {
		if item.Label == "connections" {
			if item.InsertText != "connections." {
				t.Errorf("expected InsertText 'connections.' for nested key, got %q", item.InsertText)
			}
			if item.Kind != protocol.CompletionItemKindModule {
				t.Errorf("expected Module kind for nested config, got %d", item.Kind)
			}
			return
		}
	}
	t.Error("'connections' not found in completions")
}

func TestCompleteConfigLeafNoTrailingDot(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config('database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	for _, item := range items {
		if item.Label == "default" {
			if item.InsertText != "default" {
				t.Errorf("expected InsertText 'default' for leaf key, got %q", item.InsertText)
			}
			if item.Kind != protocol.CompletionItemKindProperty {
				t.Errorf("expected Property kind for leaf config, got %d", item.Kind)
			}
			return
		}
	}
	t.Error("'default' not found in completions")
}

func TestCompleteConfigDoesNotBreakContainerCompletion(t *testing.T) {
	// app() should still trigger container completion, not config
	_, path, quote, ok := extractConfigArgContext("app('re")
	if ok {
		t.Errorf("app() should NOT trigger config context, got path=%q quote=%q", path, quote)
	}
}

func TestCompleteConfigDoubleQuotes(t *testing.T) {
	p, _ := setupConfigProvider(t)

	source := `<?php
config("database.
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 17})

	if len(items) == 0 {
		t.Fatal("expected completions with double quotes")
	}

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["connections"] {
		t.Error("expected 'connections' with double quotes")
	}
}
