package parser

import (
	"testing"
)

// TestPropertyNodeHooksAndSetVisibility verifies that PHP 8.4 property hooks
// and asymmetric visibility are propagated from PropertyDef through toPropertyNode
// into PropertyNode (L8 — parser/compat.go change).
func TestPropertyNodeHooksAndSetVisibility(t *testing.T) {
	t.Helper()

	// Build a synthetic ParseResult that mimics what the parser produces for a
	// property with PHP 8.4 features so we can test toPropertyNode in isolation.
	hooks := []PropertyHook{{Kind: "get"}, {Kind: "set"}}
	def := PropertyDef{
		Name:          "title",
		Type:          "string",
		Visibility:    "public",
		SetVisibility: "private",
		IsStatic:      false,
		Hooks:         hooks,
		Line:          3,
		DocComment:    "/** doc */",
	}
	result := &ParseResult{} // toPropertyNode only uses result for column lookup; empty is fine
	node := toPropertyNode(result, def)

	t.Run("SetVisibility propagated", func(t *testing.T) {
		if node.SetVisibility != "private" {
			t.Errorf("SetVisibility = %q; want %q", node.SetVisibility, "private")
		}
	})

	t.Run("Hooks propagated", func(t *testing.T) {
		if len(node.Hooks) != 2 {
			t.Fatalf("len(Hooks) = %d; want 2", len(node.Hooks))
		}
		if node.Hooks[0].Kind != "get" {
			t.Errorf("Hooks[0].Kind = %q; want %q", node.Hooks[0].Kind, "get")
		}
		if node.Hooks[1].Kind != "set" {
			t.Errorf("Hooks[1].Kind = %q; want %q", node.Hooks[1].Kind, "set")
		}
	})

	t.Run("Hooks slice is an independent copy", func(t *testing.T) {
		// Mutate original — node.Hooks must not be affected
		hooks[0].Kind = "mutated"
		if node.Hooks[0].Kind == "mutated" {
			t.Error("Hooks slice shares backing array with original PropertyDef; expected independent copy")
		}
	})

	t.Run("other fields still correct", func(t *testing.T) {
		if node.Name != "title" {
			t.Errorf("Name = %q; want %q", node.Name, "title")
		}
		if node.Visibility != "public" {
			t.Errorf("Visibility = %q; want %q", node.Visibility, "public")
		}
		if node.Type.Name != "string" {
			t.Errorf("Type.Name = %q; want %q", node.Type.Name, "string")
		}
	})
}

// TestPropertyNodeNoHooks verifies that a property without hooks (the common case)
// results in a nil/empty Hooks slice and empty SetVisibility.
func TestPropertyNodeNoHooks(t *testing.T) {
	t.Helper()
	def := PropertyDef{
		Name:       "count",
		Type:       "int",
		Visibility: "private",
		Line:       5,
	}
	result := &ParseResult{}
	node := toPropertyNode(result, def)

	if node.SetVisibility != "" {
		t.Errorf("SetVisibility = %q; want empty for plain property", node.SetVisibility)
	}
	if len(node.Hooks) != 0 {
		t.Errorf("Hooks len = %d; want 0 for plain property", len(node.Hooks))
	}
}
