package hover

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

// phpManualURL returns a PHP manual URL for built-in symbols, or empty string for user-defined ones.
func phpManualURL(sym *symbols.Symbol) string {
	if sym.URI != "builtin" {
		return ""
	}
	switch sym.Kind {
	case symbols.KindFunction:
		slug := strings.ReplaceAll(strings.ToLower(sym.Name), "_", "-")
		return "https://www.php.net/manual/en/function." + slug + ".php"
	case symbols.KindClass:
		return "https://www.php.net/manual/en/class." + strings.ToLower(sym.Name) + ".php"
	}
	return ""
}
