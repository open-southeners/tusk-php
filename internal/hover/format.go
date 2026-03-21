package hover

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

func (p *Provider) formatHoverDeclaration(sym *symbols.Symbol) string {
	var sb strings.Builder

	switch sym.Kind {
	case symbols.KindClass:
		if sym.IsFinal {
			sb.WriteString("final ")
		}
		if sym.IsAbstract {
			sb.WriteString("abstract ")
		}
		if sym.IsReadonly {
			sb.WriteString("readonly ")
		}
		sb.WriteString("class " + sym.Name)
		if sym.Extends != "" {
			sb.WriteString(" extends " + shortName(sym.Extends))
		}
		if len(sym.Implements) > 0 {
			sb.WriteString(" implements " + joinShortNames(sym.Implements))
		}
	case symbols.KindInterface:
		sb.WriteString("interface " + sym.Name)
		if len(sym.Implements) > 0 {
			sb.WriteString(" extends " + joinShortNames(sym.Implements))
		}
	case symbols.KindMethod:
		if sym.IsVirtual {
			sb.WriteString("(magic) ")
		}
		vis := sym.Visibility
		if vis == "" {
			vis = "public"
		}
		if sym.IsAbstract {
			sb.WriteString("abstract ")
		}
		if sym.IsFinal {
			sb.WriteString("final ")
		}
		sb.WriteString(vis)
		if sym.IsStatic {
			sb.WriteString(" static")
		}
		sb.WriteString(fmt.Sprintf(" function %s%s", sym.Name, fmtParams(sym.Params)))
		if sym.ReturnType != "" {
			sb.WriteString(": " + sym.ReturnType)
		}
	case symbols.KindFunction:
		sb.WriteString(fmt.Sprintf("function %s%s", sym.Name, fmtParams(sym.Params)))
		if sym.ReturnType != "" {
			sb.WriteString(": " + sym.ReturnType)
		}
	case symbols.KindProperty:
		if sym.IsVirtual {
			sb.WriteString("(magic) ")
		}
		vis := sym.Visibility
		if vis == "" {
			vis = "public"
		}
		t := sym.Type
		if t == "" {
			t = "mixed"
		}
		sb.WriteString(vis)
		if sym.IsStatic {
			sb.WriteString(" static")
		}
		if sym.IsReadonly {
			sb.WriteString(" readonly")
		}
		propName := sym.Name
		if !strings.HasPrefix(propName, "$") {
			propName = "$" + propName
		}
		sb.WriteString(fmt.Sprintf(" %s %s", t, propName))
	case symbols.KindEnum:
		sb.WriteString("enum " + sym.Name)
		if sym.BackedType != "" {
			sb.WriteString(": " + sym.BackedType)
		}
		if len(sym.Implements) > 0 {
			sb.WriteString(" implements " + joinShortNames(sym.Implements))
		}
	case symbols.KindEnumCase:
		sb.WriteString("case " + sym.Name)
		if sym.Value != "" {
			sb.WriteString(" = " + sym.Value)
		}
	case symbols.KindConstant:
		sb.WriteString("const " + sym.Name)
		if sym.Value != "" {
			sb.WriteString(" = " + sym.Value)
		}
	case symbols.KindTrait:
		sb.WriteString("trait " + sym.Name)
	}

	return sb.String()
}

func (p *Provider) formatHoverContext(sym *symbols.Symbol) string {
	var sb strings.Builder

	switch sym.Kind {
	case symbols.KindMethod:
		if sym.ParentFQN != "" {
			p.appendMethodOrigin(&sb, sym)
		}
	case symbols.KindProperty, symbols.KindConstant, symbols.KindEnumCase:
		if sym.ParentFQN != "" {
			sb.WriteString(fmt.Sprintf("\nDefined in `%s`\n", sym.ParentFQN))
		}
	case symbols.KindClass:
		if impls := p.index.GetImplementors(sym.FQN); len(impls) > 0 {
			sb.WriteString("\n**Implemented by:** ")
			names := make([]string, len(impls))
			for i, impl := range impls {
				names[i] = "`" + impl.FQN + "`"
			}
			sb.WriteString(strings.Join(names, ", ") + "\n")
		}
	case symbols.KindInterface:
		if impls := p.index.GetImplementors(sym.FQN); len(impls) > 0 {
			sb.WriteString("\n**Implementations:** ")
			names := make([]string, len(impls))
			for i, impl := range impls {
				names[i] = "`" + impl.FQN + "`"
			}
			sb.WriteString(strings.Join(names, ", ") + "\n")
		}
	}

	return sb.String()
}

func (p *Provider) formatDocBlockDetails(doc *parser.DocBlock) string {
	if doc == nil {
		return ""
	}

	var sb strings.Builder

	if doc.Deprecated {
		msg := doc.DeprecatedMsg
		if msg == "" {
			msg = "This symbol is deprecated."
		}
		sb.WriteString(fmt.Sprintf("\n**Deprecated:** %s\n", msg))
	}
	if len(doc.Params) > 0 {
		sb.WriteString("\n**Params**\n")
		for _, param := range doc.Params {
			line := "- "
			if param.Name != "" {
				line += "`" + param.Name + "` "
			}
			if param.Type != "" {
				line += "`" + param.Type + "`"
			}
			if param.Description != "" {
				line += " — " + param.Description
			}
			sb.WriteString(line + "\n")
		}
	}
	if doc.Return.Type != "" {
		ret := fmt.Sprintf("\n**Returns** `%s`", doc.Return.Type)
		if doc.Return.Description != "" {
			ret += " — " + doc.Return.Description
		}
		sb.WriteString(ret + "\n")
	}
	if len(doc.Throws) > 0 {
		sb.WriteString("\n**Throws**\n")
		for _, th := range doc.Throws {
			line := "- `" + th.Type + "`"
			if th.Description != "" {
				line += " — " + th.Description
			}
			sb.WriteString(line + "\n")
		}
	}
	for _, tagName := range []string{"template", "mixin", "see", "property", "property-read", "property-write", "method"} {
		if vals, ok := doc.Tags[tagName]; ok && len(vals) > 0 {
			label := "@" + tagName
			for _, v := range vals {
				sb.WriteString(fmt.Sprintf("\n`%s %s`\n", label, v))
			}
		}
	}

	return sb.String()
}

func shortName(fqn string) string {
	if i := strings.LastIndex(fqn, "\\"); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

func joinShortNames(fqns []string) string {
	names := make([]string, len(fqns))
	for i, fqn := range fqns {
		names[i] = shortName(fqn)
	}
	return strings.Join(names, ", ")
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
		if p.DefaultValue != "" {
			s += " = " + p.DefaultValue
		}
		parts = append(parts, s)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
