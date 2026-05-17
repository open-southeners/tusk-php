package completion

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// TestM1NoEmptyLabels verifies that GetCompletions never returns a
// CompletionItem with an empty Label, regardless of the code path taken.
// Regression for M1: cursor on the last segment of a use path (e.g.
// "use Illuminate\Support\Str") previously produced at least one item
// with Label == "".
func TestM1NoEmptyLabels(t *testing.T) {
	t.Helper()

	tests := []struct {
		name   string
		source string
		pos    protocol.Position
	}{
		{
			name:   "use path last segment — exact class name",
			source: "<?php\nuse Illuminate\\Support\\Str",
			pos:    protocol.Position{Line: 1, Character: 30},
		},
		{
			name:   "use path trailing backslash",
			source: "<?php\nuse Illuminate\\Support\\",
			pos:    protocol.Position{Line: 1, Character: 28},
		},
		{
			name:   "use path mid-segment partial",
			source: "<?php\nuse Illuminate\\S",
			pos:    protocol.Position{Line: 1, Character: 21},
		},
		{
			name:   "global completion empty line",
			source: "<?php\n",
			pos:    protocol.Position{Line: 1, Character: 0},
		},
		{
			name:   "member access after ->",
			source: "<?php\nnamespace App;\n$svc = new Service();\n$svc->",
			pos:    protocol.Position{Line: 3, Character: 6},
		},
	}

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file:///illuminate.php", `<?php
namespace Illuminate\Support;
class Str {}
class Collection {}
`, symbols.SourceVendor)
	idx.IndexFileWithSource("file:///service.php", `<?php
namespace App;
class Service {
    public function run(): void {}
    public string $name;
}
`, symbols.SourceProject)

	p := NewProvider(idx, nil, "")

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()
			items := p.GetCompletions("file:///test.php", tt.source, tt.pos)
			for i, item := range items {
				if item.Label == "" {
					t.Errorf("item[%d] has empty Label (Kind=%d Detail=%q SortText=%q)",
						i, item.Kind, item.Detail, item.SortText)
				}
			}
		})
	}
}

// TestM2DeterministicOrder verifies that two identical GetCompletions calls on
// the same input produce byte-identical results. Regression for M2: results
// were previously assembled from Go maps and returned in non-deterministic order.
func TestM2DeterministicOrder(t *testing.T) {
	t.Helper()

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file:///models.php", `<?php
namespace App\Models;
class Alpha {}
class Beta {}
class Gamma {}
class Delta {}
`, symbols.SourceProject)
	idx.IndexFileWithSource("file:///services.php", `<?php
namespace App\Services;
class AlphaService {}
class BetaService {}
`, symbols.SourceProject)
	idx.IndexFileWithSource("file:///illuminate.php", `<?php
namespace Illuminate\Support;
class Str {}
class Collection {}
class Arr {}
`, symbols.SourceVendor)

	p := NewProvider(idx, nil, "")

	tests := []struct {
		name   string
		source string
		pos    protocol.Position
	}{
		{
			name:   "use path namespace segments",
			source: "<?php\nuse App\\",
			pos:    protocol.Position{Line: 1, Character: 9},
		},
		{
			name:   "use path direct symbols",
			source: "<?php\nuse App\\Models\\",
			pos:    protocol.Position{Line: 1, Character: 16},
		},
		{
			name:   "use path mid-segment",
			source: "<?php\nuse App\\M",
			pos:    protocol.Position{Line: 1, Character: 10},
		},
		{
			name:   "global completion with prefix",
			source: "<?php\nnamespace App;\nAl",
			pos:    protocol.Position{Line: 2, Character: 2},
		},
		{
			name:   "global completion empty",
			source: "<?php\n",
			pos:    protocol.Position{Line: 1, Character: 0},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Helper()

			// Run the same call twice and compare results.
			first := p.GetCompletions("file:///test.php", tt.source, tt.pos)
			second := p.GetCompletions("file:///test.php", tt.source, tt.pos)

			if !reflect.DeepEqual(first, second) {
				t.Errorf("non-deterministic results for %q:\nfirst:  %s\nsecond: %s",
					tt.name, formatLabels(first), formatLabels(second))
			}
		})
	}
}

// TestM2SortTextBucketsPreserved verifies that existing SortText priority
// buckets ("0"–"5") still produce items in ascending bucket order after the
// M2 deterministic-sort is applied.
func TestM2SortTextBucketsPreserved(t *testing.T) {
	t.Helper()

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFileWithSource("file:///project.php", `<?php
namespace App;
function myFunc(): void {}
`, symbols.SourceProject)
	idx.IndexFileWithSource("file:///vendor.php", `<?php
function vendorFunc(): void {}
`, symbols.SourceVendor)

	p := NewProvider(idx, nil, "")
	source := "<?php\nnamespace App;\n"
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 0})

	// Walk items and confirm SortText values are non-decreasing.
	for i := 1; i < len(items); i++ {
		if items[i].SortText < items[i-1].SortText {
			t.Errorf("items not sorted by SortText at index %d: %q (%q) comes after %q (%q)",
				i, items[i].Label, items[i].SortText, items[i-1].Label, items[i-1].SortText)
		}
	}
}

func formatLabels(items []protocol.CompletionItem) string {
	if len(items) == 0 {
		return "[]"
	}
	s := "["
	for i, item := range items {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%q(sort=%q)", item.Label, item.SortText)
	}
	return s + "]"
}
