package completion

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/models"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/resolve"
	"github.com/open-southeners/php-lsp/internal/symbols"
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
