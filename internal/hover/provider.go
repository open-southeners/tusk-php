package hover

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/models"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/phparray"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/resolve"
	"github.com/open-southeners/php-lsp/internal/symbols"
	"github.com/open-southeners/php-lsp/internal/types"
)

type Provider struct {
	index         *symbols.Index
	container     *container.ContainerAnalyzer
	resolver      *resolve.Resolver
	framework     string
	arrayResolver *models.FrameworkArrayResolver
}

func NewProvider(index *symbols.Index, ca *container.ContainerAnalyzer, framework string) *Provider {
	return &Provider{index: index, container: ca, resolver: resolve.NewResolver(index), framework: framework}
}

// SetArrayResolver sets the framework array resolver for config hover.
func (p *Provider) SetArrayResolver(resolver *models.FrameworkArrayResolver) {
	p.arrayResolver = resolver
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

	// Check for config key hover: config('database.connections.mysql')
	if hover := p.hoverConfigKey(line, pos.Character); hover != nil {
		return hover
	}

	word := resolve.GetWordAt(source, pos)
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
				enclosing := resolve.FindEnclosingClass(file, pos)
				if enclosing != "" {
					chain := p.index.GetInheritanceChain(enclosing)
					if len(chain) > 0 {
						classFQN = chain[0]
					}
				}
			} else {
				classFQN = resolve.FindEnclosingClass(file, pos)
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
	for wordStart > 0 && resolve.IsWordChar(line[wordStart-1]) {
		wordStart--
	}

	// Check for -> or :: access context
	if classFQN := p.resolveAccessChain(line, wordStart, file, source, pos); classFQN != "" {
		if sym := p.resolver.FindMember(classFQN, word); sym != nil {
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
		fqn := p.resolver.ResolveClassName(word, file)
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
	for i > 0 && resolve.IsWordChar(line[i-1]) {
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
		return resolve.FindEnclosingClass(file, pos)
	case "parent":
		classFQN := resolve.FindEnclosingClass(file, pos)
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
		typeFQN := p.resolver.ResolveVariableType(target, file, source, pos)
		return typeFQN
	}

	// Bare word target: could be a class name (for static access)
	// or a chained property/method (e.g. the "logger" in "$this->logger->info")
	// First, try as a class name
	if fqn := p.resolver.ResolveClassName(target, file); fqn != "" {
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
	member := p.resolver.FindMember(ownerFQN, target)
	if member == nil {
		return ""
	}
	return p.resolver.MemberType(member, file)
}

func (p *Provider) hoverVariable(file *parser.FileNode, source string, pos protocol.Position, varName string) *protocol.Hover {
	if file == nil {
		return nil
	}

	// Try to resolve the variable type
	typeName := p.resolver.ResolveVariableType(varName, file, source, pos)
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
	for i >= 0 && resolve.IsWordChar(line[i]) {
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

// hoverConfigKey provides hover for config key strings inside config('key.path').
func (p *Provider) hoverConfigKey(line string, character int) *protocol.Hover {
	if p.arrayResolver == nil {
		return nil
	}

	// Find config( before the cursor on this line
	configIdx := strings.LastIndex(line[:min(character+1, len(line))], "config(")
	if configIdx < 0 {
		return nil
	}
	after := line[configIdx+len("config("):]

	// Must have an opening quote
	if len(after) == 0 || (after[0] != '\'' && after[0] != '"') {
		return nil
	}
	openQuote := after[0]
	after = after[1:]

	// Find closing quote
	closeIdx := strings.IndexByte(after, openQuote)
	if closeIdx < 0 {
		return nil
	}

	// Check cursor is inside the string
	stringStart := configIdx + len("config(") + 1 // after open quote
	stringEnd := stringStart + closeIdx
	if character < stringStart || character > stringEnd {
		return nil
	}

	fullKey := after[:closeIdx]
	if fullKey == "" {
		return nil
	}

	// Resolve the config value at this path
	parts := strings.Split(fullKey, ".")
	configFile := parts[0]
	keys := p.arrayResolver.ParseConfigFile(configFile)
	if keys == nil {
		return nil
	}

	// For top-level key only (e.g. config('database')), show the file's shape
	if len(parts) == 1 {
		var keyNames []string
		for _, k := range keys {
			if k.Key != "" {
				keyNames = append(keyNames, k.Key)
			}
			if len(keyNames) >= 6 {
				keyNames = append(keyNames, "...")
				break
			}
		}
		return &protocol.Hover{Contents: protocol.MarkupContent{
			Kind:  "markdown",
			Value: fmt.Sprintf("```php\n(config) %s: array{%s}\n```", fullKey, strings.Join(keyNames, ", ")),
		}}
	}

	// Drill through dot segments to find the target
	var targetField *types.ShapeField
	for _, segment := range parts[1:] {
		found := false
		for i := range keys {
			if keys[i].Key == segment {
				targetField = &keys[i]
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		// If there are more segments, drill into the nested shape
		nested := types.ParseArrayShape(targetField.Type)
		if nested != nil {
			keys = nested
		}
	}

	if targetField == nil {
		return nil
	}

	// Show the resolved type
	detail := targetField.Type
	if strings.HasPrefix(detail, "array{") {
		// Summarize nested keys
		inner := types.ParseArrayShape(detail)
		if len(inner) > 0 {
			var names []string
			for _, f := range inner {
				if f.Key != "" {
					names = append(names, f.Key)
				}
				if len(names) >= 6 {
					names = append(names, "...")
					break
				}
			}
			detail = "array{" + strings.Join(names, ", ") + "}"
		}
	}

	return &protocol.Hover{Contents: protocol.MarkupContent{
		Kind:  "markdown",
		Value: fmt.Sprintf("```php\n(config) %s: %s\n```", fullKey, detail),
	}}
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
					arrayText := phparray.CollectArrayLiteral(lines, i)
					return phparray.ParseLiteralToShape(arrayText)
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
