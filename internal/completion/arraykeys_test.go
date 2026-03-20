package completion

import (
	"testing"

	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

func TestExtractArrayKeyContext(t *testing.T) {
	tests := []struct {
		name      string
		prefix    string
		wantVar   string
		wantPart  string
		wantQuote string
		wantOk    bool
	}{
		{"single quote start", "        $config['", "$config", "", "'", true},
		{"double quote start", `        $config["`, "$config", "", "\"", true},
		{"partial key", "        $config['da", "$config", "da", "'", true},
		{"bracket only", "        $config[", "$config", "", "", true},
		{"nested in call", "        foo($config['k", "$config", "k", "'", true},
		{"not array access", "        foo('bar", "", "", "", false},
		{"no variable", "        ['key", "", "", "", false},
		{"method call not var", "        $this->method('", "", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			varName, partial, quote, ok := extractArrayKeyContext(tt.prefix)
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
			if partial != tt.wantPart {
				t.Errorf("partial = %q, want %q", partial, tt.wantPart)
			}
			if quote != tt.wantQuote {
				t.Errorf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestCompleteArrayKeysFromDocblock(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///test.php", `<?php
namespace App;
class Service {
    /**
     * @param array{host: string, port: int, ssl?: bool} $config
     */
    public function connect(array $config): void {}
}
`)
	p := NewProvider(idx, nil, "")

	source := `<?php
namespace App;
class Service {
    /**
     * @param array{host: string, port: int, ssl?: bool} $config
     */
    public function connect(array $config): void {
        $config['
    }
}
`
	// Cursor at $config[' — line 7, after the quote
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 17})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' key completion")
	}
	if !labels["port"] {
		t.Error("expected 'port' key completion")
	}
	if !labels["ssl"] {
		t.Error("expected 'ssl' key completion")
	}

	// Check ssl is marked optional
	for _, item := range items {
		if item.Label == "ssl" {
			if item.SortText[0:1] != "1" {
				t.Errorf("optional key 'ssl' should sort after required keys, got SortText %q", item.SortText)
			}
		}
	}
}

func TestCompleteArrayKeysFromLiteralAssignment(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $data = [
        'name' => 'John',
        'email' => 'john@example.com',
        'age' => 30,
    ];
    $data['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 7, Character: 11})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["name"] {
		t.Error("expected 'name' from literal assignment")
	}
	if !labels["email"] {
		t.Error("expected 'email' from literal assignment")
	}
	if !labels["age"] {
		t.Error("expected 'age' from literal assignment")
	}
}

func TestCompleteArrayKeysFromIncrementalAssignment(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $result = [];
    $result['name'] = 'test';
    $result['count'] = 42;
    $result['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 5, Character: 13})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["name"] {
		t.Error("expected 'name' from incremental assignment")
	}
	if !labels["count"] {
		t.Error("expected 'count' from incremental assignment")
	}
}

func TestCompleteArrayKeysFiltersOnPartial(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $config = ['host' => 'x', 'port' => 3306, 'hostname' => 'y'];
    $config['ho
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 15})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["host"] {
		t.Error("expected 'host' matching 'ho' prefix")
	}
	if !labels["hostname"] {
		t.Error("expected 'hostname' matching 'ho' prefix")
	}
	if labels["port"] {
		t.Error("should NOT show 'port' (doesn't match 'ho')")
	}
}

func TestCompleteArrayKeysFromVarAnnotation(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    /** @var array{id: int, title: string, published?: bool} $post */
    $post = getPost();
    $post['
}
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 4, Character: 11})

	labels := make(map[string]bool)
	for _, item := range items {
		labels[item.Label] = true
	}

	if !labels["id"] {
		t.Error("expected 'id' from @var annotation")
	}
	if !labels["title"] {
		t.Error("expected 'title' from @var annotation")
	}
	if !labels["published"] {
		t.Error("expected 'published' from @var annotation")
	}
}

func TestCompleteArrayKeysQuotingBehavior(t *testing.T) {
	idx := symbols.NewIndex()
	p := NewProvider(idx, nil, "")

	source := `<?php
function test() {
    $arr = ['key1' => 1, 'key2' => 2];
    $arr[
}
`
	// No quote typed yet — bracket only
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 9})

	if len(items) > 0 {
		// Should wrap keys in quotes
		for _, item := range items {
			if item.InsertText != "" && item.InsertText[0] != '\'' {
				t.Errorf("expected quote-wrapped key, got InsertText %q", item.InsertText)
			}
		}
	}
}
