package completion

import (
	"fmt"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// resolveContainerCallType checks if the expression is a container resolution call
// like app('request'), app(Request::class), resolve('cache'), $container->get('log')
// and returns the concrete FQN from the container bindings.
// config() with no args returns Repository; config('key') returns mixed (no resolution).
func (p *Provider) resolveContainerCallType(expr, source string, file *parser.FileNode) string {
	// Special case: config() with no args returns the Repository instance
	t := strings.TrimSpace(expr)
	if t == "config()" {
		if binding := p.container.ResolveDependency("config"); binding != nil {
			return binding.Concrete
		}
		return "Illuminate\\Config\\Repository"
	}
	// config('key') returns a mixed value, not a class — signal to stop resolution
	if strings.HasPrefix(t, "config(") && strings.HasSuffix(t, ")") {
		return "-"
	}

	arg := ExtractContainerCallArg(expr)
	if arg == "" {
		return ""
	}
	// Resolve ::class references: "Request::class" → FQN
	if strings.HasSuffix(arg, "::class") {
		className := strings.TrimSuffix(arg, "::class")
		arg = p.resolveClassNameFromSource(className, source, file)
	}
	if binding := p.container.ResolveDependency(arg); binding != nil {
		return binding.Concrete
	}
	// Try direct lookup if arg is already a FQN
	if p.index.Lookup(arg) != nil {
		return arg
	}
	return ""
}

// ExtractContainerCallArg is a backward-compatible wrapper for resolve.ExtractContainerCallArg.
// Deprecated: use resolve.ExtractContainerCallArg directly.
func ExtractContainerCallArg(expr string) string {
	return resolve.ExtractContainerCallArg(expr)
}

// extractContainerArgContext detects whether the cursor is inside a container
// resolution call like app(...), $container->get(...), $container->make(...),
// resolve(...). Returns the partial argument text, the quote character used
// (empty string if no quote), and true if matched.
func extractContainerArgContext(trimmed string) (string, string, bool) {
	patterns := []string{"app(", "resolve(", "$container->get(", "$container->make(", "$this->app->make("}
	for _, pat := range patterns {
		idx := strings.LastIndex(trimmed, pat)
		if idx < 0 {
			continue
		}
		after := trimmed[idx+len(pat):]
		if strings.Contains(after, ")") {
			continue
		}
		quote := ""
		if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
			quote = string(after[0])
			after = after[1:]
		}
		return after, quote, true
	}
	return "", "", false
}

// completeContainerResolve returns completion items for container resolution calls.
// quoteCtx is the opening quote character already typed ("'" or "\""), or empty
// if the user hasn't typed a quote yet. String bindings get wrapped in quotes
// accordingly.
func (p *Provider) completeContainerResolve(source, filter, currentNS, quoteCtx string, file *parser.FileNode) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	lfilter := strings.ToLower(filter)

	q := "'"
	if quoteCtx == "\"" {
		q = "\""
	}

	if p.container != nil {
		for abstract, binding := range p.container.GetBindings() {
			if lfilter != "" && !strings.HasPrefix(strings.ToLower(abstract), lfilter) {
				parts := strings.Split(abstract, "\\")
				shortName := parts[len(parts)-1]
				if !strings.HasPrefix(strings.ToLower(shortName), lfilter) {
					continue
				}
			}
			d := fmt.Sprintf("-> %s", binding.Concrete)
			if binding.Singleton {
				d += " (singleton)"
			}
			label := abstract
			sortText := "1" + abstract

			var insertText string
			if !strings.Contains(abstract, "\\") {
				sortText = "0" + abstract
			}
			if quoteCtx != "" {
				insertText = abstract
			} else {
				insertText = q + abstract + q
			}

			items = append(items, protocol.CompletionItem{
				Label:      label,
				Kind:       protocol.CompletionItemKindValue,
				Detail:     d,
				InsertText: insertText,
				SortText:   sortText,
			})
		}
	}

	for _, sym := range p.index.SearchByPrefix(filter) {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum:
			if p.container != nil {
				if _, bound := p.container.GetBindings()[sym.FQN]; bound {
					continue
				}
			}
			insertName := sym.FQN
			if file != nil {
				for _, u := range file.Uses {
					if u.FullName == sym.FQN {
						insertName = u.Alias
						break
					}
				}
			}
			classInsert := insertName + "::class"
			if quoteCtx != "" {
				classInsert = insertName + "::class"
			}
			items = append(items, protocol.CompletionItem{
				Label:      sym.Name + "::class",
				Kind:       protocol.CompletionItemKindClass,
				Detail:     sym.FQN,
				InsertText: classInsert,
				SortText:   sortPriority(sym, currentNS),
			})
		}
	}

	return items
}
