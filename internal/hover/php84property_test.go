package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// setupPHP84Provider builds a minimal provider with a PHP 8.4 class that has
// asymmetric-visibility and hooked properties so hover tests can remain self-contained.
func setupPHP84Provider(t *testing.T) *Provider {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/Model.php", `<?php
namespace App;
class Model {
    public private(set) string $id;
    public string $name { get; set; }
    public string $plain;
}
`)
	return NewProvider(idx, nil, "none")
}

// TestFormatHoverPropertyAsymmetricVisibility verifies that a property with
// SetVisibility renders as "public private(set) string $id".
func TestFormatHoverPropertyAsymmetricVisibility(t *testing.T) {
	p := setupPHP84Provider(t)

	sym := &symbols.Symbol{
		Name:          "id",
		FQN:           "App\\Model::$id",
		Kind:          symbols.KindProperty,
		ParentFQN:     "App\\Model",
		Visibility:    "public",
		SetVisibility: "private",
		Type:          "string",
	}

	decl := p.formatHoverDeclaration(sym)

	if !strings.Contains(decl, "private(set)") {
		t.Errorf("expected asymmetric visibility in declaration, got: %s", decl)
	}
	if !strings.Contains(decl, "public") {
		t.Errorf("expected read visibility 'public' in declaration, got: %s", decl)
	}
	if !strings.Contains(decl, "string $id") {
		t.Errorf("expected type and name in declaration, got: %s", decl)
	}
	// Ensure it reads in the right order: public private(set) string $id
	wantPrefix := "public private(set)"
	if !strings.HasPrefix(decl, wantPrefix) {
		t.Errorf("expected declaration to start with %q, got: %s", wantPrefix, decl)
	}
}

// TestFormatHoverPropertyHooks verifies that a property with hooks renders
// them as "{ get; set; }" appended to the declaration.
func TestFormatHoverPropertyHooks(t *testing.T) {
	p := setupPHP84Provider(t)

	sym := &symbols.Symbol{
		Name:       "name",
		FQN:        "App\\Model::$name",
		Kind:       symbols.KindProperty,
		ParentFQN:  "App\\Model",
		Visibility: "public",
		Type:       "string",
		Hooks: []parser.PropertyHook{
			{Kind: "get"},
			{Kind: "set"},
		},
	}

	decl := p.formatHoverDeclaration(sym)

	if !strings.Contains(decl, "{ get; set; }") {
		t.Errorf("expected hook block in declaration, got: %s", decl)
	}
	if !strings.Contains(decl, "string $name") {
		t.Errorf("expected type and name in declaration, got: %s", decl)
	}
}

// TestFormatHoverPropertyGetHookOnly verifies a property with only a get hook.
func TestFormatHoverPropertyGetHookOnly(t *testing.T) {
	p := setupPHP84Provider(t)

	sym := &symbols.Symbol{
		Name:       "computed",
		FQN:        "App\\Model::$computed",
		Kind:       symbols.KindProperty,
		ParentFQN:  "App\\Model",
		Visibility: "public",
		Type:       "string",
		Hooks: []parser.PropertyHook{
			{Kind: "get"},
		},
	}

	decl := p.formatHoverDeclaration(sym)

	if !strings.Contains(decl, "{ get; }") {
		t.Errorf("expected get-only hook block in declaration, got: %s", decl)
	}
}

// TestFormatHoverPropertyPlainUnchanged verifies that a plain property (no PHP 8.4
// metadata) renders exactly as before — no hooks suffix, no asymmetric visibility.
func TestFormatHoverPropertyPlainUnchanged(t *testing.T) {
	p := setupPHP84Provider(t)

	sym := &symbols.Symbol{
		Name:       "plain",
		FQN:        "App\\Model::$plain",
		Kind:       symbols.KindProperty,
		ParentFQN:  "App\\Model",
		Visibility: "public",
		Type:       "string",
	}

	decl := p.formatHoverDeclaration(sym)
	want := "public string $plain"

	if decl != want {
		t.Errorf("plain property declaration changed: want %q, got %q", want, decl)
	}
}

// TestFormatHoverPropertyAsymmetricAndHooks tests a property that has both
// asymmetric visibility and hooks simultaneously.
func TestFormatHoverPropertyAsymmetricAndHooks(t *testing.T) {
	p := setupPHP84Provider(t)

	sym := &symbols.Symbol{
		Name:          "value",
		FQN:           "App\\Model::$value",
		Kind:          symbols.KindProperty,
		ParentFQN:     "App\\Model",
		Visibility:    "public",
		SetVisibility: "protected",
		Type:          "int",
		Hooks: []parser.PropertyHook{
			{Kind: "get"},
			{Kind: "set"},
		},
	}

	decl := p.formatHoverDeclaration(sym)

	if !strings.Contains(decl, "protected(set)") {
		t.Errorf("expected protected(set) in declaration, got: %s", decl)
	}
	if !strings.Contains(decl, "{ get; set; }") {
		t.Errorf("expected hook block in declaration, got: %s", decl)
	}
	if !strings.Contains(decl, "int $value") {
		t.Errorf("expected type and name in declaration, got: %s", decl)
	}
}

// TestHoverCardPropertyAsymmetricVisibilityIntegration uses GetHover to
// exercise the full hover card path for a property with asymmetric visibility.
func TestHoverCardPropertyAsymmetricVisibilityIntegration(t *testing.T) {
	p := setupPHP84Provider(t)

	source := `<?php
namespace App;
class Consumer {
    public function run(Model $m): void {
        $m->id;
    }
}
`

	// Index the consumer file so the provider knows about it
	idx := p.index
	idx.IndexFile("file:///app/Consumer.php", source)

	// Position cursor on "id" inside "$m->id"
	pos := charPosOf(t, source, "id", "$m->id")
	hover := p.GetHover("file:///app/Consumer.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for $m->id")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "private(set)") {
		t.Errorf("hover card should show asymmetric visibility, got:\n%s", val)
	}
}

// TestHoverCardPropertyHooksIntegration uses GetHover to exercise the full
// hover card path for a property with hooks.
func TestHoverCardPropertyHooksIntegration(t *testing.T) {
	p := setupPHP84Provider(t)

	source := `<?php
namespace App;
class Consumer {
    public function run(Model $m): void {
        $m->name;
    }
}
`
	idx := p.index
	idx.IndexFile("file:///app/Consumer2.php", source)

	pos := charPosOf(t, source, "name", "$m->name")
	hover := p.GetHover("file:///app/Consumer2.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for $m->name")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "get;") || !strings.Contains(val, "set;") {
		t.Errorf("hover card should show hooks, got:\n%s", val)
	}
}
