package hover

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestPhpManualURLFunction(t *testing.T) {
	tests := []struct {
		name     string
		sym      *symbols.Symbol
		expected string
	}{
		{
			name:     "builtin function",
			sym:      &symbols.Symbol{Name: "array_map", Kind: symbols.KindFunction, URI: "builtin"},
			expected: "https://www.php.net/manual/en/function.array-map.php",
		},
		{
			name:     "builtin class",
			sym:      &symbols.Symbol{Name: "DateTime", Kind: symbols.KindClass, URI: "builtin"},
			expected: "https://www.php.net/manual/en/class.datetime.php",
		},
		{
			name:     "user-defined function",
			sym:      &symbols.Symbol{Name: "myFunc", Kind: symbols.KindFunction, URI: "file:///app.php"},
			expected: "",
		},
		{
			name:     "user-defined class",
			sym:      &symbols.Symbol{Name: "MyClass", Kind: symbols.KindClass, URI: "file:///app.php"},
			expected: "",
		},
		{
			name:     "builtin method - no link",
			sym:      &symbols.Symbol{Name: "format", Kind: symbols.KindMethod, URI: "builtin"},
			expected: "",
		},
		{
			name:     "function with underscores",
			sym:      &symbols.Symbol{Name: "str_starts_with", Kind: symbols.KindFunction, URI: "builtin"},
			expected: "https://www.php.net/manual/en/function.str-starts-with.php",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := phpManualURL(tt.sym)
			if got != tt.expected {
				t.Errorf("phpManualURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
