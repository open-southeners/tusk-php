// Package resolve provides shared PHP symbol resolution utilities used by
// completion, hover, and analyzer packages. Consolidates duplicated logic
// for class name resolution, enclosing scope detection, member lookup,
// and text helpers.
package resolve

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/phparray"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// Resolver provides shared resolution methods that depend on the symbol index.
type Resolver struct {
	Index *symbols.Index
	// ChainResolver is an optional callback to resolve method chain expressions
	// like "Category::first()" or "$foo->bar()->baz()". Set by the completion/hover
	// provider that has access to the full chain resolution logic.
	ChainResolver func(expr string, source string, pos protocol.Position, file *parser.FileNode) string
	// TypedChainResolver is like ChainResolver but returns a ResolvedType
	// preserving generic parameters. Used for variable type inference that
	// needs to carry generic context through assignments.
	TypedChainResolver func(expr string, source string, pos protocol.Position, file *parser.FileNode) ResolvedType
}

// NewResolver creates a resolver with the given symbol index.
func NewResolver(index *symbols.Index) *Resolver {
	return &Resolver{Index: index}
}

// ResolveClassName resolves a short or partially-qualified class name to a FQN
// using use statements and the file's namespace.
func (r *Resolver) ResolveClassName(name string, file *parser.FileNode) string {
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
		if r.Index.Lookup(fqn) != nil {
			return fqn
		}
	}
	return name
}

// FindEnclosingClass returns the FQN of the class that contains the given position.
func FindEnclosingClass(file *parser.FileNode, pos protocol.Position) string {
	if file == nil {
		return ""
	}
	for _, cls := range file.Classes {
		if pos.Line >= cls.StartLine {
			fqn := cls.FullName
			if fqn == "" {
				fqn = BuildFQN(file.Namespace, cls.Name)
			}
			return fqn
		}
	}
	return ""
}

// FindEnclosingMethod returns the method node that contains the given position.
func FindEnclosingMethod(file *parser.FileNode, pos protocol.Position) *parser.MethodNode {
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

// FindMember looks up a member (method, property, constant) on a class,
// traversing the inheritance chain and traits via GetClassMembers.
func (r *Resolver) FindMember(classFQN, memberName string) *symbols.Symbol {
	members := r.Index.GetClassMembers(classFQN)
	for _, m := range members {
		if m.Name == memberName || m.Name == "$"+memberName {
			return m
		}
	}
	return nil
}

// MemberTypeResolved returns the fully resolved type including generic params.
// callerFQN is the class the method was called on (for late static binding:
// Category::query() where query is on Model — callerFQN is Category).
// typeContext carries generic params from the caller (e.g., Builder<Category>).
func (r *Resolver) MemberTypeResolved(member *symbols.Symbol, file *parser.FileNode, callerFQN string, typeContext []ResolvedType) ResolvedType {
	// 1. Check if the member's return type has generic syntax or needs
	//    late static binding resolution.
	rawType := r.rawMemberReturnType(member)
	if rawType != "" && rawType != "void" && rawType != "mixed" {
		staticFQN := callerFQN
		if staticFQN == "" {
			staticFQN = member.ParentFQN
		}

		// Handle union types: pick the best part (prefer generic over bare).
		// E.g., "Builder<static>|Category" → use "Builder<static>".
		// Track if null was in the union for nullable marking.
		resolvedRaw := strings.TrimPrefix(rawType, "\\")
		nullable := strings.HasPrefix(resolvedRaw, "?")
		if nullable {
			resolvedRaw = resolvedRaw[1:]
		}
		if strings.Contains(resolvedRaw, "|") {
			if HasNullInUnion(resolvedRaw) {
				nullable = true
			}
			resolvedRaw = PickBestUnionPart(resolvedRaw)
		}

		if resolvedRaw == "self" || resolvedRaw == "static" || resolvedRaw == "$this" {
			return ResolvedType{FQN: staticFQN, Params: typeContext, Nullable: nullable}
		}

		resolvedRaw = strings.TrimPrefix(resolvedRaw, "\\")

		// Build template substitution map from the parent class's @template tags
		// and the typeContext. E.g., Builder has @template TModel and typeContext
		// is [Category], so TModel → Category.
		templateSubst := buildTemplateSubst(r.Index, member.ParentFQN, typeContext)

		if strings.Contains(resolvedRaw, "<") {
			rt := ParseGenericType(resolvedRaw)
			rt.FQN = resolveTypeParam(r, rt.FQN, staticFQN, templateSubst, file)
			for i := range rt.Params {
				rt.Params[i].FQN = resolveTypeParam(r, rt.Params[i].FQN, staticFQN, templateSubst, file)
			}
			rt.Nullable = rt.Nullable || nullable
			return rt
		}

		// Even without <>, the return type might be a bare template param name
		// (e.g., @return TModel|null → resolvedRaw is "TModel", nullable is true)
		if sub, ok := templateSubst[resolvedRaw]; ok {
			result := sub
			result.Nullable = result.Nullable || nullable
			return result
		}
	}

	// 2. Try template resolution from the registry if we have context
	if len(typeContext) > 0 && member.Kind == symbols.KindMethod {
		if rt := ResolveTemplateReturn(member.ParentFQN, member.Name, typeContext); !rt.IsEmpty() {
			rt.FQN = r.ResolveClassName(rt.FQN, file)
			for i := range rt.Params {
				if rt.Params[i].FQN != "" && rt.Params[i].FQN != "int" && rt.Params[i].FQN != "mixed" {
					rt.Params[i].FQN = r.ResolveClassName(rt.Params[i].FQN, file)
				}
			}
			return rt
		}
	}

	// 3. Fall back to standard resolution (strips generics)
	fqn := r.MemberType(member, file)
	if fqn == "" {
		return ResolvedType{}
	}
	return ResolvedType{FQN: fqn}
}

// pickBestUnionPart selects the most informative part from a union type string.
// Prefers parts with generic syntax (Builder<static>) over bare types (Category).
// Skips null/void/mixed. For "Builder<static>|Category", returns "Builder<static>".
func PickBestUnionPart(union string) string {
	// Split union respecting < > nesting
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(union); i++ {
		switch union[i] {
		case '<':
			depth++
		case '>':
			depth--
		case '|':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(union[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(union[start:]))

	// Prefer the first part with generics; otherwise first non-null part
	var bestGeneric, bestPlain string
	for _, p := range parts {
		p = strings.TrimPrefix(p, "\\")
		if p == "" || p == "null" || p == "void" || p == "mixed" {
			continue
		}
		if strings.Contains(p, "<") && bestGeneric == "" {
			bestGeneric = p
		}
		if bestPlain == "" {
			bestPlain = p
		}
	}
	if bestGeneric != "" {
		return bestGeneric
	}
	return bestPlain
}

// hasNullInUnion checks if a union type string contains "null" as one of its parts.
func HasNullInUnion(union string) bool {
	depth := 0
	start := 0
	for i := 0; i < len(union); i++ {
		switch union[i] {
		case '<':
			depth++
		case '>':
			depth--
		case '|':
			if depth == 0 {
				if strings.TrimSpace(union[start:i]) == "null" {
					return true
				}
				start = i + 1
			}
		}
	}
	return strings.TrimSpace(union[start:]) == "null"
}

// buildTemplateSubst builds a map from template parameter names to concrete types
// using the class's @template declarations and the provided type context.
// E.g., Builder has @template TModel, typeContext is [{FQN: "Category"}]
// → returns {"TModel": {FQN: "Category"}}.
func buildTemplateSubst(index *symbols.Index, classFQN string, typeContext []ResolvedType) map[string]ResolvedType {
	if len(typeContext) == 0 {
		return nil
	}

	// First check the class's own @template tags from Symbol.Templates
	sym := index.Lookup(classFQN)
	if sym != nil && len(sym.Templates) > 0 {
		subst := make(map[string]ResolvedType)
		for i, tmpl := range sym.Templates {
			if i < len(typeContext) {
				subst[tmpl.Name] = typeContext[i]
			}
		}
		return subst
	}

	// Fall back to the hardcoded knownTemplates registry
	if tmpl, ok := knownTemplates[classFQN]; ok {
		subst := make(map[string]ResolvedType)
		for i, paramName := range tmpl.Params {
			if i < len(typeContext) {
				subst[paramName] = typeContext[i]
			}
		}
		return subst
	}

	return nil
}

// resolveTypeParam resolves a type name that could be static/self/$this,
// a template parameter name (TModel), or a regular class name.
func resolveTypeParam(r *Resolver, name, staticFQN string, templateSubst map[string]ResolvedType, file *parser.FileNode) string {
	switch name {
	case "static", "self", "$this":
		return staticFQN
	}
	if sub, ok := templateSubst[name]; ok {
		return sub.FQN
	}
	return r.ResolveClassName(name, file)
}

// resolveStaticOrClassName resolves "static"/"self"/"$this" to the caller FQN,
// or resolves a regular class name through use statements.
func resolveStaticOrClassName(r *Resolver, name, staticFQN string, file *parser.FileNode) string {
	switch name {
	case "static", "self", "$this":
		return staticFQN
	default:
		return r.ResolveClassName(name, file)
	}
}

// inferArgType resolves the type of a constructor/method argument expression.
// It handles variables, array literals, and chain expressions.
func (r *Resolver) inferArgType(argStr string, lines []string, pos protocol.Position, file *parser.FileNode, assignLine int) ResolvedType {
	argStr = strings.TrimSpace(argStr)
	// First arg only (split by comma, take first)
	if commaIdx := IndexOutsideBrackets(argStr, ','); commaIdx >= 0 {
		argStr = strings.TrimSpace(argStr[:commaIdx])
	}
	if argStr == "" {
		return ResolvedType{}
	}

	// Array literal
	if strings.HasPrefix(argStr, "[") {
		return InferArrayLiteralType(argStr)
	}

	// Variable reference
	if strings.HasPrefix(argStr, "$") {
		return r.ResolveVariableTypeTyped(argStr, lines, pos, file)
	}

	return ResolvedType{}
}

// inferConstructorGenerics binds a class's @template params from the constructor
// argument type. E.g., new Collection(array<int, Shape>) → Collection<int, Shape>.
func (r *Resolver) inferConstructorGenerics(classFQN string, argType ResolvedType) ResolvedType {
	sym := r.Index.Lookup(classFQN)
	if sym == nil || len(sym.Templates) == 0 {
		return ResolvedType{}
	}

	// The argument type is array<K, V> or array{shape} — extract K and V
	if argType.FQN != "array" {
		return ResolvedType{}
	}

	var params []ResolvedType
	if len(argType.Params) >= 2 {
		// array<K, V> → bind templates in order
		for i := range sym.Templates {
			if i < len(argType.Params) {
				params = append(params, argType.Params[i])
			}
		}
	} else if argType.Shape != "" {
		// array{shape} with no generic params — treat as array<string, mixed>
		// This case is for associative arrays passed directly
		params = append(params, ResolvedType{FQN: "string"})
		params = append(params, ResolvedType{FQN: "mixed"})
	}

	if len(params) == 0 {
		return ResolvedType{}
	}
	return ResolvedType{FQN: classFQN, Params: params}
}

// indexOutsideBrackets finds the first occurrence of ch outside nested brackets/parens.
func IndexOutsideBrackets(s string, ch byte) int {
	depth := 0
	inString := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString != 0 {
			if c == inString && (i == 0 || s[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			inString = c
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		default:
			if c == ch && depth == 0 {
				return i
			}
		}
	}
	return -1
}

// rawMemberReturnType extracts the raw return type string from a member
// without any resolution or stripping. Used by MemberTypeResolved to check
// for generic syntax before falling back to MemberType.
func (r *Resolver) rawMemberReturnType(member *symbols.Symbol) string {
	switch member.Kind {
	case symbols.KindMethod:
		if member.ReturnType != "" {
			return member.ReturnType
		}
		if member.DocComment != "" {
			if doc := parser.ParseDocBlock(member.DocComment); doc != nil && doc.Return.Type != "" {
				return doc.Return.Type
			}
		}
	case symbols.KindProperty:
		return member.Type
	}
	return ""
}

// MemberType returns the resolved type of a member symbol (property type or method return type).
// Falls back to @return/@var docblock annotations when no type hint is present.
func (r *Resolver) MemberType(member *symbols.Symbol, file *parser.FileNode) string {
	var typeName string
	switch member.Kind {
	case symbols.KindProperty:
		typeName = member.Type
		if typeName == "" && member.DocComment != "" {
			if doc := parser.ParseDocBlock(member.DocComment); doc != nil {
				if vars, ok := doc.Tags["var"]; ok && len(vars) > 0 {
					typeName = strings.Fields(vars[0])[0]
				}
			}
		}
	case symbols.KindMethod:
		typeName = member.ReturnType
		if typeName == "" && member.DocComment != "" {
			if doc := parser.ParseDocBlock(member.DocComment); doc != nil && doc.Return.Type != "" {
				typeName = doc.Return.Type
			}
		}
	default:
		return ""
	}
	if typeName == "" || typeName == "void" || typeName == "mixed" {
		return ""
	}
	if typeName == "self" || typeName == "static" || typeName == "$this" {
		return member.ParentFQN
	}
	// Strip leading backslash for FQN types from docblocks
	typeName = strings.TrimPrefix(typeName, "\\")
	// Strip generic type parameters: Builder<static> → Builder
	if idx := strings.IndexByte(typeName, '<'); idx > 0 {
		typeName = typeName[:idx]
	}
	// Handle union types: take the first non-null type
	if strings.Contains(typeName, "|") {
		for _, part := range strings.Split(typeName, "|") {
			part = strings.TrimSpace(part)
			if part != "" && part != "null" && part != "void" && part != "mixed" {
				typeName = part
				break
			}
		}
	}
	return r.ResolveClassName(typeName, file)
}

// ResolveVariableTypeTyped infers the type of a variable preserving generic context.
// Used by the typed chain resolver so that $categories = Category::query()->get()
// results in $categories having type Collection<int, Category>.
func (r *Resolver) ResolveVariableTypeTyped(varName string, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	if file == nil {
		return ResolvedType{}
	}

	// Special case: $this
	if varName == "$this" {
		return ResolvedType{FQN: FindEnclosingClass(file, pos)}
	}

	// Check parameter types
	enclosingMethod := FindEnclosingMethod(file, pos)
	if enclosingMethod != nil {
		for _, param := range enclosingMethod.Params {
			if param.Name == varName {
				return ResolvedType{FQN: r.ResolveClassName(param.Type.Name, file)}
			}
		}
	}

	bare := strings.TrimPrefix(varName, "$")
	varPrefix := "$" + bare

	// Look for assignments with chain expressions
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
		rhs = strings.TrimSuffix(rhs, ";")
		rhs = strings.TrimSpace(rhs)

		// $var = new ClassName(...) — with constructor generic inference
		if strings.HasPrefix(rhs, "new ") {
			className := strings.TrimSpace(rhs[4:])
			argStr := ""
			if idx := strings.IndexByte(className, '('); idx >= 0 {
				// Extract the constructor argument(s) between ( and )
				rest := className[idx+1:]
				className = className[:idx]
				if closeIdx := strings.LastIndexByte(rest, ')'); closeIdx >= 0 {
					argStr = strings.TrimSpace(rest[:closeIdx])
				}
			}
			className = strings.TrimSpace(className)
			if className == "" {
				break
			}
			classFQN := r.ResolveClassName(className, file)

			// Try to infer generic params from constructor argument
			if argStr != "" {
				argRT := r.inferArgType(argStr, lines, pos, file, i)
				if !argRT.IsEmpty() && (argRT.IsGeneric() || argRT.Shape != "") {
					if rt := r.inferConstructorGenerics(classFQN, argRT); !rt.IsEmpty() {
						return rt
					}
				}
			}
			return ResolvedType{FQN: classFQN}
		}

		// $var = [...] — array literal with shape inference
		if strings.HasPrefix(rhs, "[") {
			// Collect the full multi-line array literal
			arrayLiteral := phparray.CollectArrayLiteral(lines, i)
			if arrayLiteral != "" {
				rt := InferArrayLiteralType(arrayLiteral)
				if !rt.IsEmpty() {
					return rt
				}
			}
		}

		// $var = $other[index] — array element access
		if bracketIdx := strings.Index(rhs, "["); bracketIdx > 0 && strings.HasPrefix(rhs, "$") {
			arrayVar := strings.TrimSpace(rhs[:bracketIdx])
			arrayType := r.ResolveVariableTypeTyped(arrayVar, lines, pos, file)
			if arrayType.FQN == "array" && len(arrayType.Params) >= 2 {
				// array<TKey, TValue>[index] → TValue
				return arrayType.Params[1]
			}
		}

		// Use typed chain resolver if available
		if r.TypedChainResolver != nil && (strings.Contains(rhs, "->") || strings.Contains(rhs, "::")) {
			if rt := r.TypedChainResolver(rhs, strings.Join(lines, "\n"), pos, file); !rt.IsEmpty() {
				return rt
			}
		}

		// Fall back to standard chain resolver
		if r.ChainResolver != nil && (strings.Contains(rhs, "->") || strings.Contains(rhs, "::")) {
			if t := r.ChainResolver(rhs, strings.Join(lines, "\n"), pos, file); t != "" {
				return ResolvedType{FQN: t}
			}
		}

		break
	}

	// Fall back to standard resolution
	fqn := r.ResolveVariableType(varName, lines, pos, file)
	return ResolvedType{FQN: fqn}
}

// ResolveVariableType infers the type of a variable from context:
// method/function parameters, class properties, $var = new ClassName(...),
// @var annotations, and literal type inference.
func (r *Resolver) ResolveVariableType(varName string, lines []string, pos protocol.Position, file *parser.FileNode) string {
	if file == nil {
		return ""
	}

	// Special case: $this
	if varName == "$this" {
		return FindEnclosingClass(file, pos)
	}

	// 1. Check enclosing method/function parameter type hints
	enclosingMethod := FindEnclosingMethod(file, pos)
	if enclosingMethod != nil {
		for _, param := range enclosingMethod.Params {
			if param.Name == varName {
				return r.ResolveClassName(param.Type.Name, file)
			}
		}
	}
	// Also check standalone function parameters
	for _, fn := range file.Functions {
		if pos.Line >= fn.StartLine {
			for _, param := range fn.Params {
				if param.Name == varName && param.Type.Name != "" {
					return r.ResolveClassName(param.Type.Name, file)
				}
			}
		}
	}

	// 2. Check class properties
	for _, cls := range file.Classes {
		for _, prop := range cls.Properties {
			if "$"+prop.Name == varName && prop.Type.Name != "" {
				return r.ResolveClassName(prop.Type.Name, file)
			}
		}
	}

	bare := strings.TrimPrefix(varName, "$")
	varPrefix := "$" + bare

	// 3. Look for $var = new ClassName(...) and literal assignments
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
				return r.ResolveClassName(className, file)
			}
		}
		// $var = expr; — infer literal type
		rhs = strings.TrimSuffix(rhs, ";")
		rhs = strings.TrimSpace(rhs)
		if t := InferLiteralType(rhs); t != "" {
			return t
		}
		// $var = ClassName::method() or $var = $foo->bar()->baz()
		if r.ChainResolver != nil && (strings.Contains(rhs, "->") || strings.Contains(rhs, "::")) {
			if t := r.ChainResolver(rhs, strings.Join(lines, "\n"), pos, file); t != "" {
				return t
			}
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
			return r.ResolveClassName(fields[0], file)
		}
	}

	return ""
}

// InferLiteralType returns the PHP type for a literal expression value.
func InferLiteralType(expr string) string {
	if expr == "" {
		return ""
	}
	if expr[0] == '\'' || expr[0] == '"' {
		return "string"
	}
	lower := strings.ToLower(expr)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	if lower == "null" {
		return "null"
	}
	if expr[0] == '[' || strings.HasPrefix(lower, "array(") {
		return "array"
	}
	if expr[0] >= '0' && expr[0] <= '9' || expr[0] == '-' {
		if strings.ContainsAny(expr, ".eE") {
			return "float"
		}
		return "int"
	}
	return ""
}

// BuildFQN combines a namespace and name into a fully qualified name.
func BuildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

// --- Text helpers ---

// IsWordChar returns true if the byte is a valid PHP identifier character
// (letters, digits, underscore, backslash).
func IsWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '\\'
}

// JoinChainLines joins the current line with preceding continuation lines
// that form a method chain. When a line's meaningful content starts with
// -> or ::, the previous line is prepended (trimmed) to form a single
// expression. This allows resolveAccessChain to walk multi-line chains.
// Returns the joined line and the adjusted character offset within it.
func JoinChainLines(lines []string, lineNum, character int) (string, int) {
	if lineNum < 0 || lineNum >= len(lines) {
		return "", character
	}
	joined := lines[lineNum]
	offset := character

	// Walk backward, prepending lines as long as the current joined line
	// starts with a chain continuation operator (-> or ::).
	for i := lineNum - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(joined)
		if !strings.HasPrefix(trimmed, "->") && !strings.HasPrefix(trimmed, "::") && !strings.HasPrefix(trimmed, "?->") {
			break
		}
		prev := strings.TrimRight(lines[i], " \t")
		offset += len(prev)
		joined = prev + joined
	}
	return joined, offset
}

// SplitLines splits source into lines. Use this once per request,
// then pass the result to LineAt/WordAt to avoid repeated splitting.
func SplitLines(source string) []string {
	return strings.Split(source, "\n")
}

// LineAt returns the line at the given 0-based index from pre-split lines.
func LineAt(lines []string, line int) string {
	if line >= 0 && line < len(lines) {
		return lines[line]
	}
	return ""
}

// GetLineAt returns the line at the given 0-based line number.
func GetLineAt(source string, line int) string {
	return LineAt(SplitLines(source), line)
}

// WordAt extracts the word at the given position from pre-split lines,
// including $ prefix for variables.
func WordAt(lines []string, pos protocol.Position) string {
	if pos.Line < 0 || pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character > len(line) {
		return ""
	}
	ch := pos.Character
	if ch < len(line) && line[ch] == '$' {
		start := ch
		end := ch + 1
		for end < len(line) && IsWordChar(line[end]) {
			end++
		}
		if end > start+1 {
			return line[start:end]
		}
		return ""
	}
	start := pos.Character
	for start > 0 && IsWordChar(line[start-1]) {
		start--
	}
	if start > 0 && line[start-1] == '$' {
		start--
	}
	end := pos.Character
	for end < len(line) && IsWordChar(line[end]) {
		end++
	}
	if start >= end {
		return ""
	}
	return line[start:end]
}

// GetWordAt extracts the word at the given position, including $ prefix for variables.
func GetWordAt(source string, pos protocol.Position) string {
	return WordAt(SplitLines(source), pos)
}

// ExtractContainerCallArg extracts the string/class argument from a container
// resolution expression like app('request'), app(Request::class), resolve('cache').
// Returns the cleaned argument or empty string if not a container call.
func ExtractContainerCallArg(expr string) string {
	t := strings.TrimSpace(expr)
	if !strings.HasSuffix(t, ")") {
		return ""
	}
	depth := 0
	openIdx := -1
	for i := len(t) - 1; i >= 0; i-- {
		if t[i] == ')' {
			depth++
		} else if t[i] == '(' {
			depth--
			if depth == 0 {
				openIdx = i
				break
			}
		}
	}
	if openIdx < 0 {
		return ""
	}
	funcPart := strings.TrimSpace(t[:openIdx])
	isContainerCall := false
	for _, suffix := range []string{"app", "resolve", "->get", "->make"} {
		if strings.HasSuffix(funcPart, suffix) {
			isContainerCall = true
			break
		}
	}
	if !isContainerCall {
		return ""
	}
	arg := strings.TrimSpace(t[openIdx+1 : len(t)-1])
	if commaIdx := strings.Index(arg, ","); commaIdx >= 0 {
		arg = strings.TrimSpace(arg[:commaIdx])
	}
	arg = strings.Trim(arg, "'\"")
	return arg
}
