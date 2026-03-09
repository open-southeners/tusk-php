package hover

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

type Provider struct {
	index     *symbols.Index
	container *container.ContainerAnalyzer
	framework string
}

func NewProvider(index *symbols.Index, ca *container.ContainerAnalyzer, framework string) *Provider {
	return &Provider{index: index, container: ca, framework: framework}
}

func (p *Provider) GetHover(uri, source string, pos protocol.Position) *protocol.Hover {
	word := getWordAt(source, pos)
	if word == "" {
		return nil
	}
	if strings.HasPrefix(word, "$") {
		return p.hoverVariable(source, word)
	}
	syms := p.index.LookupByName(word)
	if len(syms) == 0 {
		return nil
	}
	content := p.formatHover(syms[0])
	if content == "" {
		return nil
	}
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
}

func (p *Provider) hoverVariable(source, varName string) *protocol.Hover {
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			for _, param := range m.Params {
				if param.Name == varName {
					t := param.Type.Name
					if t == "" {
						t = "mixed"
					}
					content := fmt.Sprintf("```php\n%s %s\n```", t, varName)
					if binding := p.container.ResolveDependency(t); binding != nil {
						content += fmt.Sprintf("\n\n---\n**Container Binding**\n- Concrete: `%s`\n- Singleton: %v", binding.Concrete, binding.Singleton)
					}
					return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
				}
			}
		}
	}
	return nil
}

func (p *Provider) formatHover(sym *symbols.Symbol) string {
	var sb strings.Builder
	switch sym.Kind {
	case symbols.KindClass:
		sb.WriteString(fmt.Sprintf("```php\nclass %s", sym.FQN))
		if chain := p.index.GetInheritanceChain(sym.FQN); len(chain) > 0 {
			sb.WriteString(fmt.Sprintf(" extends %s", chain[0]))
		}
		sb.WriteString("\n```\n")
		if impls := p.index.GetImplementors(sym.FQN); len(impls) > 0 {
			sb.WriteString("\n**Implemented by:**\n")
			for _, impl := range impls {
				sb.WriteString(fmt.Sprintf("- `%s`\n", impl.FQN))
			}
		}
	case symbols.KindInterface:
		sb.WriteString(fmt.Sprintf("```php\ninterface %s\n```\n", sym.FQN))
		if impls := p.index.GetImplementors(sym.FQN); len(impls) > 0 {
			sb.WriteString("\n**Implementations:**\n")
			for _, impl := range impls {
				sb.WriteString(fmt.Sprintf("- `%s`\n", impl.FQN))
			}
		}
		if binding := p.container.ResolveDependency(sym.FQN); binding != nil {
			sb.WriteString(fmt.Sprintf("\n**Container -> `%s`**", binding.Concrete))
			if binding.Singleton {
				sb.WriteString(" (singleton)")
			}
		}
	case symbols.KindMethod:
		vis := sym.Visibility
		if vis == "" {
			vis = "public"
		}
		if sym.IsStatic {
			vis += " static"
		}
		sb.WriteString(fmt.Sprintf("```php\n%s function %s%s", vis, sym.Name, fmtParams(sym.Params)))
		if sym.ReturnType != "" {
			sb.WriteString(": " + sym.ReturnType)
		}
		sb.WriteString("\n```\n")
		if sym.ParentFQN != "" {
			sb.WriteString(fmt.Sprintf("\nDefined in `%s`\n", sym.ParentFQN))
		}
	case symbols.KindFunction:
		sb.WriteString(fmt.Sprintf("```php\nfunction %s%s", sym.Name, fmtParams(sym.Params)))
		if sym.ReturnType != "" {
			sb.WriteString(": " + sym.ReturnType)
		}
		sb.WriteString("\n```\n")
	case symbols.KindProperty:
		vis := sym.Visibility
		if vis == "" {
			vis = "public"
		}
		t := sym.Type
		if t == "" {
			t = "mixed"
		}
		sb.WriteString(fmt.Sprintf("```php\n%s %s %s\n```\n", vis, t, sym.Name))
	case symbols.KindEnum:
		sb.WriteString(fmt.Sprintf("```php\nenum %s\n```\n", sym.FQN))
	case symbols.KindConstant:
		sb.WriteString(fmt.Sprintf("```php\nconst %s\n```\n", sym.Name))
	case symbols.KindTrait:
		sb.WriteString(fmt.Sprintf("```php\ntrait %s\n```\n", sym.FQN))
	}
	if sym.DocComment != "" {
		if doc := parser.ParseDocBlock(sym.DocComment); doc != nil && doc.Summary != "" {
			sb.WriteString("\n" + doc.Summary + "\n")
		}
	}
	return sb.String()
}

func fmtParams(params []symbols.ParamInfo) string {
	var parts []string
	for _, p := range params {
		s := ""
		if p.Type != "" {
			s += p.Type + " "
		}
		if p.IsVariadic {
			s += "..."
		}
		if p.IsReference {
			s += "&"
		}
		s += p.Name
		parts = append(parts, s)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func getWordAt(source string, pos protocol.Position) string {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character > len(line) {
		return ""
	}
	start := pos.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	if start > 0 && line[start-1] == '$' {
		start--
	}
	end := pos.Character
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	if start >= end {
		return ""
	}
	return line[start:end]
}

func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '\\'
}
