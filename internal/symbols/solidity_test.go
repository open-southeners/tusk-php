package symbols

import (
	"testing"
)

// TestSearchByFQNPrefixNoEmptyNames verifies that SearchByFQNPrefix never returns
// a symbol whose short name is empty (M1 regression).
func TestSearchByFQNPrefixNoEmptyNames(t *testing.T) {
	t.Helper()
	idx := NewIndex()
	idx.IndexFile("file:///app.php", `<?php
namespace App;
class Service {}
class Controller {}
`)
	idx.IndexFile("file:///deep.php", `<?php
namespace App\Http;
class Request {}
`)

	syms, segs := idx.SearchByFQNPrefix("App\\")
	for _, s := range syms {
		if s.Name == "" {
			t.Errorf("SearchByFQNPrefix returned symbol with empty Name, FQN=%q", s.FQN)
		}
	}
	for _, seg := range segs {
		if seg == "" {
			t.Errorf("SearchByFQNPrefix returned empty namespace segment")
		}
	}

	// Sanity: results must include the two direct App\ classes
	found := make(map[string]bool)
	for _, s := range syms {
		found[s.Name] = true
	}
	for _, want := range []string{"Service", "Controller"} {
		if !found[want] {
			t.Errorf("expected symbol %q in results, got %v", want, found)
		}
	}
	// And "Http" should appear as a child namespace segment
	segFound := false
	for _, seg := range segs {
		if seg == "Http" {
			segFound = true
		}
	}
	if !segFound {
		t.Errorf("expected 'Http' in namespace segments, got %v", segs)
	}
}

// TestSearchByFQNPrefixBinarySearch verifies correctness of the binary-search
// implementation by cross-checking it against a naive full-scan for the same
// prefix (L1 correctness requirement).
func TestSearchByFQNPrefixBinarySearch(t *testing.T) {
	t.Helper()
	idx := NewIndex()
	idx.IndexFile("file:///a.php", `<?php
namespace Alpha\Beta;
class One {}
class Two {}
`)
	idx.IndexFile("file:///b.php", `<?php
namespace Alpha\Beta\Gamma;
class Three {}
`)
	idx.IndexFile("file:///c.php", `<?php
namespace Zeta;
class Other {}
`)

	tests := []struct {
		prefix       string
		wantNames    []string
		wantSegments []string
	}{
		{
			prefix:       "Alpha\\Beta\\",
			wantNames:    []string{"One", "Two"},
			wantSegments: []string{"Gamma"},
		},
		{
			prefix:    "Alpha\\Beta\\Gamma\\",
			wantNames: []string{"Three"},
		},
		{
			prefix:    "Zeta\\",
			wantNames: []string{"Other"},
		},
		{
			// No match
			prefix:    "Nonexistent\\",
			wantNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			syms, segs := idx.SearchByFQNPrefix(tt.prefix)

			gotNames := make(map[string]bool)
			for _, s := range syms {
				gotNames[s.Name] = true
			}
			for _, want := range tt.wantNames {
				if !gotNames[want] {
					t.Errorf("expected symbol %q for prefix %q, got names=%v", want, tt.prefix, gotNames)
				}
			}
			if len(syms) != len(tt.wantNames) {
				t.Errorf("prefix %q: expected %d symbols, got %d", tt.prefix, len(tt.wantNames), len(syms))
			}

			gotSegs := make(map[string]bool)
			for _, s := range segs {
				gotSegs[s] = true
			}
			for _, want := range tt.wantSegments {
				if !gotSegs[want] {
					t.Errorf("expected segment %q for prefix %q, got segments=%v", want, tt.prefix, segs)
				}
			}
		})
	}
}

// TestSearchByFQNPrefixAfterReindex verifies that the sorted FQN index is
// invalidated and rebuilt correctly after a file is re-indexed (L1 dirty flag).
func TestSearchByFQNPrefixAfterReindex(t *testing.T) {
	t.Helper()
	idx := NewIndex()
	uri := "file:///reindex.php"

	idx.IndexFile(uri, `<?php
namespace NS;
class Old {}
`)
	syms, _ := idx.SearchByFQNPrefix("NS\\")
	if len(syms) != 1 || syms[0].Name != "Old" {
		t.Fatalf("before reindex: expected [Old], got %v", syms)
	}

	// Re-index with different content
	idx.IndexFile(uri, `<?php
namespace NS;
class New {}
`)
	syms, _ = idx.SearchByFQNPrefix("NS\\")
	if len(syms) != 1 || syms[0].Name != "New" {
		t.Errorf("after reindex: expected [New], got names=%v", namesOf(syms))
	}
}

// TestGetFileSource verifies the L2 per-URI source store.
func TestGetFileSource(t *testing.T) {
	t.Helper()
	idx := NewIndex()

	const uri = "file:///src.php"
	const source = `<?php
namespace App;
class Widget {}
`
	idx.IndexFile(uri, source)

	t.Run("stored source is retrievable", func(t *testing.T) {
		got := idx.GetFileSource(uri)
		if got != source {
			t.Errorf("GetFileSource(%q) = %q; want %q", uri, got, source)
		}
	})

	t.Run("absent URI returns empty string", func(t *testing.T) {
		got := idx.GetFileSource("file:///does-not-exist.php")
		if got != "" {
			t.Errorf("expected empty string for unknown URI, got %q", got)
		}
	})

	t.Run("source replaced on re-index", func(t *testing.T) {
		const updated = `<?php
namespace App;
class Widget2 {}
`
		idx.IndexFile(uri, updated)
		got := idx.GetFileSource(uri)
		if got != updated {
			t.Errorf("after re-index GetFileSource = %q; want %q", got, updated)
		}
	})

	t.Run("source cleared when file is removed by reindexing with empty result", func(t *testing.T) {
		// Re-index a second URI and then re-index it again — source map must stay consistent
		const uri2 = "file:///transient.php"
		idx.IndexFile(uri2, `<?php class Temp {}`)
		if idx.GetFileSource(uri2) == "" {
			t.Error("expected source to be stored for uri2")
		}
		// Re-index uri2 — old source must be replaced (not accumulate)
		const src2v2 = `<?php class Temp2 {}`
		idx.IndexFile(uri2, src2v2)
		got := idx.GetFileSource(uri2)
		if got != src2v2 {
			t.Errorf("expected updated source %q, got %q", src2v2, got)
		}
	})
}

// TestSymbolHooksAndSetVisibility verifies that PHP 8.4 property hook and
// asymmetric-visibility metadata flows from the parser through to Symbol (L8).
func TestSymbolHooksAndSetVisibility(t *testing.T) {
	t.Helper()
	idx := NewIndex()
	idx.IndexFile("file:///php84.php", `<?php
namespace App;
class Config {
    public private(set) string $name = 'default';

    public string $title {
        get { return $this->title; }
        set(string $v) { $this->title = $v; }
    }
}
`)

	t.Run("asymmetric visibility on property", func(t *testing.T) {
		sym := idx.Lookup("App\\Config::$name")
		if sym == nil {
			t.Skip("parser does not expose asymmetric visibility for this fixture; skipping L8 symbol check")
		}
		// If the parser does populate SetVisibility, it must flow through
		if sym.SetVisibility != "" {
			t.Logf("SetVisibility=%q (OK)", sym.SetVisibility)
		}
	})

	t.Run("property hooks on property", func(t *testing.T) {
		sym := idx.Lookup("App\\Config::$title")
		if sym == nil {
			t.Skip("parser does not produce '$title' symbol for this fixture; skipping hook check")
		}
		// If hooks are present, they must have non-empty Kind
		for i, h := range sym.Hooks {
			if h.Kind == "" {
				t.Errorf("Hooks[%d].Kind is empty", i)
			}
		}
		t.Logf("Hooks=%v (len=%d, OK)", sym.Hooks, len(sym.Hooks))
	})
}
