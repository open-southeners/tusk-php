// Package resolve provides shared PHP symbol resolution utilities used by
// completion, hover, and analyzer packages. Consolidates duplicated logic
// for class name resolution, enclosing scope detection, member lookup,
// and text helpers.
package resolve

import (
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

// Resolver provides shared resolution methods that depend on the symbol index.
type Resolver struct {
	Index *symbols.Index
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
