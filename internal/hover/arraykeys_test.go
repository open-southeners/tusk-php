package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

func TestGetArrayKeyAt(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		char      int
		wantVar   string
		wantKey   string
		wantOk    bool
	}{
		{"single quote", "        $config['host']", 18, "$config", "host", true},
		{"double quote", `        $config["host"]`, 18, "$config", "host", true},
		{"cursor on key start", "        $config['host']", 17, "$config", "host", true},
		{"cursor on key end", "        $config['host']", 20, "$config", "host", true},
		{"not in string", "        $config[0]", 17, "", "", false},
		{"no bracket", "        echo 'hello'", 14, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			varName, key, ok := getArrayKeyAt(tt.line, tt.char)
			if ok != tt.wantOk {
				t.Errorf("ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !ok {
				return
			}
			if varName != tt.wantVar {
				t.Errorf("varName = %q, want %q", varName, tt.wantVar)
			}
			if key != tt.wantKey {
				t.Errorf("key = %q, want %q", key, tt.wantKey)
			}
		})
	}
}

func TestHoverArrayKeyFromDocblock(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{host: string, port: int, ssl?: bool} $config
 */
function connect(array $config): void {
    $config['host'];
}
`
	// Hover on 'host' at line 5
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 14})
	if hover == nil {
		t.Fatal("expected hover for array key 'host'")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected hover to contain type 'string', got %q", hover.Contents.Value)
	}
	if !strings.Contains(hover.Contents.Value, "host") {
		t.Errorf("expected hover to contain key name 'host', got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyOptional(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{name: string, nickname?: string} $data
 */
function test(array $data): void {
    $data['nickname'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 12})
	if hover == nil {
		t.Fatal("expected hover for optional array key")
	}
	if !strings.Contains(hover.Contents.Value, "optional") {
		t.Errorf("expected hover to indicate optional, got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyFromLiteral(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $data = ['name' => 'John', 'age' => 30];
    $data['name'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 12})
	if hover == nil {
		t.Fatal("expected hover for literal array key")
	}
	if !strings.Contains(hover.Contents.Value, "name") {
		t.Errorf("expected hover to contain 'name', got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyFromVarAnnotation(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    /** @var array{id: int, title: string} $post */
    $post = getPost();
    $post['title'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 4, Character: 12})
	if hover == nil {
		t.Fatal("expected hover for @var array key")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverNestedArrayKey(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{database: array{host: string, port: int}, cache: array{driver: string}} $config
 */
function connect(array $config): void {
    $config['database']['host'];
}
`
	// Hover on 'host' inside $config['database']['host']
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 27})
	if hover == nil {
		t.Fatal("expected hover for nested array key 'host'")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverNestedArrayKeyWrongParent(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	p := NewProvider(idx, nil, "")

	source := `<?php
/**
 * @param array{database: array{host: string}, cache: array{driver: string}} $config
 */
function connect(array $config): void {
    $config['cache']['host'];
}
`
	// 'host' doesn't exist in cache shape, only in database
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 5, Character: 24})
	if hover != nil && strings.Contains(hover.Contents.Value, "array key") {
		t.Error("should not show array key hover for 'host' in cache (wrong parent)")
	}
}

func TestHoverNestedLiteralArrayKey(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $config = [
        'database' => [
            'host' => 'localhost',
            'port' => 3306,
        ],
    ];
    $config['database']['host'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 8, Character: 27})
	if hover == nil {
		t.Fatal("expected hover for nested literal array key 'host'")
	}
	if !strings.Contains(hover.Contents.Value, "string") {
		t.Errorf("expected type 'string' in hover, got %q", hover.Contents.Value)
	}
}

func TestHoverArrayKeyNonExistent(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $data = ['name' => 'John'];
    $data['unknown'];
}
`
	hover := p.GetHover("file:///test.php", source, protocol.Position{Line: 3, Character: 12})
	if hover != nil {
		// Should not contain type info for non-existent key
		if strings.Contains(hover.Contents.Value, "array key") {
			t.Error("expected no array key hover for non-existent key")
		}
	}
}
