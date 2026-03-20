package hover

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
	"github.com/open-southeners/php-lsp/internal/types"
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
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return nil
	}
	line := lines[pos.Line]

	// Check for array key hover: $config['key'] or $config['db']['host'] — cursor on a key
	if ctx, ok := getArrayKeyContext(line, pos.Character); ok {
		return p.hoverArrayKey(source, pos, ctx)
	}

	word := getWordAt(source, pos)
	if word == "" {
		return nil
	}

	// No hover card for PHP primitive types (except self/static/parent which resolve to classes)
	if symbols.IsPHPBuiltinType(word) && word != "self" && word != "static" && word != "parent" {
		return nil
	}

	file := parser.ParseFile(source)

	// Handle $variable hover
	if strings.HasPrefix(word, "$") {
		return p.hoverVariable(file, source, pos, word)
	}

	// Handle self/static/parent keywords — resolve to enclosing class
	if word == "self" || word == "static" || word == "parent" {
		if file != nil {
			var classFQN string
			if word == "parent" {
				enclosing := p.findEnclosingClass(file, pos)
				if enclosing != "" {
					chain := p.index.GetInheritanceChain(enclosing)
					if len(chain) > 0 {
						classFQN = chain[0]
					}
				}
			} else {
				classFQN = p.findEnclosingClass(file, pos)
			}
			if classFQN != "" {
				if sym := p.index.Lookup(classFQN); sym != nil {
					content := p.formatHover(sym)
					if content != "" {
						return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
					}
				}
			}
		}
	}

	// Find the start position of the word on the line
	wordStart := pos.Character
	for wordStart > 0 && isWordChar(line[wordStart-1]) {
		wordStart--
	}

	// Check for -> or :: access context
	if classFQN := p.resolveAccessChain(line, wordStart, file, source, pos); classFQN != "" {
		if sym := p.findMember(classFQN, word); sym != nil {
			content := p.formatHover(sym)
			if content != "" {
				return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
			}
		}
	}

	// Resolve the word via use statements
	if file != nil {
		for _, u := range file.Uses {
			if u.Alias == word {
				if sym := p.index.Lookup(u.FullName); sym != nil {
					content := p.formatHover(sym)
					if content != "" {
						return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
					}
				}
			}
		}
		// Try resolving as a class name in the current namespace context
		fqn := p.resolveClassName(word, file)
		if fqn != word {
			if sym := p.index.Lookup(fqn); sym != nil {
				content := p.formatHover(sym)
				if content != "" {
					return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
				}
			}
		}
	}

	// If the word contains backslashes (FQN like Monolog\Logger), try direct FQN lookup
	if strings.Contains(word, "\\") {
		if sym := p.index.Lookup(word); sym != nil {
			content := p.formatHover(sym)
			if content != "" {
				return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
			}
		}
	}

	// Fallback: lookup by short name
	lookupName := word
	if idx := strings.LastIndex(word, "\\"); idx >= 0 {
		lookupName = word[idx+1:]
	}
	syms := p.index.LookupByName(lookupName)
	if len(syms) == 0 {
		return nil
	}

	// We're in standalone context (no -> or :: before the word).
	// Rank candidates: prefer functions/classes/enums/interfaces over methods/properties,
	// and prefer exact case matches over case-insensitive ones.
	best := symbols.PickBestStandalone(syms, word)
	if best == nil {
		return nil
	}
	content := p.formatHover(best)
	if content == "" {
		return nil
	}
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
}

// resolveAccessChain walks left through a chain of -> and :: accesses and
// returns the FQN of the class that owns the member at wordStart.
// E.g. for "$this->logger->info()", if wordStart points at "info",
// it resolves $this -> Service, finds property "logger" -> Logger type, returns Logger FQN.
func (p *Provider) resolveAccessChain(line string, wordStart int, file *parser.FileNode, source string, pos protocol.Position) string {
	i := wordStart

	// Skip whitespace before the word
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return ""
	}

	// Check for -> or ::
	var op string
	if line[i-2] == '-' && line[i-1] == '>' {
		op = "->"
		i -= 2
	} else if line[i-2] == ':' && line[i-1] == ':' {
		op = "::"
		i -= 2
	} else {
		return ""
	}
	_ = op

	// Skip whitespace before operator
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	// Skip past a method call's closing paren: $foo->bar()->baz
	if i > 0 && line[i-1] == ')' {
		depth := 1
		i--
		for i > 0 && depth > 0 {
			i--
			if line[i] == ')' {
				depth++
			} else if line[i] == '(' {
				depth--
			}
		}
		// Now i points at '(', skip whitespace before it
		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	// Extract the target word
	end := i
	for i > 0 && isWordChar(line[i-1]) {
		i--
	}
	// Include $ for variables
	if i > 0 && line[i-1] == '$' {
		i--
	}
	if i >= end {
		return ""
	}
	target := line[i:end]

	if file == nil {
		return ""
	}

	// Resolve the target to a class FQN
	switch target {
	case "$this", "self", "static":
		return p.findEnclosingClass(file, pos)
	case "parent":
		classFQN := p.findEnclosingClass(file, pos)
		if classFQN == "" {
			return ""
		}
		chain := p.index.GetInheritanceChain(classFQN)
		if len(chain) > 0 {
			return chain[0]
		}
		return ""
	}

	if strings.HasPrefix(target, "$") {
		// Variable: resolve its type
		typeFQN := p.resolveVariableType(target, file, source, pos)
		return typeFQN
	}

	// Bare word target: could be a class name (for static access)
	// or a chained property/method (e.g. the "logger" in "$this->logger->info")
	// First, try as a class name
	if fqn := p.resolveClassName(target, file); fqn != "" {
		if p.index.Lookup(fqn) != nil {
			return fqn
		}
	}

	// Otherwise, recursively resolve the chain to get the owner class,
	// then find the target as a member and return its type.
	ownerFQN := p.resolveAccessChain(line, i, file, source, pos)
	if ownerFQN == "" {
		return ""
	}
	member := p.findMember(ownerFQN, target)
	if member == nil {
		return ""
	}
	return p.memberType(member, file)
}

// memberType returns the resolved FQN of the type that a member evaluates to.
func (p *Provider) memberType(member *symbols.Symbol, file *parser.FileNode) string {
	var typeName string
	switch member.Kind {
	case symbols.KindProperty:
		typeName = member.Type
	case symbols.KindMethod:
		typeName = member.ReturnType
	default:
		return ""
	}
	if typeName == "" || typeName == "void" || typeName == "mixed" {
		return ""
	}
	// Handle self/static return types
	if typeName == "self" || typeName == "static" {
		return member.ParentFQN
	}
	return p.resolveClassName(typeName, file)
}

// findEnclosingClass returns the FQN of the class that contains the given position.
func (p *Provider) findEnclosingClass(file *parser.FileNode, pos protocol.Position) string {
	for _, cls := range file.Classes {
		if pos.Line >= cls.StartLine {
			fqn := cls.FullName
			if fqn == "" {
				fqn = buildFQN(file.Namespace, cls.Name)
			}
			return fqn
		}
	}
	return ""
}

// resolveVariableType tries to infer the type of a variable from context.
func (p *Provider) resolveVariableType(varName string, file *parser.FileNode, source string, pos protocol.Position) string {
	// 1. Check method/function parameter type hints in the enclosing scope
	enclosingMethod := p.findEnclosingMethod(file, pos)
	if enclosingMethod != nil {
		for _, param := range enclosingMethod.Params {
			if param.Name == varName {
				return p.resolveClassName(param.Type.Name, file)
			}
		}
	}

	// 2. Check class properties for $this->prop patterns
	// (handled at chain level, but also check promoted constructor params)
	for _, cls := range file.Classes {
		for _, prop := range cls.Properties {
			if "$"+prop.Name == varName && prop.Type.Name != "" {
				return p.resolveClassName(prop.Type.Name, file)
			}
		}
	}

	lines := strings.Split(source, "\n")
	bare := strings.TrimPrefix(varName, "$")
	varPrefix := "$" + bare

	// 3. Look for `$var = new ClassName(...)` and literal assignments
	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, varPrefix) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(varPrefix):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rhs := strings.TrimSpace(rest[1:])
		// $var = new ClassName(...)
		if strings.HasPrefix(rhs, "new ") {
			className := strings.TrimSpace(rhs[4:])
			if idx := strings.IndexByte(className, '('); idx >= 0 {
				className = className[:idx]
			}
			className = strings.TrimSuffix(className, ";")
			className = strings.TrimSpace(className)
			if className != "" {
				return p.resolveClassName(className, file)
			}
		}
		// $var = expr; — infer literal type
		rhs = strings.TrimSuffix(rhs, ";")
		rhs = strings.TrimSpace(rhs)
		if t := inferLiteralType(rhs); t != "" {
			return t
		}
	}

	// 4. Check @var annotations: /** @var ClassName $var */
	for i := pos.Line; i >= 0 && i >= pos.Line-5; i-- {
		if i >= len(lines) {
			continue
		}
		line := lines[i]
		varIdx := strings.Index(line, "@var ")
		if varIdx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[varIdx+5:])
		fields := strings.Fields(rest)
		if len(fields) >= 2 && fields[1] == varPrefix {
			return p.resolveClassName(fields[0], file)
		}
	}

	return ""
}

// inferLiteralType returns the PHP type for a literal expression value.
func inferLiteralType(expr string) string {
	if expr == "" {
		return ""
	}
	// String literals: '', "", heredoc
	if (expr[0] == '\'' || expr[0] == '"') {
		return "string"
	}
	// Boolean literals
	lower := strings.ToLower(expr)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	// Null
	if lower == "null" {
		return "null"
	}
	// Array literals: [], array()
	if expr[0] == '[' || strings.HasPrefix(lower, "array(") {
		return "array"
	}
	// Numeric: int or float
	if expr[0] >= '0' && expr[0] <= '9' || expr[0] == '-' {
		if strings.ContainsAny(expr, ".eE") {
			return "float"
		}
		return "int"
	}
	return ""
}

// findEnclosingMethod returns the method node that contains the given position.
func (p *Provider) findEnclosingMethod(file *parser.FileNode, pos protocol.Position) *parser.MethodNode {
	if file == nil {
		return nil
	}
	for ci := len(file.Classes) - 1; ci >= 0; ci-- {
		cls := file.Classes[ci]
		if pos.Line < cls.StartLine {
			continue
		}
		var best *parser.MethodNode
		for mi := range cls.Methods {
			m := &cls.Methods[mi]
			if pos.Line >= m.StartLine {
				if best == nil || m.StartLine > best.StartLine {
					best = m
				}
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}

// resolveClassName resolves a short or partially-qualified class name to a FQN
// using use statements and the file's namespace.
func (p *Provider) resolveClassName(name string, file *parser.FileNode) string {
	if name == "" {
		return ""
	}
	if file == nil {
		return name
	}
	// Already fully qualified
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
	// Strip nullable
	if strings.HasPrefix(name, "?") {
		name = name[1:]
	}

	parts := strings.SplitN(name, "\\", 2)
	for _, u := range file.Uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	if file.Namespace != "" {
		fqn := file.Namespace + "\\" + name
		if p.index.Lookup(fqn) != nil {
			return fqn
		}
	}
	return name
}

// findMember looks up a member (method, property, constant) on a class,
// traversing the inheritance chain and traits.
func (p *Provider) findMember(classFQN, memberName string) *symbols.Symbol {
	members := p.index.GetClassMembers(classFQN)
	for _, m := range members {
		if m.Name == memberName || m.Name == "$"+memberName {
			return m
		}
	}
	return nil
}

func (p *Provider) hoverVariable(file *parser.FileNode, source string, pos protocol.Position, varName string) *protocol.Hover {
	if file == nil {
		return nil
	}

	// Try to resolve the variable type
	typeName := p.resolveVariableType(varName, file, source, pos)
	if typeName != "" {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**%s**\n", varName))
		if sym := p.index.Lookup(typeName); sym != nil {
			if sym.DocComment != "" {
				if doc := parser.ParseDocBlock(sym.DocComment); doc != nil && doc.Summary != "" {
					sb.WriteString("\n" + doc.Summary + "\n")
				}
			}
		}
		sb.WriteString(fmt.Sprintf("\n```php\n%s %s\n```\n", shortName(typeName), varName))
		p.appendContainerBinding(&sb, typeName)
		return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: sb.String()}}
	}

	// Fallback: search all method params in file
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			for _, param := range m.Params {
				if param.Name == varName {
					t := param.Type.Name
					if t == "" {
						t = "mixed"
					}
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("**%s**\n", varName))
					sb.WriteString(fmt.Sprintf("\n```php\n%s %s\n```\n", t, varName))
					p.appendContainerBinding(&sb, t)
					return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: sb.String()}}
				}
			}
		}
	}
	return nil
}

func (p *Provider) formatHover(sym *symbols.Symbol) string {
	var sb strings.Builder

	// === 1. Header: bold FQN ===
	sb.WriteString("**" + sym.FQN + "**\n")

	// === 2. Summary (from docblock, shown right after header) ===
	doc := p.getEffectiveDocBlock(sym)
	if doc != nil && doc.Summary != "" {
		sb.WriteString("\n" + doc.Summary + "\n")
	}

	// === 3. Code block: PHP declaration ===
	sb.WriteString("\n```php\n")
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
	sb.WriteString("\n```\n")

	// === 4. Context line (parent class, override info) ===
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

	// === 5. Extended docblock details ===
	if doc != nil {
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
	}

	// === 6. Container binding ===
	switch sym.Kind {
	case symbols.KindInterface, symbols.KindClass:
		p.appendContainerBinding(&sb, sym.FQN)
	}

	// === 7. PHP Manual link ===
	if url := phpManualURL(sym); url != "" {
		sb.WriteString(fmt.Sprintf("\n[PHP Manual](%s)\n", url))
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

// appendContainerBinding adds container binding info if available.
func (p *Provider) appendContainerBinding(sb *strings.Builder, fqn string) {
	if binding := p.container.ResolveDependency(fqn); binding != nil {
		sb.WriteString(fmt.Sprintf("\n---\n**Container Binding**\n- Concrete: `%s`\n- Singleton: %v\n", binding.Concrete, binding.Singleton))
	}
}

// appendMethodOrigin detects if a method overrides a parent method or implements an interface method.
func (p *Provider) appendMethodOrigin(sb *strings.Builder, sym *symbols.Symbol) {
	// Check interfaces
	ifaces := p.index.GetImplementedInterfaces(sym.ParentFQN)
	for _, ifaceFQN := range ifaces {
		ifaceSym := p.index.Lookup(ifaceFQN)
		if ifaceSym == nil {
			continue
		}
		for _, child := range ifaceSym.Children {
			if child.Kind == symbols.KindMethod && child.Name == sym.Name {
				sb.WriteString(fmt.Sprintf("\nImplements `%s::%s`\n", ifaceFQN, sym.Name))
				return
			}
		}
	}
	// Check parent chain
	chain := p.index.GetInheritanceChain(sym.ParentFQN)
	for _, parentFQN := range chain {
		parentSym := p.index.Lookup(parentFQN)
		if parentSym == nil {
			continue
		}
		for _, child := range parentSym.Children {
			if child.Kind == symbols.KindMethod && child.Name == sym.Name {
				sb.WriteString(fmt.Sprintf("\nOverrides `%s::%s`\n", parentFQN, sym.Name))
				return
			}
		}
	}
	// Default: show defined in
	sb.WriteString(fmt.Sprintf("\nDefined in `%s`\n", sym.ParentFQN))
}

// getEffectiveDocBlock returns the docblock for a symbol, falling back to parent/interface docs.
func (p *Provider) getEffectiveDocBlock(sym *symbols.Symbol) *parser.DocBlock {
	if sym.DocComment != "" {
		if doc := parser.ParseDocBlock(sym.DocComment); doc != nil {
			return doc
		}
	}
	// For methods, try inheriting from parent or interface
	if sym.Kind == symbols.KindMethod && sym.ParentFQN != "" {
		// Check interfaces
		ifaces := p.index.GetImplementedInterfaces(sym.ParentFQN)
		for _, ifaceFQN := range ifaces {
			ifaceSym := p.index.Lookup(ifaceFQN)
			if ifaceSym == nil {
				continue
			}
			for _, child := range ifaceSym.Children {
				if child.Kind == symbols.KindMethod && child.Name == sym.Name && child.DocComment != "" {
					if doc := parser.ParseDocBlock(child.DocComment); doc != nil {
						return doc
					}
				}
			}
		}
		// Check parent chain
		chain := p.index.GetInheritanceChain(sym.ParentFQN)
		for _, parentFQN := range chain {
			parentSym := p.index.Lookup(parentFQN)
			if parentSym == nil {
				continue
			}
			for _, child := range parentSym.Children {
				if child.Kind == symbols.KindMethod && child.Name == sym.Name && child.DocComment != "" {
					if doc := parser.ParseDocBlock(child.DocComment); doc != nil {
						return doc
					}
				}
			}
		}
	}
	return nil
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

func buildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
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
	// If cursor is on '$', include it and scan forward from the next char
	ch := pos.Character
	if ch < len(line) && line[ch] == '$' {
		start := ch
		end := ch + 1
		for end < len(line) && isWordChar(line[end]) {
			end++
		}
		if end > start+1 {
			return line[start:end]
		}
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

// hoverArrayKeyContext holds the parsed context for hovering an array key.
type hoverArrayKeyContext struct {
	VarName    string   // e.g. "$config"
	AccessKeys []string // preceding completed keys, e.g. ["database"] for $config['database']['host']
	CurrentKey string   // the key the cursor is on, e.g. "host"
}

// getArrayKeyContext checks if the cursor is on a string key inside an array access
// expression like $config['key'] or $config['db']['host'].
func getArrayKeyContext(line string, character int) (*hoverArrayKeyContext, bool) {
	if character >= len(line) || len(line) < 4 {
		return nil, false
	}

	// Step 1: find the string literal boundaries around the cursor
	left := character
	for left > 0 && line[left] != '\'' && line[left] != '"' {
		if line[left] == ']' || line[left] == '[' {
			return nil, false
		}
		left--
	}
	if left <= 0 || (line[left] != '\'' && line[left] != '"') {
		return nil, false
	}
	openQuote := line[left]

	right := character
	if right < len(line) && line[right] == openQuote {
		right++
	} else {
		for right < len(line) && line[right] != openQuote {
			right++
		}
		if right >= len(line) {
			return nil, false
		}
		right++
	}

	if left+1 >= right-1 {
		return nil, false
	}
	currentKey := line[left+1 : right-1]
	if currentKey == "" {
		return nil, false
	}

	// Step 2: expect [ before the opening quote
	i := left - 1
	for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
		i--
	}
	if i < 0 || line[i] != '[' {
		return nil, false
	}
	i--

	// Step 3: collect preceding completed access keys backward: ]['key2']['key1']
	var accessKeys []string
	for i >= 0 {
		for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
			i--
		}
		if i < 0 || line[i] != ']' {
			break
		}
		i-- // skip ]

		if i < 0 || (line[i] != '\'' && line[i] != '"') {
			break
		}
		closeQ := line[i]
		i--

		keyEnd := i + 1
		for i >= 0 && line[i] != closeQ {
			i--
		}
		if i < 0 {
			break
		}
		key := line[i+1 : keyEnd]
		i-- // skip opening quote

		for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
			i--
		}
		if i < 0 || line[i] != '[' {
			break
		}
		i-- // skip [
		accessKeys = append([]string{key}, accessKeys...)
	}

	// Step 4: skip whitespace and extract $variable
	for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil, false
	}

	end := i + 1
	for i >= 0 && isWordChar(line[i]) {
		i--
	}
	if i >= 0 && line[i] == '$' {
		return &hoverArrayKeyContext{
			VarName:    line[i:end],
			AccessKeys: accessKeys,
			CurrentKey: currentKey,
		}, true
	}
	return nil, false
}

// getArrayKeyAt is a convenience wrapper for backward compatibility in tests.
func getArrayKeyAt(line string, character int) (varName, key string, ok bool) {
	ctx, found := getArrayKeyContext(line, character)
	if !found {
		return "", "", false
	}
	return ctx.VarName, ctx.CurrentKey, true
}

// hoverArrayKey provides hover information for an array key, including nested access.
func (p *Provider) hoverArrayKey(source string, pos protocol.Position, ctx *hoverArrayKeyContext) *protocol.Hover {
	// Resolve top-level shape
	fields := p.resolveArrayShape(source, pos, ctx.VarName)
	if len(fields) == 0 {
		fields = scanArrayKeysForHover(source, pos, ctx.VarName)
	}

	// Drill into nested shapes via accessKeys
	for _, accessKey := range ctx.AccessKeys {
		var nestedType string
		for _, f := range fields {
			if f.Key == accessKey {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		fields = types.ParseArrayShape(nestedType)
		if len(fields) == 0 {
			return nil
		}
	}

	// Find the current key in the (possibly nested) shape
	for _, f := range fields {
		if f.Key == ctx.CurrentKey {
			var sb strings.Builder
			sb.WriteString("```php\n")
			if f.Type != "" {
				sb.WriteString(fmt.Sprintf("(array key) %s $%s", f.Type, ctx.CurrentKey))
			} else {
				sb.WriteString(fmt.Sprintf("(array key) $%s", ctx.CurrentKey))
			}
			if f.Optional {
				sb.WriteString(" (optional)")
			}
			sb.WriteString("\n```")
			return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: sb.String()}}
		}
	}

	return nil
}

// resolveArrayShape resolves shape fields for a variable from docblock annotations.
func (p *Provider) resolveArrayShape(source string, pos protocol.Position, varName string) []types.ShapeField {
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}
	bare := strings.TrimPrefix(varName, "$")

	// Check method parameters
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if pos.Line >= m.StartLine {
				if m.DocComment != "" {
					doc := parser.ParseDocBlock(m.DocComment)
					if doc != nil {
						for _, param := range doc.Params {
							if param.Name == "$"+bare {
								if fields := types.ParseArrayShape(param.Type); len(fields) > 0 {
									return fields
								}
							}
						}
					}
				}
				for _, param := range m.Params {
					if param.Name == varName {
						if fields := types.ParseArrayShape(param.Type.Name); len(fields) > 0 {
							return fields
						}
					}
				}
			}
		}
	}
	for _, fn := range file.Functions {
		if pos.Line >= fn.StartLine {
			if fn.DocComment != "" {
				doc := parser.ParseDocBlock(fn.DocComment)
				if doc != nil {
					for _, param := range doc.Params {
						if param.Name == "$"+bare {
							if fields := types.ParseArrayShape(param.Type); len(fields) > 0 {
								return fields
							}
						}
					}
				}
			}
		}
	}

	// Check @var annotations
	lines := strings.Split(source, "\n")
	for i := pos.Line; i >= 0 && i >= pos.Line-10; i-- {
		if i >= len(lines) {
			continue
		}
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "@var") && strings.Contains(line, varName) {
			varIdx := strings.Index(line, "@var ")
			if varIdx >= 0 {
				rest := strings.TrimSpace(line[varIdx+5:])
				typeStr, _ := types.ExtractDocTypeString(rest)
				if fields := types.ParseArrayShape(typeStr); len(fields) > 0 {
					return fields
				}
			}
		}
	}

	return nil
}

// scanArrayKeysForHover extracts keys from literal array assignments,
// preserving nested structure for drilling.
func scanArrayKeysForHover(source string, pos protocol.Position, varName string) []types.ShapeField {
	lines := strings.Split(source, "\n")
	var keys []types.ShapeField
	seen := make(map[string]bool)

	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, varName) {
			rest := strings.TrimSpace(trimmed[len(varName):])
			if strings.HasPrefix(rest, "=") {
				rhs := strings.TrimSpace(rest[1:])
				if strings.HasPrefix(rhs, "[") || strings.HasPrefix(strings.ToLower(rhs), "array(") {
					arrayText := collectArrayLiteralText(lines, i)
					return parseLiteralArrayToShape(arrayText)
				}
			}
		}
		// Incremental: $var['key'] = ... (must have = after ])
		if strings.HasPrefix(trimmed, varName+"[") {
			after := trimmed[len(varName)+1:]
			if len(after) > 2 && (after[0] == '\'' || after[0] == '"') {
				q := after[0]
				endQ := strings.IndexByte(after[1:], q)
				if endQ > 0 {
					afterClose := strings.TrimSpace(after[endQ+2:])
					if !strings.HasPrefix(afterClose, "]") {
						continue
					}
					afterBracket := strings.TrimSpace(afterClose[1:])
					if !strings.HasPrefix(afterBracket, "=") {
						continue
					}
					key := after[1 : endQ+1]
					if !seen[key] {
						seen[key] = true
						keys = append(keys, types.ShapeField{Key: key})
					}
				}
			}
		}
	}
	return keys
}

// collectArrayLiteralText collects the full text of an array literal starting from startLine.
func collectArrayLiteralText(lines []string, startLine int) string {
	var sb strings.Builder
	depth := 0
	started := false
	for i := startLine; i < len(lines) && i < startLine+100; i++ {
		line := lines[i]
		for j := 0; j < len(line); j++ {
			ch := line[j]
			if ch == '[' {
				depth++
				started = true
			} else if ch == ']' {
				depth--
			}
			if started {
				sb.WriteByte(ch)
			}
			if started && depth == 0 {
				return sb.String()
			}
		}
		if started {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// parseLiteralArrayToShape parses a PHP array literal into ShapeFields with nested structure.
func parseLiteralArrayToShape(arrayText string) []types.ShapeField {
	arrayText = strings.TrimSpace(arrayText)
	if len(arrayText) < 2 || arrayText[0] != '[' || arrayText[len(arrayText)-1] != ']' {
		return nil
	}
	return parseLiteralEntries(arrayText[1 : len(arrayText)-1])
}

func parseLiteralEntries(content string) []types.ShapeField {
	var fields []types.ShapeField
	depth := 0
	inString := byte(0)
	start := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString != 0 {
			if ch == inString && (i == 0 || content[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ',':
			if depth == 0 {
				if f := parseLiteralEntry(content[start:i]); f != nil {
					fields = append(fields, *f)
				}
				start = i + 1
			}
		}
	}
	if start < len(content) {
		if f := parseLiteralEntry(content[start:]); f != nil {
			fields = append(fields, *f)
		}
	}
	return fields
}

func parseLiteralEntry(entry string) *types.ShapeField {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}
	arrowIdx := -1
	depth := 0
	inString := byte(0)
	for i := 0; i < len(entry)-1; i++ {
		ch := entry[i]
		if inString != 0 {
			if ch == inString && (i == 0 || entry[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '=':
			if depth == 0 && i+1 < len(entry) && entry[i+1] == '>' {
				arrowIdx = i
				goto found
			}
		}
	}
found:
	if arrowIdx < 0 {
		return nil
	}
	keyPart := strings.TrimSpace(entry[:arrowIdx])
	valuePart := strings.TrimSpace(entry[arrowIdx+2:])
	if len(keyPart) >= 2 && (keyPart[0] == '\'' || keyPart[0] == '"') && keyPart[len(keyPart)-1] == keyPart[0] {
		keyPart = keyPart[1 : len(keyPart)-1]
	} else {
		return nil
	}
	valueType := inferNestedType(valuePart)
	return &types.ShapeField{Key: keyPart, Type: valueType}
}

func inferNestedType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ",")
	value = strings.TrimSpace(value)
	if value == "" {
		return "mixed"
	}
	if strings.HasPrefix(value, "[") {
		nested := parseLiteralArrayToShape(value)
		if len(nested) > 0 {
			var parts []string
			for _, f := range nested {
				if f.Key != "" {
					parts = append(parts, f.Key+": "+f.Type)
				}
			}
			if len(parts) > 0 {
				return "array{" + strings.Join(parts, ", ") + "}"
			}
		}
		return "array"
	}
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
		return "string"
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	if lower == "null" {
		return "null"
	}
	if len(value) > 0 && (value[0] >= '0' && value[0] <= '9' || value[0] == '-') {
		if strings.ContainsAny(value, ".eE") {
			return "float"
		}
		return "int"
	}
	return "mixed"
}
