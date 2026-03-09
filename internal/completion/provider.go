package completion

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

func (p *Provider) GetCompletions(uri, source string, pos protocol.Position) []protocol.CompletionItem {
	line := getLineAt(source, pos.Line)
	prefix := ""
	if pos.Character <= len(line) {
		prefix = line[:pos.Character]
	}
	trimmed := strings.TrimSpace(prefix)

	if strings.HasSuffix(trimmed, "->") || strings.HasSuffix(trimmed, "?->") {
		return p.completeMemberAccess(uri, source, pos, prefix)
	}
	if strings.HasSuffix(trimmed, "::") {
		return p.completeStaticAccess(prefix)
	}
	if strings.HasSuffix(trimmed, "|>") {
		return p.completePipe()
	}
	if strings.Contains(trimmed, "#[") && !strings.Contains(trimmed, "]") {
		return p.completeAttribute()
	}
	words := strings.Fields(trimmed)
	if len(words) >= 1 && (words[len(words)-1] == "new" || (len(words) >= 2 && words[len(words)-2] == "new")) {
		return p.completeNew(prefix)
	}
	if len(words) >= 1 && words[0] == "use" {
		return p.completeUse(prefix)
	}
	if strings.Contains(trimmed, "app(") || strings.Contains(trimmed, "$container->get(") {
		return p.completeContainerResolve()
	}
	return p.completeGlobal(prefix)
}

func (p *Provider) completeMemberAccess(uri, source string, pos protocol.Position, prefix string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	varName := extractVariableBefore(prefix, "->")
	typeName := p.resolveVariableType(source, varName)
	if typeName == "" {
		return items
	}
	if binding := p.container.ResolveDependency(typeName); binding != nil {
		typeName = binding.Concrete
	}
	for _, m := range p.index.GetClassMembers(typeName) {
		if m.IsStatic || m.Visibility == "private" {
			continue
		}
		item := protocol.CompletionItem{Label: m.Name, Detail: formatDetail(m)}
		switch m.Kind {
		case symbols.KindMethod:
			item.Kind = protocol.CompletionItemKindMethod
			item.InsertText = m.Name + "($0)"
			item.InsertTextFormat = 2
		case symbols.KindProperty:
			item.Label = strings.TrimPrefix(m.Name, "$")
			item.Kind = protocol.CompletionItemKindProperty
		case symbols.KindConstant:
			item.Kind = protocol.CompletionItemKindConstant
		}
		items = append(items, item)
	}
	return items
}

func (p *Provider) completeStaticAccess(prefix string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	className := extractClassBefore(prefix, "::")
	for _, sym := range p.index.LookupByName(className) {
		for _, m := range p.index.GetClassMembers(sym.FQN) {
			if !m.IsStatic && m.Kind != symbols.KindConstant && m.Kind != symbols.KindEnumCase {
				continue
			}
			item := protocol.CompletionItem{Label: m.Name, Detail: formatDetail(m)}
			switch m.Kind {
			case symbols.KindMethod:
				item.Kind = protocol.CompletionItemKindMethod
			case symbols.KindConstant:
				item.Kind = protocol.CompletionItemKindConstant
			case symbols.KindEnumCase:
				item.Kind = protocol.CompletionItemKindEnumMember
			}
			items = append(items, item)
		}
	}
	return items
}

func (p *Provider) completeNew(prefix string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	words := strings.Fields(prefix)
	search := ""
	if len(words) > 1 {
		search = words[len(words)-1]
		if search == "new" {
			search = ""
		}
	}
	for _, sym := range p.index.SearchByPrefix(search) {
		if sym.Kind != symbols.KindClass {
			continue
		}
		items = append(items, protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindClass, Detail: sym.FQN, InsertText: sym.Name + "($0)", InsertTextFormat: 2})
	}
	return items
}

func (p *Provider) completeUse(prefix string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	parts := strings.Fields(prefix)
	ns := ""
	if len(parts) > 1 {
		ns = parts[len(parts)-1]
	}
	for _, sym := range p.index.SearchByPrefix(ns) {
		if sym.Kind == symbols.KindMethod || sym.Kind == symbols.KindProperty {
			continue
		}
		items = append(items, protocol.CompletionItem{Label: sym.FQN, Kind: symKind(sym.Kind), Detail: sym.Name})
	}
	return items
}

func (p *Provider) completePipe() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, sym := range p.index.SearchByPrefix("") {
		if sym.Kind == symbols.KindFunction && len(sym.Params) > 0 {
			items = append(items, protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindFunction, Detail: fmtSig(sym)})
		}
	}
	return items
}

func (p *Provider) completeAttribute() []protocol.CompletionItem {
	attrs := [][2]string{
		{"Override", "PHP 8.3"}, {"Deprecated", "PHP 8.4"}, {"SensitiveParameter", "Sensitive in stack traces"}, {"AllowDynamicProperties", "Allow dynamic props"},
	}
	if p.framework == "symfony" {
		attrs = append(attrs, [2]string{"Route", "Define route"}, [2]string{"AsController", "Register controller"}, [2]string{"AsCommand", "Register command"}, [2]string{"Autowire", "Autowire service"}, [2]string{"AsEventListener", "Event listener"}, [2]string{"AsMessageHandler", "Message handler"})
	}
	var items []protocol.CompletionItem
	for _, a := range attrs {
		items = append(items, protocol.CompletionItem{Label: a[0], Kind: protocol.CompletionItemKindClass, Detail: a[1]})
	}
	return items
}

func (p *Provider) completeContainerResolve() []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for abstract, binding := range p.container.GetBindings() {
		d := fmt.Sprintf("-> %s", binding.Concrete)
		if binding.Singleton {
			d += " (singleton)"
		}
		items = append(items, protocol.CompletionItem{Label: abstract, Kind: protocol.CompletionItemKindClass, Detail: d})
	}
	return items
}

func (p *Provider) completeGlobal(prefix string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	words := strings.Fields(strings.TrimSpace(prefix))
	search := ""
	if len(words) > 0 {
		search = words[len(words)-1]
	}
	for _, kw := range []string{"abstract", "class", "const", "enum", "extends", "final", "fn", "for", "foreach", "function", "if", "implements", "interface", "match", "namespace", "new", "private", "protected", "public", "readonly", "return", "static", "switch", "throw", "trait", "try", "use", "while", "yield"} {
		if search == "" || strings.HasPrefix(kw, strings.ToLower(search)) {
			items = append(items, protocol.CompletionItem{Label: kw, Kind: protocol.CompletionItemKindKeyword, SortText: "2" + kw})
		}
	}
	if search != "" {
		for _, sym := range p.index.SearchByPrefix(search) {
			item := protocol.CompletionItem{Label: sym.Name, Kind: symKind(sym.Kind), Detail: sym.FQN}
			if sym.Kind == symbols.KindFunction {
				item.InsertText = sym.Name + "($0)"
				item.InsertTextFormat = 2
			}
			items = append(items, item)
		}
	}
	if p.framework == "laravel" {
		for _, h := range [][3]string{
			{"app", "Resolve from container", "app($0)"}, {"config", "Get/set config", "config('$0')"}, {"env", "Get env var", "env('$0')"},
			{"route", "URL for route", "route('$0')"}, {"view", "Create view", "view('$0')"}, {"redirect", "Redirect", "redirect('$0')"},
			{"collect", "Create collection", "collect($0)"}, {"dd", "Dump and die", "dd($0)"}, {"now", "Current time", "now()"},
		} {
			if search == "" || strings.HasPrefix(h[0], strings.ToLower(search)) {
				items = append(items, protocol.CompletionItem{Label: h[0], Kind: protocol.CompletionItemKindFunction, Detail: h[1], InsertText: h[2], InsertTextFormat: 2, SortText: "0" + h[0]})
			}
		}
	}
	return items
}

func (p *Provider) resolveVariableType(source, varName string) string {
	if varName == "$this" {
		file := parser.ParseFile(source)
		if file != nil && len(file.Classes) > 0 {
			return file.Namespace + "\\" + file.Classes[0].Name
		}
		return ""
	}
	file := parser.ParseFile(source)
	if file == nil {
		return ""
	}
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			for _, param := range m.Params {
				if param.Name == varName && param.Type.Name != "" {
					for _, u := range file.Uses {
						if u.Alias == param.Type.Name {
							return u.FullName
						}
					}
					if file.Namespace != "" {
						return file.Namespace + "\\" + param.Type.Name
					}
					return param.Type.Name
				}
			}
		}
	}
	return ""
}

func getLineAt(source string, line int) string {
	lines := strings.Split(source, "\n")
	if line >= 0 && line < len(lines) {
		return lines[line]
	}
	return ""
}

func extractVariableBefore(prefix, op string) string {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return ""
	}
	before := strings.TrimSpace(prefix[:idx])
	for i := len(before) - 1; i >= 0; i-- {
		if before[i] == '$' {
			return before[i:]
		}
		if !(before[i] >= 'a' && before[i] <= 'z') && !(before[i] >= 'A' && before[i] <= 'Z') && !(before[i] >= '0' && before[i] <= '9') && before[i] != '_' {
			break
		}
	}
	return ""
}

func extractClassBefore(prefix, op string) string {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return ""
	}
	before := strings.TrimSpace(prefix[:idx])
	for i := len(before) - 1; i >= 0; i-- {
		ch := before[i]
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '_' && ch != '\\' {
			return before[i+1:]
		}
	}
	return before
}

func symKind(kind symbols.SymbolKind) protocol.CompletionItemKind {
	switch kind {
	case symbols.KindClass:
		return protocol.CompletionItemKindClass
	case symbols.KindInterface:
		return protocol.CompletionItemKindInterface
	case symbols.KindEnum:
		return protocol.CompletionItemKindEnum
	case symbols.KindFunction:
		return protocol.CompletionItemKindFunction
	case symbols.KindMethod:
		return protocol.CompletionItemKindMethod
	case symbols.KindProperty:
		return protocol.CompletionItemKindProperty
	case symbols.KindConstant:
		return protocol.CompletionItemKindConstant
	case symbols.KindEnumCase:
		return protocol.CompletionItemKindEnumMember
	default:
		return protocol.CompletionItemKindText
	}
}

func formatDetail(sym *symbols.Symbol) string {
	if sym.Kind == symbols.KindMethod {
		return fmtSig(sym)
	}
	if sym.Kind == symbols.KindProperty {
		if sym.Type != "" {
			return sym.Type
		}
		return "mixed"
	}
	return sym.FQN
}

func fmtSig(sym *symbols.Symbol) string {
	var params []string
	for _, p := range sym.Params {
		s := ""
		if p.Type != "" {
			s += p.Type + " "
		}
		if p.IsVariadic {
			s += "..."
		}
		s += p.Name
		params = append(params, s)
	}
	ret := sym.ReturnType
	if ret == "" {
		ret = "mixed"
	}
	return fmt.Sprintf("(%s): %s", strings.Join(params, ", "), ret)
}
