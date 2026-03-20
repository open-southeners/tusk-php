package analyzer

import (
	"encoding/json"
	"strings"

	"github.com/open-southeners/php-lsp/internal/completion"
	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

type Analyzer struct {
	index     *symbols.Index
	container *container.ContainerAnalyzer
}

func NewAnalyzer(index *symbols.Index, ca *container.ContainerAnalyzer) *Analyzer {
	return &Analyzer{index: index, container: ca}
}

func (a *Analyzer) FindDefinition(uri, source string, pos protocol.Position) *protocol.Location {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return nil
	}
	line := lines[pos.Line]
	word := getWordAt(source, pos)
	if word == "" {
		return nil
	}

	file := parser.ParseFile(source)

	// Handle container call arguments: app('request'), app(Request::class)
	// Go to the concrete class definition
	if a.container != nil {
		if loc := a.definitionForContainerArg(line, pos, file, source); loc != nil {
			return loc
		}
	}

	// Handle $variable → go to its type definition
	if strings.HasPrefix(word, "$") {
		return a.definitionForVariable(word, file, source, pos)
	}

	// Find the start of the word on the line
	wordStart := pos.Character
	for wordStart > 0 && isWordChar(line[wordStart-1]) {
		wordStart--
	}

	// Check for -> or :: access (method/property on a class)
	if classFQN := a.resolveAccessChain(line, wordStart, file, source, pos); classFQN != "" {
		if sym := a.findMember(classFQN, word); sym != nil {
			return symbolLocation(sym)
		}
	}

	// Resolve via use statements
	if file != nil {
		for _, u := range file.Uses {
			if u.Alias == word {
				if sym := a.index.Lookup(u.FullName); sym != nil {
					return symbolLocation(sym)
				}
			}
		}
		// Try as class name in current namespace
		fqn := a.resolveClassName(word, file)
		if fqn != word {
			if sym := a.index.Lookup(fqn); sym != nil {
				return symbolLocation(sym)
			}
		}
	}

	// FQN with backslashes
	if strings.Contains(word, "\\") {
		if sym := a.index.Lookup(word); sym != nil {
			return symbolLocation(sym)
		}
	}

	// Fallback: lookup by short name — prefer standalone-appropriate symbols
	// (functions/classes over methods/properties) since we have no access chain.
	lookupName := word
	if idx := strings.LastIndex(word, "\\"); idx >= 0 {
		lookupName = word[idx+1:]
	}
	syms := a.index.LookupByName(lookupName)
	if best := symbols.PickBestStandalone(syms, word); best != nil {
		return symbolLocation(best)
	}

	return nil
}

// definitionForVariable resolves a $variable to its type's definition.
func (a *Analyzer) definitionForVariable(varName string, file *parser.FileNode, source string, pos protocol.Position) *protocol.Location {
	if file == nil {
		return nil
	}
	typeName := a.resolveVariableType(varName, file, source, pos)
	if typeName == "" {
		return nil
	}
	if sym := a.index.Lookup(typeName); sym != nil {
		return symbolLocation(sym)
	}
	return nil
}

// definitionForContainerArg checks if the cursor is inside a container resolution
// call argument (e.g. app('request'), app(Request::class)) and returns the
// definition of the concrete class.
func (a *Analyzer) definitionForContainerArg(line string, pos protocol.Position, file *parser.FileNode, source string) *protocol.Location {
	prefix := line[:min(pos.Character, len(line))]
	// Check if we're inside a container call
	patterns := []string{"app(", "resolve(", "->get(", "->make("}
	insideCall := false
	var argStart int
	for _, pat := range patterns {
		idx := strings.LastIndex(prefix, pat)
		if idx < 0 {
			continue
		}
		after := prefix[idx+len(pat):]
		// If there's already a closing paren before cursor, we're past the call
		if strings.Contains(after, ")") {
			continue
		}
		insideCall = true
		argStart = idx + len(pat)
		break
	}
	if !insideCall {
		return nil
	}

	// Also grab everything after cursor to the closing paren on the same line
	rest := line[min(pos.Character, len(line)):]
	closeIdx := strings.Index(rest, ")")
	var fullArg string
	if closeIdx >= 0 {
		fullArg = line[argStart : pos.Character+closeIdx]
	} else {
		fullArg = line[argStart:min(pos.Character, len(line))]
	}

	arg := strings.TrimSpace(fullArg)
	// Strip first arg only
	if commaIdx := strings.Index(arg, ","); commaIdx >= 0 {
		arg = strings.TrimSpace(arg[:commaIdx])
	}
	arg = strings.Trim(arg, "'\"")

	// Handle ::class suffix
	if strings.HasSuffix(arg, "::class") {
		className := strings.TrimSuffix(arg, "::class")
		arg = a.resolveClassName(className, file)
	}

	// Resolve via container bindings
	if binding := a.container.ResolveDependency(arg); binding != nil {
		if sym := a.index.Lookup(binding.Concrete); sym != nil {
			return symbolLocation(sym)
		}
	}
	// Direct FQN lookup
	if sym := a.index.Lookup(arg); sym != nil {
		return symbolLocation(sym)
	}
	return nil
}

// resolveAccessChain walks left through -> and :: chains to return the FQN
// of the class that owns the member at wordStart.
func (a *Analyzer) resolveAccessChain(line string, wordStart int, file *parser.FileNode, source string, pos protocol.Position) string {
	i := wordStart
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return ""
	}

	if line[i-2] == '-' && line[i-1] == '>' {
		i -= 2
	} else if line[i-2] == ':' && line[i-1] == ':' {
		i -= 2
	} else {
		return ""
	}

	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	// Skip past closing paren for method chains
	if i > 0 && line[i-1] == ')' {
		parenEnd := i
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

		// Check if this is a container call: app(...), resolve(...)
		if a.container != nil {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if concrete := a.resolveContainerCallType(callExpr, file, source); concrete != "" {
				return concrete
			}
		}

		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	end := i
	for i > 0 && isWordChar(line[i-1]) {
		i--
	}
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

	switch target {
	case "$this", "self", "static":
		return a.findEnclosingClass(file, pos)
	case "parent":
		classFQN := a.findEnclosingClass(file, pos)
		if classFQN == "" {
			return ""
		}
		chain := a.index.GetInheritanceChain(classFQN)
		if len(chain) > 0 {
			return chain[0]
		}
		return ""
	}

	if strings.HasPrefix(target, "$") {
		return a.resolveVariableType(target, file, source, pos)
	}

	// Try as a class name (for static access like Logger::create)
	if fqn := a.resolveClassName(target, file); fqn != "" {
		if a.index.Lookup(fqn) != nil {
			return fqn
		}
	}

	// Recursive chain resolution
	ownerFQN := a.resolveAccessChain(line, i, file, source, pos)
	if ownerFQN == "" {
		return ""
	}
	member := a.findMember(ownerFQN, target)
	if member == nil {
		return ""
	}
	return a.memberType(member, file)
}

// resolveVariableType infers the type of a variable from context.
func (a *Analyzer) resolveVariableType(varName string, file *parser.FileNode, source string, pos protocol.Position) string {
	// Check enclosing method parameters
	enclosingMethod := a.findEnclosingMethod(file, pos)
	if enclosingMethod != nil {
		for _, param := range enclosingMethod.Params {
			if param.Name == varName {
				return a.resolveClassName(param.Type.Name, file)
			}
		}
	}

	// Check class properties
	for _, cls := range file.Classes {
		for _, prop := range cls.Properties {
			if "$"+prop.Name == varName && prop.Type.Name != "" {
				return a.resolveClassName(prop.Type.Name, file)
			}
		}
	}

	lines := strings.Split(source, "\n")
	bare := strings.TrimPrefix(varName, "$")
	varPrefix := "$" + bare

	// Look for $var = new ClassName(...)
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
		if strings.HasPrefix(rhs, "new ") {
			className := strings.TrimSpace(rhs[4:])
			if idx := strings.IndexByte(className, '('); idx >= 0 {
				className = className[:idx]
			}
			className = strings.TrimSuffix(className, ";")
			className = strings.TrimSpace(className)
			if className != "" {
				return a.resolveClassName(className, file)
			}
		}
	}

	// Check @var annotations
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
			return a.resolveClassName(fields[0], file)
		}
	}

	return ""
}

func (a *Analyzer) findEnclosingClass(file *parser.FileNode, pos protocol.Position) string {
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

func (a *Analyzer) findEnclosingMethod(file *parser.FileNode, pos protocol.Position) *parser.MethodNode {
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

func (a *Analyzer) resolveClassName(name string, file *parser.FileNode) string {
	if name == "" {
		return ""
	}
	if file == nil {
		return name
	}
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
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
		if a.index.Lookup(fqn) != nil {
			return fqn
		}
	}
	return name
}

func (a *Analyzer) findMember(classFQN, memberName string) *symbols.Symbol {
	members := a.index.GetClassMembers(classFQN)
	for _, m := range members {
		if m.Name == memberName || m.Name == "$"+memberName {
			return m
		}
	}
	return nil
}

func (a *Analyzer) memberType(member *symbols.Symbol, file *parser.FileNode) string {
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
	if typeName == "self" || typeName == "static" {
		return member.ParentFQN
	}
	return a.resolveClassName(typeName, file)
}

// resolveContainerCallType resolves a container call expression to a concrete FQN.
// Uses the shared ExtractContainerCallArg helper from the completion package.
func (a *Analyzer) resolveContainerCallType(expr string, file *parser.FileNode, source string) string {
	arg := completion.ExtractContainerCallArg(expr)
	if arg == "" {
		return ""
	}
	if strings.HasSuffix(arg, "::class") {
		className := strings.TrimSuffix(arg, "::class")
		arg = a.resolveClassName(className, file)
	}
	if binding := a.container.ResolveDependency(arg); binding != nil {
		return binding.Concrete
	}
	if a.index.Lookup(arg) != nil {
		return arg
	}
	return ""
}

func symbolLocation(sym *symbols.Symbol) *protocol.Location {
	if sym.URI == "" || sym.URI == "builtin" {
		return nil
	}
	return &protocol.Location{URI: sym.URI, Range: sym.Range}
}

func buildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

func (a *Analyzer) FindReferences(uri, source string, pos protocol.Position) []protocol.Location {
	word := getWordAt(source, pos)
	if word == "" {
		return nil
	}
	var locs []protocol.Location
	for _, sym := range a.index.LookupByName(word) {
		if sym.URI != "builtin" {
			locs = append(locs, protocol.Location{URI: sym.URI, Range: sym.Range})
		}
		if sym.Kind == symbols.KindInterface {
			for _, impl := range a.index.GetImplementors(sym.FQN) {
				if impl.URI != "builtin" {
					locs = append(locs, protocol.Location{URI: impl.URI, Range: impl.Range})
				}
			}
		}
	}
	return locs
}

func (a *Analyzer) GetDocumentSymbols(uri, source string) []protocol.DocumentSymbol {
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}
	var ds []protocol.DocumentSymbol
	for _, cls := range file.Classes {
		if cls.Name == "" {
			continue
		}
		s := protocol.DocumentSymbol{Name: cls.Name, Kind: protocol.SymbolKindClass,
			Range: mkRange(cls.StartLine), SelectionRange: mkRange(cls.StartLine)}
		for _, m := range cls.Methods {
			if m.Name == "" {
				continue
			}
			s.Children = append(s.Children, protocol.DocumentSymbol{Name: m.Name, Detail: m.Visibility, Kind: protocol.SymbolKindMethod, Range: mkRange(m.StartLine), SelectionRange: mkRange(m.StartLine)})
		}
		for _, p := range cls.Properties {
			if p.Name == "" {
				continue
			}
			s.Children = append(s.Children, protocol.DocumentSymbol{Name: p.Name, Detail: p.Type.Name, Kind: protocol.SymbolKindProperty, Range: mkRange(p.StartLine), SelectionRange: mkRange(p.StartLine)})
		}
		ds = append(ds, s)
	}
	for _, iface := range file.Interfaces {
		if iface.Name == "" {
			continue
		}
		s := protocol.DocumentSymbol{Name: iface.Name, Kind: protocol.SymbolKindInterface, Range: mkRange(iface.StartLine), SelectionRange: mkRange(iface.StartLine)}
		ds = append(ds, s)
	}
	for _, en := range file.Enums {
		if en.Name == "" {
			continue
		}
		ds = append(ds, protocol.DocumentSymbol{Name: en.Name, Kind: protocol.SymbolKindEnum, Range: mkRange(en.StartLine), SelectionRange: mkRange(en.StartLine)})
	}
	for _, fn := range file.Functions {
		if fn.Name == "" {
			continue
		}
		ds = append(ds, protocol.DocumentSymbol{Name: fn.Name, Kind: protocol.SymbolKindFunction, Range: mkRange(fn.StartLine), SelectionRange: mkRange(fn.StartLine)})
	}
	return ds
}

func (a *Analyzer) GetSignatureHelp(uri, source string, pos protocol.Position) *protocol.SignatureHelp {
	line := getLineAt(source, pos.Line)
	if line == "" {
		return nil
	}
	prefix := line[:min(pos.Character, len(line))]
	funcName, activeParam := extractCallInfo(prefix)
	if funcName == "" {
		return nil
	}
	syms := a.index.LookupByName(funcName)
	if len(syms) == 0 {
		return nil
	}
	sym := symbols.PickBestStandalone(syms, funcName)
	if sym == nil {
		return nil
	}
	sig := protocol.SignatureInformation{Label: sym.Name + formatParamLabel(sym)}
	for _, p := range sym.Params {
		l := ""
		if p.Type != "" {
			l = p.Type + " "
		}
		l += p.Name
		sig.Parameters = append(sig.Parameters, protocol.ParameterInformation{Label: l})
	}
	return &protocol.SignatureHelp{Signatures: []protocol.SignatureInformation{sig}, ActiveParameter: activeParam}
}

func extractCallInfo(prefix string) (string, int) {
	activeParam := 0
	depth := 0
	parenPos := -1
	for i := len(prefix) - 1; i >= 0; i-- {
		switch prefix[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				parenPos = i
				goto found
			}
			depth--
		case ',':
			if depth == 0 {
				activeParam++
			}
		}
	}
	return "", 0
found:
	if parenPos <= 0 {
		return "", 0
	}
	end := parenPos
	start := end - 1
	for start >= 0 && isWordChar(prefix[start]) {
		start--
	}
	start++
	if start >= end {
		return "", 0
	}
	return prefix[start:end], activeParam
}

func formatParamLabel(sym *symbols.Symbol) string {
	var ps []string
	for _, p := range sym.Params {
		s := ""
		if p.Type != "" {
			s = p.Type + " "
		}
		s += p.Name
		ps = append(ps, s)
	}
	ret := sym.ReturnType
	if ret == "" {
		ret = "mixed"
	}
	return "(" + strings.Join(ps, ", ") + "): " + ret
}

func mkRange(line int) protocol.Range {
	return protocol.Range{Start: protocol.Position{Line: line}, End: protocol.Position{Line: line}}
}

func getLineAt(source string, line int) string {
	lines := strings.Split(source, "\n")
	if line >= 0 && line < len(lines) {
		return lines[line]
	}
	return ""
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
	// Handle cursor on '$'
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

// --- Rename support ---

// getWordRangeAt returns the word at the cursor and its exact range in the document.
func getWordRangeAt(source string, pos protocol.Position) (string, protocol.Range) {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return "", protocol.Range{}
	}
	line := lines[pos.Line]
	if pos.Character > len(line) {
		return "", protocol.Range{}
	}

	ch := pos.Character
	// Handle cursor on '$' for variables
	if ch < len(line) && line[ch] == '$' {
		start := ch
		end := ch + 1
		for end < len(line) && isWordChar(line[end]) {
			end++
		}
		if end > start+1 {
			return line[start:end], protocol.Range{
				Start: protocol.Position{Line: pos.Line, Character: start},
				End:   protocol.Position{Line: pos.Line, Character: end},
			}
		}
		return "", protocol.Range{}
	}

	start := pos.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	hasDollar := start > 0 && line[start-1] == '$'
	if hasDollar {
		start--
	}
	end := pos.Character
	for end < len(line) && isWordChar(line[end]) {
		end++
	}
	if start >= end {
		return "", protocol.Range{}
	}
	return line[start:end], protocol.Range{
		Start: protocol.Position{Line: pos.Line, Character: start},
		End:   protocol.Position{Line: pos.Line, Character: end},
	}
}

// PrepareRename checks if the symbol at the cursor can be renamed and returns
// the range and current name as a placeholder.
func (a *Analyzer) PrepareRename(uri, source string, pos protocol.Position) *protocol.PrepareRenameResult {
	word, wordRange := getWordRangeAt(source, pos)
	if word == "" {
		return nil
	}

	// Reject built-in symbols
	file := parser.ParseFile(source)

	// Determine what kind of symbol this is
	// Variables are always renameable (local scope)
	if strings.HasPrefix(word, "$") && word != "$this" {
		return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
	}

	// Check if it's a known symbol in the index
	fqn := ""
	if file != nil {
		fqn = a.resolveClassName(word, file)
	}
	if fqn == "" {
		fqn = word
	}

	sym := a.index.Lookup(fqn)
	if sym == nil {
		// Try by short name
		syms := a.index.LookupByName(word)
		if len(syms) > 0 {
			sym = syms[0]
		}
	}

	// Reject built-ins
	if sym != nil && sym.Source == symbols.SourceBuiltin {
		return nil
	}

	// Check for member access context (method/property rename)
	lines := strings.Split(source, "\n")
	if pos.Line >= 0 && pos.Line < len(lines) {
		line := lines[pos.Line]
		wordStart := wordRange.Start.Character
		if wordStart >= 2 {
			before := line[:wordStart]
			trimmed := strings.TrimRight(before, " \t")
			if strings.HasSuffix(trimmed, "->") || strings.HasSuffix(trimmed, "::") {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
	}

	// If it's a symbol we can find, allow rename
	if sym != nil {
		return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
	}

	// Allow rename for identifiers that could be declarations in the current file
	if file != nil {
		for _, cls := range file.Classes {
			if cls.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
			for _, m := range cls.Methods {
				if m.Name == word {
					return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
				}
			}
			for _, p := range cls.Properties {
				if p.Name == word || "$"+p.Name == word {
					return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
				}
			}
		}
		for _, fn := range file.Functions {
			if fn.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
		for _, iface := range file.Interfaces {
			if iface.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
		for _, en := range file.Enums {
			if en.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
	}

	return nil
}

// Rename performs a rename of the symbol at the given position.
// readDocument is used to read file contents for files not open in the editor.
func (a *Analyzer) Rename(uri, source string, pos protocol.Position, newName string, readDocument func(string) string) *protocol.WorkspaceEdit {
	word, _ := getWordRangeAt(source, pos)
	if word == "" {
		return nil
	}

	// Variable rename — local scope only
	if strings.HasPrefix(word, "$") && word != "$this" {
		return a.renameVariable(uri, source, pos, word, newName)
	}

	// Resolve the symbol to its FQN
	file := parser.ParseFile(source)
	fqn := ""

	// Check if cursor is on a member access
	lines := strings.Split(source, "\n")
	if pos.Line >= 0 && pos.Line < len(lines) {
		line := lines[pos.Line]
		wordStart := pos.Character
		for wordStart > 0 && isWordChar(line[wordStart-1]) {
			wordStart--
		}
		if wordStart >= 2 {
			before := line[:wordStart]
			trimmed := strings.TrimRight(before, " \t")
			if strings.HasSuffix(trimmed, "->") || strings.HasSuffix(trimmed, "::") {
				classFQN := a.resolveAccessChain(line, wordStart, file, source, pos)
				if classFQN != "" {
					member := a.findMember(classFQN, word)
					if member != nil {
						fqn = member.FQN
					}
				}
			}
		}
	}

	if fqn == "" && file != nil {
		fqn = a.resolveClassName(word, file)
	}
	if fqn == "" {
		fqn = word
	}

	sym := a.index.Lookup(fqn)
	if sym == nil {
		syms := a.index.LookupByName(word)
		if best := symbols.PickBestStandalone(syms, word); best != nil {
			sym = best
			fqn = sym.FQN
		}
	}

	if sym == nil || sym.Source == symbols.SourceBuiltin {
		return nil
	}

	return a.renameSymbol(sym, newName, readDocument)
}

// renameVariable renames a variable within its enclosing function scope.
func (a *Analyzer) renameVariable(uri, source string, pos protocol.Position, oldName, newName string) *protocol.WorkspaceEdit {
	if !strings.HasPrefix(newName, "$") {
		newName = "$" + newName
	}
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}

	// Find enclosing method/function scope
	method := a.findEnclosingMethod(file, pos)
	scopeStart := 0
	lines := strings.Split(source, "\n")
	scopeEnd := len(lines)
	if method != nil {
		scopeStart = method.StartLine
		// Estimate scope end: find the next method after this one, or use file end
		// Scan for closing brace by counting depth from method start
		depth := 0
		for i := scopeStart; i < len(lines); i++ {
			for _, ch := range lines[i] {
				if ch == '{' {
					depth++
				} else if ch == '}' {
					depth--
					if depth == 0 {
						scopeEnd = i + 1
						goto scopeFound
					}
				}
			}
		}
	scopeFound:
	}

	var edits []protocol.TextEdit
	for i := scopeStart; i < scopeEnd && i < len(lines); i++ {
		line := lines[i]
		// Find all occurrences of the variable on this line
		offset := 0
		for {
			idx := strings.Index(line[offset:], oldName)
			if idx < 0 {
				break
			}
			col := offset + idx
			end := col + len(oldName)
			// Ensure it's a complete token (not part of a longer identifier)
			if end < len(line) && isWordChar(line[end]) {
				offset = end
				continue
			}
			edits = append(edits, protocol.TextEdit{
				Range: protocol.Range{
					Start: protocol.Position{Line: i, Character: col},
					End:   protocol.Position{Line: i, Character: end},
				},
				NewText: newName,
			})
			offset = end
		}
	}

	if len(edits) == 0 {
		return nil
	}
	return &protocol.WorkspaceEdit{
		Changes: map[string][]protocol.TextEdit{uri: edits},
	}
}

// renameSymbol renames a class, method, property, function, interface, enum, or trait
// across the entire workspace.
func (a *Analyzer) renameSymbol(sym *symbols.Symbol, newName string, readDocument func(string) string) *protocol.WorkspaceEdit {
	changes := make(map[string][]protocol.TextEdit)
	oldName := sym.Name

	// For properties, strip $ from the name for access sites
	oldBare := strings.TrimPrefix(oldName, "$")
	newBare := strings.TrimPrefix(newName, "$")

	// Get all indexed file URIs
	allURIs := a.index.GetAllFileURIs()

	for _, fileURI := range allURIs {
		source := readDocument(fileURI)
		if source == "" {
			continue
		}
		lines := strings.Split(source, "\n")
		var fileEdits []protocol.TextEdit

		for i, line := range lines {
			switch sym.Kind {
			case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
				// Search for the short name as a word boundary
				fileEdits = append(fileEdits, findWordEdits(line, i, oldName, newName)...)

			case symbols.KindMethod:
				// Search for ->oldName or ::oldName
				fileEdits = append(fileEdits, findWordEdits(line, i, oldName, newName)...)

			case symbols.KindProperty:
				// In declaration: $oldName
				fileEdits = append(fileEdits, findWordEdits(line, i, "$"+oldBare, "$"+newBare)...)
				// In access: ->oldName (without $)
				needle := "->" + oldBare
				offset := 0
				for {
					idx := strings.Index(line[offset:], needle)
					if idx < 0 {
						break
					}
					col := offset + idx + 2 // skip "->"
					end := col + len(oldBare)
					if end < len(line) && isWordChar(line[end]) {
						offset = end
						continue
					}
					fileEdits = append(fileEdits, protocol.TextEdit{
						Range: protocol.Range{
							Start: protocol.Position{Line: i, Character: col},
							End:   protocol.Position{Line: i, Character: end},
						},
						NewText: newBare,
					})
					offset = end
				}

			case symbols.KindFunction:
				fileEdits = append(fileEdits, findWordEdits(line, i, oldName, newName)...)
			}
		}

		if len(fileEdits) > 0 {
			changes[fileURI] = fileEdits
		}
	}

	if len(changes) == 0 {
		return nil
	}
	return &protocol.WorkspaceEdit{Changes: changes}
}

// findWordEdits finds all occurrences of `word` on `line` that appear as complete
// identifiers (not part of a longer word) and returns TextEdits to replace them.
func findWordEdits(line string, lineNum int, oldWord, newWord string) []protocol.TextEdit {
	var edits []protocol.TextEdit
	offset := 0
	for {
		idx := strings.Index(line[offset:], oldWord)
		if idx < 0 {
			break
		}
		col := offset + idx
		end := col + len(oldWord)
		// Check word boundary before
		if col > 0 && (isWordChar(line[col-1]) || line[col-1] == '$') {
			offset = end
			continue
		}
		// Check word boundary after
		if end < len(line) && isWordChar(line[end]) {
			offset = end
			continue
		}
		edits = append(edits, protocol.TextEdit{
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: col},
				End:   protocol.Position{Line: lineNum, Character: end},
			},
			NewText: newWord,
		})
		offset = end
	}
	return edits
}

// --- Code actions ---

// GetCodeActions returns available code actions for the given range.
func (a *Analyzer) GetCodeActions(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	var actions []protocol.CodeAction

	file := parser.ParseFile(source)
	if file == nil {
		return actions
	}

	// Offer "Copy Namespace" for any position
	ns := file.Namespace
	primaryClass := ""
	for _, cls := range file.Classes {
		primaryClass = cls.Name
		break
	}
	if primaryClass == "" {
		for _, iface := range file.Interfaces {
			primaryClass = iface.Name
			break
		}
	}
	if primaryClass == "" {
		for _, en := range file.Enums {
			primaryClass = en.Name
			break
		}
	}

	fqn := ns
	if primaryClass != "" {
		if ns != "" {
			fqn = ns + "\\" + primaryClass
		} else {
			fqn = primaryClass
		}
	}

	if fqn != "" {
		uriJSON, _ := json.Marshal(uri)
		actions = append(actions, protocol.CodeAction{
			Title:   "Copy Namespace: " + fqn,
			Kind:    "source",
			Command: &protocol.Command{Title: "Copy Namespace", Command: "phpLsp.copyNamespace", Arguments: []json.RawMessage{uriJSON}},
		})
	}

	return actions
}

// GetFileNamespace returns the FQN of the primary class/interface/enum in a file,
// or the namespace if no named type exists.
func (a *Analyzer) GetFileNamespace(uri, source string) string {
	file := parser.ParseFile(source)
	if file == nil {
		return ""
	}
	ns := file.Namespace
	for _, cls := range file.Classes {
		if ns != "" {
			return ns + "\\" + cls.Name
		}
		return cls.Name
	}
	for _, iface := range file.Interfaces {
		if ns != "" {
			return ns + "\\" + iface.Name
		}
		return iface.Name
	}
	for _, en := range file.Enums {
		if ns != "" {
			return ns + "\\" + en.Name
		}
		return en.Name
	}
	return ns
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
