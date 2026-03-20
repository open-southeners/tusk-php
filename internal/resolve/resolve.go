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
func (r *Resolver) MemberType(member *symbols.Symbol, file *parser.FileNode) string {
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
	return r.ResolveClassName(typeName, file)
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

// GetLineAt returns the line at the given 0-based line number.
func GetLineAt(source string, line int) string {
	lines := strings.Split(source, "\n")
	if line >= 0 && line < len(lines) {
		return lines[line]
	}
	return ""
}

// GetWordAt extracts the word at the given position, including $ prefix for variables.
func GetWordAt(source string, pos protocol.Position) string {
	lines := strings.Split(source, "\n")
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
