package completion

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

// SetArrayResolver sets the framework array resolver for config/request/model key completion.
func (p *Provider) SetArrayResolver(resolver *models.FrameworkArrayResolver) {
	p.arrayResolver = resolver
}

func (p *Provider) GetCompletions(uri, source string, pos protocol.Position) []protocol.CompletionItem {
	line := resolve.GetLineAt(source, pos.Line)
	prefix := ""
	if pos.Character <= len(line) {
		prefix = line[:pos.Character]
	}
	trimmed := strings.TrimSpace(prefix)

	if strings.HasSuffix(trimmed, "->") || strings.HasSuffix(trimmed, "?->") {
		return p.completeMemberAccess(uri, source, pos, prefix)
	}
	// Container argument context takes priority over :: detection
	// (e.g. app(Request::class) should not trigger static access)
	if _, _, isContainer := extractContainerArgContext(trimmed); !isContainer {
		if strings.HasSuffix(trimmed, "::") {
			return p.completeStaticAccess(source, prefix, pos)
		}
		// Typing after -> or :: (e.g. "$foo->ba" or "Foo::cr")
		if memberCtx, filter := detectMemberContext(trimmed); memberCtx != "" {
			if strings.Contains(memberCtx, "::") {
				items := p.completeStaticAccess(source, memberCtx, pos)
				return filterByPrefix(items, filter)
			}
			items := p.completeMemberAccess(uri, source, pos, memberCtx)
			return filterByPrefix(items, filter)
		}
	}
	if strings.HasSuffix(trimmed, "|>") {
		currentNS := extractNamespace(source)
		return p.completePipe(currentNS)
	}
	if strings.Contains(trimmed, "#[") && !strings.Contains(trimmed, "]") {
		return p.completeAttribute()
	}
	words := strings.Fields(trimmed)
	if len(words) >= 1 && (words[len(words)-1] == "new" || (len(words) >= 2 && words[len(words)-2] == "new")) {
		currentNS := extractNamespace(source)
		return p.completeNew(prefix, currentNS)
	}
	if len(words) >= 1 && words[0] == "use" {
		currentNS := extractNamespace(source)
		return p.completeUse(prefix, currentNS)
	}
	// Array key completion: $var['partial or $var['key1']['partial (nested)
	if ctx := parseArrayKeyContext(prefix); ctx != nil {
		return p.completeArrayKeys(source, pos, ctx)
	}
	// Config result array access: config('database')['connections']['
	if ctx := parseConfigResultArrayContext(prefix); ctx != nil {
		return p.completeConfigResultKeys(ctx)
	}
	// Config key completion: config('database.|') with dot-notation navigation
	if configPath, partial, quote, ok := extractConfigArgContext(trimmed); ok {
		return p.completeConfigKeys(configPath, partial, quote)
	}
	if filter, quoteCtx, ok := extractContainerArgContext(trimmed); ok {
		currentNS := extractNamespace(source)
		return p.completeContainerResolve(source, filter, currentNS, quoteCtx)
	}
	currentNS := extractNamespace(source)
	// Detect namespace path typing (contains \)
	search := extractLastWord(prefix)
	if strings.Contains(search, "\\") {
		return p.completeNamespacePath(search, currentNS)
	}
	items := p.completeGlobal(prefix, currentNS)
	// Add $this if inside a class method
	lastWord := ""
	if w := strings.Fields(strings.TrimSpace(prefix)); len(w) > 0 {
		lastWord = w[len(w)-1]
	}
	if lastWord == "" || strings.HasPrefix("$this", strings.ToLower(lastWord)) {
		if file := parser.ParseFile(source); file != nil {
			if resolve.FindEnclosingClass(file, pos) != "" {
				items = append(items, protocol.CompletionItem{
					Label:    "$this",
					Kind:     protocol.CompletionItemKindVariable,
					Detail:   "Current object instance",
					SortText: "0$this",
				})
			}
		}
	}
	return items
}

func (p *Provider) completeMemberAccess(uri, source string, pos protocol.Position, prefix string) []protocol.CompletionItem {
	typeName := p.resolveChainType(source, prefix, "->", pos)
	if typeName == "" {
		return nil
	}
	if p.container != nil {
		if binding := p.container.ResolveDependency(typeName); binding != nil {
			typeName = binding.Concrete
		}
	}
	var items []protocol.CompletionItem
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
		}
		items = append(items, item)
	}
	return items
}

func (p *Provider) completeStaticAccess(source, prefix string, pos protocol.Position) []protocol.CompletionItem {
	typeName := p.resolveChainType(source, prefix, "::", pos)
	if typeName == "" {
		return nil
	}
	var items []protocol.CompletionItem
	for _, m := range p.index.GetClassMembers(typeName) {
		if !m.IsStatic && m.Kind != symbols.KindConstant && m.Kind != symbols.KindEnumCase {
			continue
		}
		item := protocol.CompletionItem{Label: m.Name, Detail: formatDetail(m)}
		switch m.Kind {
		case symbols.KindMethod:
			item.Kind = protocol.CompletionItemKindMethod
			item.InsertText = m.Name + "($0)"
			item.InsertTextFormat = 2
		case symbols.KindConstant:
			item.Kind = protocol.CompletionItemKindConstant
		case symbols.KindEnumCase:
			item.Kind = protocol.CompletionItemKindEnumMember
		case symbols.KindProperty:
			if m.IsStatic {
				item.Kind = protocol.CompletionItemKindProperty
			} else {
				continue
			}
		}
		items = append(items, item)
	}
	return items
}

// resolveChainType resolves the class FQN from the expression before op (-> or ::).
// Handles: $var->, $this->, self::, static::, parent::, ClassName::,
// $var::, new ClassName()->, (new ClassName)->, and method chains.
// Also handles container calls: app('request')->, app(Request::class)->, resolve(...)->
func (p *Provider) resolveChainType(source, prefix, op string, pos protocol.Position) string {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return ""
	}
	before := strings.TrimSpace(prefix[:idx])

	// Check for container call pattern: app('key'), app(Class::class), resolve('key')
	// Also blocks config('key')-> which returns mixed, not a class
	if op == "->" {
		if concrete := p.resolveContainerCallType(before, source); concrete != "" {
			if concrete == "-" {
				return "" // signal: known call returning mixed, stop resolution
			}
			return concrete
		}
	}

	// Check for "new ClassName(...)" pattern before ->
	if op == "->" {
		if newClass := extractNewClass(before); newClass != "" {
			return p.resolveClassNameFromSource(newClass, source)
		}
	}

	// Extract the target token (variable or class name)
	target := extractTrailingToken(before)
	if target == "" {
		return ""
	}

	file := parser.ParseFile(source)

	switch target {
	case "$this", "self", "static":
		if file != nil {
			return resolve.FindEnclosingClass(file, pos)
		}
		return ""
	case "parent":
		if file != nil {
			classFQN := resolve.FindEnclosingClass(file, pos)
			if classFQN != "" {
				chain := p.index.GetInheritanceChain(classFQN)
				if len(chain) > 0 {
					return chain[0]
				}
			}
		}
		return ""
	}

	// $variable-> or $variable::
	if strings.HasPrefix(target, "$") {
		return p.resolver.ResolveVariableType(target, file, source, pos)
	}

	// ClassName:: or ClassName->  (static access or after new)
	return p.resolveClassNameFromSource(target, source)
}

// resolveContainerCallType checks if the expression is a container resolution call
// like app('request'), app(Request::class), resolve('cache'), $container->get('log')
// and returns the concrete FQN from the container bindings.
// config() with no args returns Repository; config('key') returns mixed (no resolution).
func (p *Provider) resolveContainerCallType(expr, source string) string {
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
		arg = p.resolveClassNameFromSource(className, source)
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

// resolveClassNameFromSource resolves a short or FQN class name using
// the source file's use statements and namespace.
func (p *Provider) resolveClassNameFromSource(name, source string) string {
	if name == "" {
		return ""
	}
	// Already fully qualified
	if strings.HasPrefix(name, "\\") {
		fqn := strings.TrimPrefix(name, "\\")
		if p.index.Lookup(fqn) != nil {
			return fqn
		}
		return fqn
	}

	file := parser.ParseFile(source)
	if file == nil {
		// Try direct lookup by name
		syms := p.index.LookupByName(name)
		if best := symbols.PickBestStandalone(syms, name); best != nil {
			return best.FQN
		}
		return name
	}

	// Check use statements
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range file.Uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	// Try in current namespace
	if file.Namespace != "" {
		fqn := file.Namespace + "\\" + name
		if p.index.Lookup(fqn) != nil {
			return fqn
		}
	}
	// Try as global
	if p.index.Lookup(name) != nil {
		return name
	}
	// Fallback: search by short name
	syms := p.index.LookupByName(name)
	if best := symbols.PickBestStandalone(syms, name); best != nil {
		return best.FQN
	}
	return name
}

// extractTrailingToken extracts the last variable or identifier token
// from a string, handling method call chains by skipping parenthesized args.
func extractTrailingToken(s string) string {
	i := len(s)
	// Skip trailing whitespace
	for i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
		i--
	}
	if i == 0 {
		return ""
	}
	// Skip closing paren (method chain: $foo->bar()-> )
	if s[i-1] == ')' {
		depth := 1
		i--
		for i > 0 && depth > 0 {
			i--
			if s[i] == ')' {
				depth++
			} else if s[i] == '(' {
				depth--
			}
		}
		for i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
			i--
		}
	}
	// Extract the word
	end := i
	for i > 0 && resolve.IsWordChar(s[i-1]) {
		i--
	}
	if i > 0 && s[i-1] == '$' {
		i--
	}
	if i >= end {
		return ""
	}
	return s[i:end]
}

// extractNewClass extracts the class name from patterns like:
// "new ClassName()", "(new ClassName())", "new ClassName", "(new ClassName)"
func extractNewClass(s string) string {
	t := strings.TrimSpace(s)

	// Strip wrapping parens: "(new ClassName())" → "new ClassName()"
	for strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		inner := t[1 : len(t)-1]
		// Only strip if the parens are balanced wrapping parens (not constructor args)
		if parenBalanced(inner) {
			t = strings.TrimSpace(inner)
		} else {
			break
		}
	}

	// Strip constructor args: "new ClassName(...)" → "new ClassName"
	if strings.HasSuffix(t, ")") {
		depth := 1
		i := len(t) - 2
		for i >= 0 && depth > 0 {
			if t[i] == ')' {
				depth++
			} else if t[i] == '(' {
				depth--
			}
			i--
		}
		t = strings.TrimSpace(t[:i+1])
	}

	// Extract class name after "new"
	if idx := strings.LastIndex(t, "new "); idx >= 0 {
		className := strings.TrimSpace(t[idx+4:])
		if className != "" && !strings.ContainsAny(className, " \t(") {
			return className
		}
	}
	return ""
}

func parenBalanced(s string) bool {
	depth := 0
	for _, ch := range s {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func (p *Provider) completeNew(prefix, currentNS string) []protocol.CompletionItem {
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
		items = append(items, protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindClass, Detail: sym.FQN, InsertText: sym.Name + "($0)", InsertTextFormat: 2, SortText: sortPriority(sym, currentNS)})
	}
	return items
}

func (p *Provider) completeUse(prefix, currentNS string) []protocol.CompletionItem {
	parts := strings.Fields(prefix)
	search := ""
	if len(parts) > 1 {
		search = parts[len(parts)-1]
	}
	// If typing a namespace path, use namespace-aware completion
	if strings.Contains(search, "\\") {
		return p.completeNamespacePath(search, currentNS)
	}
	// Otherwise show all top-level namespace segments + global symbols
	var items []protocol.CompletionItem
	_, nsSegs := p.index.SearchByFQNPrefix("")
	for _, seg := range nsSegs {
		if search == "" || strings.HasPrefix(strings.ToLower(seg), strings.ToLower(search)) {
			items = append(items, protocol.CompletionItem{
				Label:    seg,
				Kind:     protocol.CompletionItemKindModule,
				Detail:   seg,
				SortText: "0" + seg,
			})
		}
	}
	if search != "" {
		for _, sym := range p.index.SearchByPrefix(search) {
			if sym.Kind == symbols.KindMethod || sym.Kind == symbols.KindProperty {
				continue
			}
			items = append(items, protocol.CompletionItem{Label: sym.FQN, Kind: symKind(sym.Kind), Detail: sym.Name, SortText: sortPriority(sym, currentNS)})
		}
	}
	return items
}

func (p *Provider) completePipe(currentNS string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, sym := range p.index.SearchByPrefix("") {
		if sym.Kind == symbols.KindFunction && len(sym.Params) > 0 {
			items = append(items, protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindFunction, Detail: fmtSig(sym), SortText: sortPriority(sym, currentNS)})
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

// arrayKeyContext holds the parsed context for array key completion.
type arrayKeyContext struct {
	VarName    string   // e.g. "$config"
	AccessKeys []string // e.g. ["database"] for $config['database']['
	Partial    string   // partial key typed so far
	Quote      string   // quote character used ("'" or "\"" or "")
}

// extractArrayKeyContext detects if the cursor is inside an array key access:
// $var['partial, $var["partial, or nested: $var['key1']['partial
// Returns the context and true if matched.
func extractArrayKeyContext(prefix string) (varName, partial, quote string, ok bool) {
	ctx := parseArrayKeyContext(prefix)
	if ctx == nil {
		return
	}
	return ctx.VarName, ctx.Partial, ctx.Quote, true
}

// parseArrayKeyContext parses the full array access chain from the prefix.
// Returns nil if the cursor is not inside an array key context.
func parseArrayKeyContext(prefix string) *arrayKeyContext {
	i := len(prefix) - 1

	// Step 1: extract the partial key at cursor (text after opening quote)
	partialStart := i + 1
	for i >= 0 && prefix[i] != '\'' && prefix[i] != '"' && prefix[i] != '[' {
		i--
	}
	if i < 0 {
		return nil
	}

	var partial, quote string
	if prefix[i] == '\'' || prefix[i] == '"' {
		quote = string(prefix[i])
		partial = prefix[i+1 : partialStart]
		i--
	} else if prefix[i] == '[' {
		partial = ""
	}

	// Step 2: expect [
	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil
	}
	if prefix[i] == '[' {
		i--
	} else if quote == "" {
		// [ was already consumed in step 1
	} else {
		return nil
	}

	// Step 3: collect preceding completed access keys: ]['key2']['key1']
	// Working backward through the chain
	var accessKeys []string
	for i >= 0 {
		// Skip whitespace
		for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
			i--
		}
		if i < 0 {
			break
		}

		// Check for ] (end of a completed access)
		if prefix[i] != ']' {
			break
		}
		i-- // skip ]

		// Expect closing quote
		if i < 0 || (prefix[i] != '\'' && prefix[i] != '"') {
			break
		}
		closeQuote := prefix[i]
		i--

		// Scan backward for opening quote
		keyEnd := i + 1
		for i >= 0 && prefix[i] != closeQuote {
			i--
		}
		if i < 0 {
			break
		}
		key := prefix[i+1 : keyEnd]
		i-- // skip opening quote

		// Expect [
		for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
			i--
		}
		if i < 0 || prefix[i] != '[' {
			break
		}
		i-- // skip [

		// Prepend key (we're going backward)
		accessKeys = append([]string{key}, accessKeys...)
	}

	// Step 4: skip whitespace before the first [
	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil
	}

	// Step 5: extract $variable name
	end := i + 1
	for i >= 0 && resolve.IsWordChar(prefix[i]) {
		i--
	}
	if i >= 0 && prefix[i] == '$' {
		return &arrayKeyContext{
			VarName:    prefix[i:end],
			AccessKeys: accessKeys,
			Partial:    partial,
			Quote:      quote,
		}
	}
	return nil
}

// completeArrayKeys provides completion items for array keys.
// It resolves the variable's type from docblocks/params, drills into nested
// shapes via accessKeys, then falls back to scanning literal assignments.
func (p *Provider) completeArrayKeys(source string, pos protocol.Position, ctx *arrayKeyContext) []protocol.CompletionItem {
	var keys []types.ShapeField

	// 1. Try to resolve from docblock shapes (param type hints, @var annotations)
	keys = p.resolveArrayKeysFromType(source, pos, ctx.VarName)

	// 2. Fall back to literal assignment scanning
	if len(keys) == 0 {
		keys = scanLiteralArrayKeys(source, pos, ctx.VarName)
	}

	// 3. Drill into nested shapes via accessKeys
	for _, accessKey := range ctx.AccessKeys {
		var nestedType string
		for _, f := range keys {
			if f.Key == accessKey {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil // key not found in shape
		}
		keys = types.ParseArrayShape(nestedType)
		if len(keys) == 0 {
			return nil // not a nested shape
		}
	}

	if len(keys) == 0 {
		return nil
	}

	// Default quote
	q := "'"
	if ctx.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(ctx.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue // skip positional fields
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		detail := k.Type
		if k.Optional {
			detail += " (optional)"
		}

		insertText := k.Key
		if ctx.Quote == "" {
			// No quote typed yet — wrap fully
			insertText = q + k.Key + q
		}
		// When quote is already typed, insert just the key (editor auto-pairs closing quote)

		sortText := "0" + k.Key
		if k.Optional {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}

// resolveArrayKeysFromType resolves array shape keys from the variable's type
// as declared in docblocks or parameter type hints.
func (p *Provider) resolveArrayKeysFromType(source string, pos protocol.Position, varName string) []types.ShapeField {
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}

	bare := strings.TrimPrefix(varName, "$")

	// Check method/function parameters
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if pos.Line >= m.StartLine {
				// Check params with docblock shapes
				if m.DocComment != "" {
					if fields := extractShapeFromDocParams(m.DocComment, bare); len(fields) > 0 {
						return fields
					}
				}
				// Check param type hints (for shapes declared in type hint directly)
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
				if fields := extractShapeFromDocParams(fn.DocComment, bare); len(fields) > 0 {
					return fields
				}
			}
			for _, param := range fn.Params {
				if param.Name == varName {
					if fields := types.ParseArrayShape(param.Type.Name); len(fields) > 0 {
						return fields
					}
				}
			}
		}
	}

	// Check @var annotations above the variable assignment
	lines := strings.Split(source, "\n")
	for i := pos.Line; i >= 0 && i >= pos.Line-10; i-- {
		if i >= len(lines) {
			continue
		}
		line := strings.TrimSpace(lines[i])
		// Look for @var array{...} $varName
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

	// Check if variable comes from a function/method return with a shape
	// Scan for $varName = someFunc() or $varName = $this->someMethod()
	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, varName) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(varName):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rhs := strings.TrimSpace(rest[1:])

		// Try framework-specific resolver first (config(), $request->validated(), etc.)
		if p.arrayResolver != nil {
			if fields := p.arrayResolver.ResolveCallReturnKeys(rhs, source); len(fields) > 0 {
				return fields
			}
		}

		// Try to resolve the method/function return type from docblocks
		if retType := p.resolveCallReturnType(rhs, source); retType != "" {
			if fields := types.ParseArrayShape(retType); len(fields) > 0 {
				return fields
			}
		}

		// Check framework resolver for method calls: $this->method() or ClassName::method()
		if p.arrayResolver != nil {
			if classFQN, methodName := parseMethodCall(rhs, file); classFQN != "" && methodName != "" {
				if fields := p.arrayResolver.ResolveMethodReturnKeys(classFQN, methodName); len(fields) > 0 {
					return fields
				}
			}
		}
		break
	}

	return nil
}

// parseMethodCall extracts class FQN and method name from "$this->method()" or "$var->method()".
func parseMethodCall(expr string, file *parser.FileNode) (classFQN, methodName string) {
	expr = strings.TrimSuffix(strings.TrimSpace(expr), ";")
	if parenIdx := strings.Index(expr, "("); parenIdx > 0 {
		expr = expr[:parenIdx]
	}
	if !strings.Contains(expr, "->") {
		return
	}
	parts := strings.Split(expr, "->")
	methodName = parts[len(parts)-1]

	target := strings.TrimSpace(parts[0])
	if target == "$this" && file != nil && len(file.Classes) > 0 {
		cls := file.Classes[0]
		if cls.FullName != "" {
			classFQN = cls.FullName
		} else if file.Namespace != "" {
			classFQN = file.Namespace + "\\" + cls.Name
		} else {
			classFQN = cls.Name
		}
	}
	return
}

// extractShapeFromDocParams parses a docblock to find @param with array shape
// for the given parameter name (without $).
func extractShapeFromDocParams(docComment, paramBare string) []types.ShapeField {
	doc := parser.ParseDocBlock(docComment)
	if doc == nil {
		return nil
	}
	target := "$" + paramBare
	for _, param := range doc.Params {
		if param.Name == target {
			return types.ParseArrayShape(param.Type)
		}
	}
	// Also check @var tags
	if vars, ok := doc.Tags["var"]; ok {
		for _, v := range vars {
			typeStr, rest := types.ExtractDocTypeString(v)
			if strings.Contains(rest, target) {
				return types.ParseArrayShape(typeStr)
			}
		}
	}
	return nil
}

// resolveCallReturnType tries to resolve the return type of a simple call expression.
// Handles: funcName(...), $this->method(...), ClassName::method(...)
func (p *Provider) resolveCallReturnType(expr, source string) string {
	expr = strings.TrimSuffix(strings.TrimSpace(expr), ";")
	// Strip arguments: funcName(args) → funcName
	if parenIdx := strings.Index(expr, "("); parenIdx > 0 {
		expr = expr[:parenIdx]
	}
	expr = strings.TrimSpace(expr)

	// $this->method
	if strings.Contains(expr, "->") {
		parts := strings.Split(expr, "->")
		methodName := parts[len(parts)-1]
		// Resolve $this type
		file := parser.ParseFile(source)
		if file != nil && len(file.Classes) > 0 {
			cls := file.Classes[0]
			classFQN := cls.FullName
			if classFQN == "" && file.Namespace != "" {
				classFQN = file.Namespace + "\\" + cls.Name
			} else if classFQN == "" {
				classFQN = cls.Name
			}
			// Check method return type and docblock
			for _, member := range p.index.GetClassMembers(classFQN) {
				if member.Name == methodName && member.Kind == symbols.KindMethod {
					// Check docblock for shape return type
					if member.DocComment != "" {
						doc := parser.ParseDocBlock(member.DocComment)
						if doc != nil && doc.Return.Type != "" {
							return doc.Return.Type
						}
					}
					return member.ReturnType
				}
			}
		}
	}

	// Simple function name
	funcName := expr
	syms := p.index.LookupByName(funcName)
	for _, sym := range syms {
		if sym.Kind == symbols.KindFunction || sym.Kind == symbols.KindMethod {
			if sym.DocComment != "" {
				doc := parser.ParseDocBlock(sym.DocComment)
				if doc != nil && doc.Return.Type != "" {
					return doc.Return.Type
				}
			}
			return sym.ReturnType
		}
	}
	return ""
}

// scanLiteralArrayKeys scans the source for literal array assignments to the
// given variable and extracts string keys.
func scanLiteralArrayKeys(source string, pos protocol.Position, varName string) []types.ShapeField {
	lines := strings.Split(source, "\n")
	var keys []types.ShapeField
	seen := make(map[string]bool)

	// Scan backward for $varName = [...] or $varName = array(...)
	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])

		// Pattern: $varName = [...]
		if strings.HasPrefix(trimmed, varName) {
			rest := strings.TrimSpace(trimmed[len(varName):])
			if strings.HasPrefix(rest, "=") {
				rhs := strings.TrimSpace(rest[1:])
				if strings.HasPrefix(rhs, "[") || strings.HasPrefix(strings.ToLower(rhs), "array(") {
					// Collect the full array literal text, then parse it
					arrayText := phparray.CollectArrayLiteral(lines, i)
					parsed := phparray.ParseLiteralToShape(arrayText)
					if len(parsed) > 0 {
						keys = parsed
						break
					}
					// Empty literal (e.g. $arr = []) — continue to find incremental assignments
				}
			}
		}

		// Pattern: $varName['key'] = ... (incremental building — must have ] then =)
		if k := extractIncrementalKey(trimmed, varName); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, types.ShapeField{Key: k})
		}
	}

	// Also scan forward for incremental assignments
	for i := pos.Line + 1; i < len(lines) && i <= pos.Line+50; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if k := extractIncrementalKey(trimmed, varName); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, types.ShapeField{Key: k})
		}
	}

	return keys
}

// extractIncrementalKey extracts the key from "$var['key'] = ..." patterns,
// verifying it's actually an assignment (has ] followed by =).
func extractIncrementalKey(trimmed, varName string) string {
	if !strings.HasPrefix(trimmed, varName+"[") {
		return ""
	}
	after := trimmed[len(varName)+1:]
	if len(after) < 3 || (after[0] != '\'' && after[0] != '"') {
		return ""
	}
	q := after[0]
	endQ := strings.IndexByte(after[1:], q)
	if endQ <= 0 {
		return ""
	}
	// Check that closing quote is followed by ] then =
	rest := strings.TrimSpace(after[endQ+2:])
	if !strings.HasPrefix(rest, "]") {
		return ""
	}
	afterBracket := strings.TrimSpace(rest[1:])
	if !strings.HasPrefix(afterBracket, "=") {
		return ""
	}
	return after[1 : endQ+1]
}

// configResultArrayContext holds parsed context for config('key')['nested']['
type configResultArrayContext struct {
	ConfigArg  string   // the config argument, e.g. "database"
	AccessKeys []string // completed bracket accesses after ), e.g. ["connections"]
	Partial    string   // partial key being typed
	Quote      string   // quote character
}

// parseConfigResultArrayContext detects config('key')['nested'][' patterns.
// Returns nil if the cursor is not in this context.
func parseConfigResultArrayContext(prefix string) *configResultArrayContext {
	// Scan backward from cursor to find the pattern:
	//   config('arg')['key1']['key2']['partial
	i := len(prefix) - 1

	// Step 1: extract partial key (same as array key context)
	partialStart := i + 1
	for i >= 0 && prefix[i] != '\'' && prefix[i] != '"' && prefix[i] != '[' {
		i--
	}
	if i < 0 {
		return nil
	}

	var partial, quote string
	if prefix[i] == '\'' || prefix[i] == '"' {
		quote = string(prefix[i])
		partial = prefix[i+1 : partialStart]
		i--
	} else if prefix[i] == '[' {
		partial = ""
	}

	// Step 2: expect [
	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil
	}
	if prefix[i] == '[' {
		i--
	} else if quote == "" {
		// already on [
	} else {
		return nil
	}

	// Step 3: collect completed ['key'] access chains
	var accessKeys []string
	for i >= 0 {
		for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
			i--
		}
		if i < 0 || prefix[i] != ']' {
			break
		}
		i-- // skip ]
		if i < 0 || (prefix[i] != '\'' && prefix[i] != '"') {
			break
		}
		closeQ := prefix[i]
		i--
		keyEnd := i + 1
		for i >= 0 && prefix[i] != closeQ {
			i--
		}
		if i < 0 {
			break
		}
		key := prefix[i+1 : keyEnd]
		i-- // skip opening quote
		for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
			i--
		}
		if i < 0 || prefix[i] != '[' {
			break
		}
		i-- // skip [
		accessKeys = append([]string{key}, accessKeys...)
	}

	// Step 4: expect ) from the config() call
	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	if i < 0 || prefix[i] != ')' {
		return nil
	}
	i-- // skip )

	// Step 5: find the matching ( and extract config argument
	depth := 1
	for i >= 0 && depth > 0 {
		if prefix[i] == ')' {
			depth++
		} else if prefix[i] == '(' {
			depth--
		}
		if depth > 0 {
			i--
		}
	}
	if i < 0 || prefix[i] != '(' {
		return nil
	}
	// Extract the string argument inside ()
	argContent := prefix[i+1:]
	// Find the closing ) we started from
	closeP := strings.Index(argContent, ")")
	if closeP < 0 {
		return nil
	}
	argContent = strings.TrimSpace(argContent[:closeP])
	// Strip quotes
	configArg := ""
	if len(argContent) >= 2 && (argContent[0] == '\'' || argContent[0] == '"') && argContent[len(argContent)-1] == argContent[0] {
		configArg = argContent[1 : len(argContent)-1]
	}
	if configArg == "" {
		return nil
	}

	i-- // move before (

	// Step 6: expect "config" before (
	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	end := i + 1
	for i >= 0 && resolve.IsWordChar(prefix[i]) {
		i--
	}
	funcName := prefix[i+1 : end]
	if funcName != "config" {
		return nil
	}

	return &configResultArrayContext{
		ConfigArg:  configArg,
		AccessKeys: accessKeys,
		Partial:    partial,
		Quote:      quote,
	}
}

// completeConfigResultKeys provides completion for config('key')['nested'][' patterns.
func (p *Provider) completeConfigResultKeys(ctx *configResultArrayContext) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	// Resolve the config value at the dot-notation path
	parts := strings.Split(ctx.ConfigArg, ".")
	configFile := parts[0]
	keys := p.arrayResolver.ParseConfigFile(configFile)
	if keys == nil {
		return nil
	}

	// Drill through dot-notation segments
	for _, segment := range parts[1:] {
		var nestedType string
		for _, f := range keys {
			if f.Key == segment {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		keys = types.ParseArrayShape(nestedType)
		if keys == nil {
			return nil
		}
	}

	// Drill through bracket access keys
	for _, accessKey := range ctx.AccessKeys {
		var nestedType string
		for _, f := range keys {
			if f.Key == accessKey {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		keys = types.ParseArrayShape(nestedType)
		if keys == nil {
			return nil
		}
	}

	if len(keys) == 0 {
		return nil
	}

	// Build completion items
	q := "'"
	if ctx.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(ctx.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		insertText := k.Key
		if ctx.Quote == "" {
			insertText = q + k.Key + q
		}
		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     k.Type,
			InsertText: insertText,
			SortText:   "0" + k.Key,
		})
	}
	return items
}

// extractConfigArgContext detects if the cursor is inside a config() call.
// Returns the dot-separated path typed so far, the partial segment being typed,
// the quote character, and true if matched.
//
// Examples:
//
//	config('             → configPath="", partial="", quote="'"
//	config('da           → configPath="", partial="da", quote="'"
//	config('database.    → configPath="database", partial="", quote="'"
//	config('database.co  → configPath="database", partial="co", quote="'"
//	config('database.connections.  → configPath="database.connections", partial="", quote="'"
func extractConfigArgContext(trimmed string) (configPath, partial, quote string, ok bool) {
	idx := strings.LastIndex(trimmed, "config(")
	if idx < 0 {
		return
	}
	after := trimmed[idx+len("config("):]
	// If there's a closing paren, cursor is past the argument
	if strings.Contains(after, ")") {
		return
	}
	// Detect quote
	if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
		quote = string(after[0])
		after = after[1:]
	} else if len(after) == 0 {
		// Just "config(" with nothing after — offer config files with quotes
		ok = true
		return
	}
	// Strip trailing closing quote if cursor is before it (editor auto-paired)
	// e.g. prefix="config('database.'" → after="database.'" → strip trailing '
	if quote != "" && len(after) > 0 && after[len(after)-1] == quote[0] {
		after = after[:len(after)-1]
	}

	// Split on dots: "database.connections.co" → path="database.connections", partial="co"
	lastDot := strings.LastIndex(after, ".")
	if lastDot >= 0 {
		configPath = after[:lastDot]
		partial = after[lastDot+1:]
	} else {
		configPath = ""
		partial = after
	}
	ok = true
	return
}

// completeConfigKeys provides completion for config() string arguments.
// Offers config file names at the top level, and nested keys via dot notation.
func (p *Provider) completeConfigKeys(configPath, partial, quote string) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	var keys []types.ShapeField

	if configPath == "" {
		// Top level: list config file names
		keys = p.arrayResolver.ListConfigFiles()
	} else {
		// Drill into the config file via dot-separated path
		parts := strings.Split(configPath, ".")
		configFile := parts[0]
		keys = p.arrayResolver.ParseConfigFile(configFile)
		if keys == nil {
			return nil
		}
		// Drill through remaining segments
		for _, segment := range parts[1:] {
			var nestedType string
			for _, f := range keys {
				if f.Key == segment {
					nestedType = f.Type
					break
				}
			}
			if nestedType == "" {
				return nil
			}
			keys = types.ParseArrayShape(nestedType)
			if keys == nil {
				return nil
			}
		}
	}

	if len(keys) == 0 {
		return nil
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(partial)

	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}

		isNested := strings.HasPrefix(k.Type, "array{") || k.Type == "array"

		// Show helpful detail
		detail := k.Type
		if isNested && strings.HasPrefix(k.Type, "array{") {
			// Summarize nested keys: "array{host: string, port: int}" → "{host, port, ...}"
			inner := types.ParseArrayShape(k.Type)
			if len(inner) > 0 {
				var names []string
				for _, f := range inner {
					if f.Key != "" {
						names = append(names, f.Key)
					}
					if len(names) >= 4 {
						names = append(names, "...")
						break
					}
				}
				detail = "{" + strings.Join(names, ", ") + "}"
			}
		}

		// InsertText: wrap in quotes if user hasn't typed one, otherwise just the key
		insertText := k.Key
		if isNested {
			insertText = k.Key + "."
		}
		if quote == "" {
			// No quote typed — wrap fully (e.g. config( → 'database.')
			q := "'"
			insertText = q + insertText + q
		}

		kind := protocol.CompletionItemKindProperty
		if isNested {
			kind = protocol.CompletionItemKindModule
		}

		// Sort nested sections after leaf values
		sortText := "0" + k.Key
		if isNested {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       kind,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}

// extractContainerArgContext detects whether the cursor is inside a container
// resolution call like app(...), $container->get(...), $container->make(...),
// resolve(...). Returns the partial argument text, the quote character used
// (empty string if no quote), and true if matched.
func extractContainerArgContext(trimmed string) (string, string, bool) {
	// Patterns that resolve from the container
	patterns := []string{"app(", "resolve(", "$container->get(", "$container->make(", "$this->app->make("}
	for _, pat := range patterns {
		idx := strings.LastIndex(trimmed, pat)
		if idx < 0 {
			continue
		}
		after := trimmed[idx+len(pat):]
		// If there's a closing paren, cursor is past the argument — not inside
		if strings.Contains(after, ")") {
			continue
		}
		// Detect quote context
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
func (p *Provider) completeContainerResolve(source, filter, currentNS, quoteCtx string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	lfilter := strings.ToLower(filter)

	// Default quote style for string bindings
	q := "'"
	if quoteCtx == "\"" {
		q = "\""
	}

	// 1. Container bindings (string aliases and interface FQNs)
	if p.container != nil {
		for abstract, binding := range p.container.GetBindings() {
			if lfilter != "" && !strings.HasPrefix(strings.ToLower(abstract), lfilter) {
				// Also try matching the short name (e.g. "Request" matches "Illuminate\Http\Request")
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

			// Build insert text with proper quoting
			var insertText string
			if !strings.Contains(abstract, "\\") {
				sortText = "0" + abstract
			}
			if quoteCtx != "" {
				// User already typed opening quote — insert just the value
				// (editor auto-pairs closing quote)
				insertText = abstract
			} else {
				// No quote yet, wrap fully
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

	// 2. Class name completions (ClassName::class form)
	for _, sym := range p.index.SearchByPrefix(filter) {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum:
			// Skip if already covered by a container binding with same FQN
			if p.container != nil {
				if _, bound := p.container.GetBindings()[sym.FQN]; bound {
					continue
				}
			}
			// Determine the insert text: use short name if imported, FQN otherwise
			insertName := sym.FQN
			file := parser.ParseFile(source)
			if file != nil {
				for _, u := range file.Uses {
					if u.FullName == sym.FQN {
						insertName = u.Alias
						break
					}
				}
			}
			// If user started typing inside quotes, the ::class form needs to
			// replace the quote context — but ::class is typically used without quotes
			classInsert := insertName + "::class"
			if quoteCtx != "" {
				// User typed a quote but is selecting ::class — backtrack the quote
				// by providing the raw class reference (editors handle textEdit better,
				// but insertText is the baseline)
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

func (p *Provider) completeNamespacePath(search, currentNS string) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// Strip leading \ for absolute namespace paths
	fqnPrefix := strings.TrimPrefix(search, "\\")

	// Ensure trailing \ so we search within the namespace
	if !strings.HasSuffix(fqnPrefix, "\\") {
		// User is mid-segment: "Illuminate\Fo" → search prefix "Illuminate\" with filter "Fo"
		if idx := strings.LastIndex(fqnPrefix, "\\"); idx >= 0 {
			nsPrefix := fqnPrefix[:idx+1] // "Illuminate\"
			filter := strings.ToLower(fqnPrefix[idx+1:])
			syms, nsSegs := p.index.SearchByFQNPrefix(nsPrefix)

			// Add matching namespace segments
			for _, seg := range nsSegs {
				if filter == "" || strings.HasPrefix(strings.ToLower(seg), filter) {
					items = append(items, protocol.CompletionItem{
						Label:    seg,
						Kind:     protocol.CompletionItemKindModule,
						Detail:   nsPrefix + seg,
						SortText: "0" + seg,
					})
				}
			}
			// Add matching direct symbols
			for _, sym := range syms {
				if filter != "" && !strings.HasPrefix(strings.ToLower(sym.Name), filter) {
					continue
				}
				item := protocol.CompletionItem{
					Label:    sym.Name,
					Kind:     symKind(sym.Kind),
					Detail:   sym.FQN,
					SortText: sortPriority(sym, currentNS),
				}
				if sym.Kind == symbols.KindFunction {
					item.InsertText = sym.Name + "($0)"
					item.InsertTextFormat = 2
				}
				items = append(items, item)
			}
			return items
		}
	}

	// Exact namespace prefix with trailing \
	syms, nsSegs := p.index.SearchByFQNPrefix(fqnPrefix)

	// Add child namespace segments
	for _, seg := range nsSegs {
		items = append(items, protocol.CompletionItem{
			Label:    seg,
			Kind:     protocol.CompletionItemKindModule,
			Detail:   fqnPrefix + seg,
			SortText: "0" + seg,
		})
	}
	// Add direct symbols in this namespace
	for _, sym := range syms {
		item := protocol.CompletionItem{
			Label:    sym.Name,
			Kind:     symKind(sym.Kind),
			Detail:   sym.FQN,
			SortText: sortPriority(sym, currentNS),
		}
		if sym.Kind == symbols.KindFunction {
			item.InsertText = sym.Name + "($0)"
			item.InsertTextFormat = 2
		}
		items = append(items, item)
	}
	return items
}

// detectMemberContext checks if the cursor is typing after -> or ::
// e.g. "$foo->ba" returns ("$foo->", "ba"), "Foo::cr" returns ("Foo::", "cr").
// Returns ("", "") if not in a member context.
func detectMemberContext(trimmed string) (string, string) {
	// Find the last -> or :: that has text after it (the partial member name)
	for i := len(trimmed) - 1; i >= 2; i-- {
		if trimmed[i-1] == '-' && trimmed[i] == '>' {
			filter := trimmed[i+1:]
			if filter != "" && !strings.ContainsAny(filter, " \t(=;,") {
				return trimmed[:i+1], filter
			}
		}
		if trimmed[i-1] == '?' && i >= 2 && trimmed[i] == '-' && i+1 < len(trimmed) && trimmed[i+1] == '>' {
			filter := trimmed[i+2:]
			if filter != "" && !strings.ContainsAny(filter, " \t(=;,") {
				return trimmed[:i+2], filter
			}
		}
		if trimmed[i-1] == ':' && trimmed[i] == ':' {
			filter := trimmed[i+1:]
			if filter != "" && !strings.ContainsAny(filter, " \t(=;,") {
				return trimmed[:i+1], filter
			}
		}
	}
	return "", ""
}

func filterByPrefix(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
	if prefix == "" {
		return items
	}
	lp := strings.ToLower(prefix)
	var filtered []protocol.CompletionItem
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), lp) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func extractLastWord(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	words := strings.Fields(trimmed)
	if len(words) == 0 {
		return ""
	}
	return words[len(words)-1]
}

func (p *Provider) completeGlobal(prefix, currentNS string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	words := strings.Fields(strings.TrimSpace(prefix))
	search := ""
	if len(words) > 0 {
		search = words[len(words)-1]
	}
	lsearch := strings.ToLower(search)
	// PHP primitive types — highest priority
	for _, t := range []string{"string", "int", "float", "bool", "array", "object", "callable", "iterable", "void", "never", "null", "mixed", "self", "static", "true", "false"} {
		if search == "" || strings.HasPrefix(t, lsearch) {
			items = append(items, protocol.CompletionItem{Label: t, Kind: protocol.CompletionItemKindTypeParameter, SortText: "0" + t})
		}
	}
	for _, kw := range []string{"abstract", "class", "const", "enum", "extends", "final", "fn", "for", "foreach", "function", "if", "implements", "interface", "match", "namespace", "new", "private", "protected", "public", "readonly", "return", "switch", "throw", "trait", "try", "use", "while", "yield"} {
		if search == "" || strings.HasPrefix(kw, lsearch) {
			items = append(items, protocol.CompletionItem{Label: kw, Kind: protocol.CompletionItemKindKeyword, SortText: "5" + kw})
		}
	}
	for _, sym := range p.index.SearchByPrefix(search) {
		// In standalone context only show callable/referenceable symbols:
		// functions, classes, interfaces, enums, traits
		switch sym.Kind {
		case symbols.KindFunction:
			// Skip magic methods and namespaced functions that are likely
			// class methods misidentified by the parser (real global helpers
			// like collect(), config() have no namespace in their FQN)
			if strings.HasPrefix(sym.Name, "__") {
				continue
			}
			if sym.ParentFQN != "" {
				continue
			}
			// Namespaced functions from vendor are almost always parser leaks
			// (class methods that escaped their class body). Skip them unless
			// they're from the project source.
			if strings.Contains(sym.FQN, "\\") && sym.Source == symbols.SourceVendor {
				continue
			}
			item := protocol.CompletionItem{Label: sym.Name, Kind: symKind(sym.Kind), Detail: sym.FQN, SortText: sortPriority(sym, currentNS)}
			item.InsertText = sym.Name + "($0)"
			item.InsertTextFormat = 2
			items = append(items, item)
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
			// Only show type-level symbols when user is actively typing a name
			if search != "" {
				items = append(items, protocol.CompletionItem{Label: sym.Name, Kind: symKind(sym.Kind), Detail: sym.FQN, SortText: sortPriority(sym, currentNS)})
			}
		}
	}
	return items
}

func sortPriority(sym *symbols.Symbol, currentNS string) string {
	switch sym.Source {
	case symbols.SourceProject:
		if currentNS != "" && strings.HasPrefix(sym.FQN, currentNS+"\\") {
			return "1" + sym.Name
		}
		return "2" + sym.Name
	case symbols.SourceBuiltin:
		return "3" + sym.Name
	case symbols.SourceVendor:
		return "4" + sym.Name
	default:
		return "2" + sym.Name
	}
}

func extractNamespace(source string) string {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "namespace ") {
			ns := strings.TrimPrefix(trimmed, "namespace ")
			ns = strings.TrimSuffix(ns, ";")
			ns = strings.TrimSuffix(ns, " {")
			return strings.TrimSpace(ns)
		}
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
