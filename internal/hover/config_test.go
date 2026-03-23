package hover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/models"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupConfigHoverProvider(t *testing.T) *Provider {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "config"), 0755)
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

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	resolver := models.NewFrameworkArrayResolver(idx, dir, "laravel")
	p := NewProvider(idx, nil, "laravel")
	p.SetArrayResolver(resolver)
	return p
}

func TestHoverConfigTopLevel(t *testing.T) {
	p := setupConfigHoverProvider(t)

	source := `<?php
config('database');
`
	// Hover on 'database' — cursor inside the string
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 10})
	if hover == nil {
		t.Fatal("expected hover for config('database')")
	}
	if !strings.Contains(hover.Contents.Value, "config") {
		t.Errorf("expected '(config)' in hover, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "default") {
		t.Errorf("expected 'default' key in hover, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "connections") {
		t.Errorf("expected 'connections' key in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverConfigNestedKey(t *testing.T) {
	p := setupConfigHoverProvider(t)

	source := `<?php
config('database.connections');
`
	// Hover on 'connections' part
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 22})
	if hover == nil {
		t.Fatal("expected hover for config('database.connections')")
	}
	if !strings.Contains(hover.Contents.Value, "mysql") {
		t.Errorf("expected 'mysql' in hover, got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "sqlite") {
		t.Errorf("expected 'sqlite' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverConfigLeafValue(t *testing.T) {
	p := setupConfigHoverProvider(t)

	source := `<?php
config('database.default');
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 18})
	if hover == nil {
		t.Fatal("expected hover for config('database.default')")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverConfigDeepNested(t *testing.T) {
	p := setupConfigHoverProvider(t)

	source := `<?php
config('database.connections.mysql');
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 32})
	if hover == nil {
		t.Fatal("expected hover for config('database.connections.mysql')")
	}
	if !strings.Contains(hover.Contents.Value, "host") {
		t.Errorf("expected 'host' in hover for mysql config, got %q", hover.Contents.Value)
	}
}

func TestHoverConfigCursorOutside(t *testing.T) {
	p := setupConfigHoverProvider(t)

	source := `<?php
config('database');
`
	// Cursor on 'config' function name, not inside the string
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 2})
	// Should not return a config hover (might return a function hover or nil)
	if hover != nil && strings.Contains(hover.Contents.Value, "(config)") {
		t.Error("should not show config key hover when cursor is on function name")
	}
}

func TestHoverConfigNonExistentKey(t *testing.T) {
	p := setupConfigHoverProvider(t)

	source := `<?php
config('nonexistent');
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 1, Character: 12})
	if hover != nil && strings.Contains(hover.Contents.Value, "(config)") {
		t.Error("should not show config hover for non-existent config key")
	}
}
