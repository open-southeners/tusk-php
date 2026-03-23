package completion

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// Builder methods whose first string argument is a column name.
// These suggest all model properties (declared, virtual, migration-derived, etc.)
// except relations and accessors.
var columnMethods = map[string]bool{
	"where": true, "whereIn": true, "whereNotIn": true,
	"whereNull": true, "whereNotNull": true,
	"whereBetween": true, "whereNotBetween": true,
	"whereDate": true, "whereMonth": true, "whereDay": true,
	"whereYear": true, "whereTime": true, "whereColumn": true,
	"orderBy": true, "orderByDesc": true,
	"latest": true, "oldest": true,
	"groupBy": true, "having": true,
	"select": true, "addSelect": true,
	"pluck": true, "value": true,
	"increment": true, "decrement": true,
}

// Builder methods that only accept actual database column names (from
// migrations, database introspection, IDE helper @property, or Doctrine).
// These are stricter than columnMethods — they exclude declared PHP properties
// and accessors since those aren't real DB columns.
var dbColumnMethods = map[string]bool{
	"get": true,
}

// Builder methods whose first string argument is a relation name.
var relationMethods = map[string]bool{
	"with": true, "without": true,
	"has": true, "doesntHave": true,
	"whereHas": true, "whereDoesntHave": true,
	"withCount": true, "withSum": true, "withAvg": true,
	"withMin": true, "withMax": true, "withExists": true,
	"load": true, "loadMissing": true, "loadCount": true,
}

// Methods that accept arrays of column/relation names as elements.
// Methods NOT in this set (e.g. where) use associative array syntax
// and should not trigger completion inside array arguments.
var arrayArgMethods = map[string]bool{
	// Column arrays
	"select": true, "addSelect": true, "get": true,
	"groupBy": true, "whereNull": true, "whereNotNull": true,
	// Relation arrays
	"with": true, "without": true,
	"withCount": true, "withSum": true, "withAvg": true,
	"withMin": true, "withMax": true, "withExists": true,
	"load": true, "loadMissing": true, "loadCount": true,
}

// Relation return type FQN fragments used to identify relation methods on models.
var relationReturnTypes = map[string]bool{
	"HasOne": true, "HasMany": true,
	"BelongsTo": true, "BelongsToMany": true,
	"MorphOne": true, "MorphMany": true,
	"MorphTo": true, "MorphToMany": true, "MorphedByMany": true,
	"HasOneThrough": true, "HasManyThrough": true,
}

// extractBuilderArgContext detects when the cursor is inside a string argument
// of a Builder method call. Handles both direct string args and array elements:
//
//	"Category::where('"              → ("where", "", "'", true)
//	"$q->orderBy('na"                → ("orderBy", "na", "'", true)
//	"Category::with(['pro"           → ("with", "pro", "'", true)
//	"Category::select(['id', '"      → ("select", "", "'", true)
//	"Category::with(["               → ("with", "", "", true)
//	"Category::where("               → ("where", "", "", true)
func extractBuilderArgContext(trimmed string) (method, partial, quote string, ok bool) {
	// Search backward for the last ->method( or ::method( pattern
	// We look for the opening paren first, then extract the method name before it.
	for i := len(trimmed) - 1; i >= 0; i-- {
		if trimmed[i] != '(' {
			continue
		}
		// Everything after '(' is the argument area
		after := trimmed[i+1:]

		// If there's a closing paren, this call is complete — skip
		if strings.Contains(after, ")") {
			continue
		}

		// Try to extract a string context from the argument area.
		// This handles both direct string args and array element args.
		q, rest, isArray, matched := extractStringFromArgs(after)
		if !matched {
			continue
		}

		// Extract method name before the paren
		j := i - 1
		for j >= 0 && (trimmed[j] == ' ' || trimmed[j] == '\t') {
			j--
		}
		nameEnd := j + 1
		for j >= 0 && resolve.IsWordChar(trimmed[j]) {
			j--
		}
		if j+1 >= nameEnd {
			continue
		}
		methodName := trimmed[j+1 : nameEnd]

		// Verify there's a -> or :: before the method name
		k := j
		for k >= 0 && (trimmed[k] == ' ' || trimmed[k] == '\t') {
			k--
		}
		if k < 1 {
			continue
		}
		isArrow := k >= 1 && trimmed[k-1] == '-' && trimmed[k] == '>'
		isStatic := k >= 1 && trimmed[k-1] == ':' && trimmed[k] == ':'
		if !isArrow && !isStatic {
			continue
		}

		// Check if this is a known Builder method
		if !columnMethods[methodName] && !relationMethods[methodName] && !dbColumnMethods[methodName] {
			continue
		}

		// Array args are only valid for methods that accept column/relation arrays
		if isArray && !arrayArgMethods[methodName] {
			continue
		}

		return methodName, rest, q, true
	}
	return "", "", "", false
}

// extractStringFromArgs extracts the quote character and partial text from
// a method's argument area. Handles:
//   - Direct string arg: "'partial" or "" (no quote yet)
//   - Array element: "['partial" or "['done', 'partial" or "["
//
// Returns (quote, partial, isArray, matched). matched is false if the context
// doesn't look like a string argument (e.g. variable, closure, associative array).
func extractStringFromArgs(after string) (quote, partial string, isArray, matched bool) {
	s := strings.TrimSpace(after)

	// Array argument: starts with [
	if strings.HasPrefix(s, "[") {
		inner := s[1:]
		// Closed array — skip
		if strings.Contains(inner, "]") {
			return "", "", true, false
		}
		// Find the last string-start position after ',' or '[' boundary
		q, p, ok := extractStringAtEnd(inner)
		return q, p, true, ok
	}

	// Direct argument (no array)
	// Must be in the first argument position: no comma before cursor
	if strings.Contains(s, ",") {
		return "", "", false, false
	}

	// Quote + partial
	if len(s) > 0 && (s[0] == '\'' || s[0] == '"') {
		return string(s[0]), s[1:], false, true
	}

	// Empty (no quote yet) — only if nothing else is typed
	if len(s) == 0 {
		return "", "", false, true
	}

	// Something else (variable, closure, etc.) — not a string context
	return "", "", false, false
}

// extractStringAtEnd finds the string being typed at the end of an array's
// inner content. Handles: "", "'partial", "'done', 'partial", "'done','".
// Returns (quote, partial, matched).
func extractStringAtEnd(inner string) (string, string, bool) {
	s := strings.TrimSpace(inner)

	// Empty array so far: [  or [  (just whitespace)
	if len(s) == 0 {
		return "", "", true
	}

	// Find the last separator (comma) that's not inside a string
	lastSep := -1
	inStr := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inStr != 0 {
			if ch == inStr {
				inStr = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			inStr = ch
			continue
		}
		if ch == ',' {
			lastSep = i
		}
		// Associative array: => means not a simple value list
		if ch == '=' && i+1 < len(s) && s[i+1] == '>' {
			return "", "", false
		}
	}

	// Get the text after the last separator (or all of it if no separator)
	var tail string
	if lastSep >= 0 {
		tail = strings.TrimSpace(s[lastSep+1:])
	} else {
		tail = s
	}

	// Quote + partial
	if len(tail) > 0 && (tail[0] == '\'' || tail[0] == '"') {
		return string(tail[0]), tail[1:], true
	}

	// Empty after comma or at start — no quote typed yet
	if len(tail) == 0 {
		return "", "", true
	}

	// Something else — not a string context
	return "", "", false
}

// completeBuilderArg dispatches to column or relation completion based on the method.
func (p *Provider) completeBuilderArg(method, partial, quote, prefix, source string, pos protocol.Position, file *parser.FileNode) []protocol.CompletionItem {
	modelFQN := p.resolveBuilderModel(prefix, method, source, pos, file)
	if modelFQN == "" {
		return nil
	}

	if dbColumnMethods[method] {
		return p.completeBuilderDBColumns(modelFQN, partial, quote)
	}
	if columnMethods[method] {
		return p.completeBuilderColumns(modelFQN, partial, quote)
	}
	return p.completeBuilderRelations(modelFQN, partial, quote)
}

// resolveBuilderModel finds the Eloquent model FQN from the expression before
// the Builder method call. Works for both static calls (Category::where) and
// chained calls (Category::query()->where, $query->where).
func (p *Provider) resolveBuilderModel(prefix, method, source string, pos protocol.Position, file *parser.FileNode) string {
	// Find the method call in the prefix to get everything before it
	// We need to find ->method( or ::method( and resolve the chain before it
	trimmed := strings.TrimSpace(prefix)

	// Find the last occurrence of the method call pattern
	patterns := []string{"->" + method + "(", "::" + method + "("}
	bestIdx := -1
	bestOp := ""
	for _, pat := range patterns {
		idx := strings.LastIndex(trimmed, pat)
		if idx > bestIdx {
			bestIdx = idx
			if strings.HasPrefix(pat, "::") {
				bestOp = "::"
			} else {
				bestOp = "->"
			}
		}
	}
	if bestIdx < 0 {
		return ""
	}

	// Get the expression before the method call
	beforeMethod := trimmed[:bestIdx]

	// Resolve the type of the expression before the method call
	var typeFQN string
	if bestOp == "::" {
		// Static call: Category::where( — resolve the class directly
		// Extract the class name right before ::
		end := len(beforeMethod)
		i := end - 1
		for i >= 0 && (resolve.IsWordChar(beforeMethod[i]) || beforeMethod[i] == '\\') {
			i--
		}
		className := beforeMethod[i+1 : end]
		if className == "" {
			return ""
		}
		typeFQN = p.resolveClassNameFromSource(className, source, file)
	} else {
		// Instance/chain call: Category::query()->where( or $query->where(
		// Use resolveAccessChain to walk the chain
		// We need to create a dummy line that ends at the -> before our method
		chainLine := beforeMethod + "->__dummy__"
		wordStart := len(beforeMethod) + 2
		typeFQN = p.resolveAccessChain(chainLine, wordStart, source, pos, file)
	}

	if typeFQN == "" {
		return ""
	}

	// If the type is already a Model subclass, use it directly
	if p.isEloquentModel(typeFQN) {
		return typeFQN
	}

	// If the type is Builder, try to find the model from the chain origin
	// For now, walk the chain backward looking for a Model class
	if typeFQN == "Illuminate\\Database\\Eloquent\\Builder" {
		// Try resolving the full chain to find the originating model
		// The static call origin (e.g., Category::query()) tells us the model
		if bestOp == "->" {
			return p.findModelInChain(beforeMethod, source, pos, file)
		}
	}

	return ""
}

// isEloquentModel checks if the FQN is an Eloquent Model subclass.
func (p *Provider) isEloquentModel(fqn string) bool {
	chain := p.index.GetInheritanceChain(fqn)
	for _, parent := range chain {
		if parent == "Illuminate\\Database\\Eloquent\\Model" {
			return true
		}
	}
	return false
}

// findModelInChain walks backward through a chain expression to find
// the originating Model class. For "Category::query()" it returns
// the FQN for Category.
func (p *Provider) findModelInChain(expr, source string, pos protocol.Position, file *parser.FileNode) string {
	trimmed := strings.TrimSpace(expr)

	// Look for ClassName:: pattern at the start of the chain
	if idx := strings.Index(trimmed, "::"); idx > 0 {
		className := trimmed[:idx]
		// Strip any prefix before the class name (e.g., whitespace, assignment)
		for i := len(className) - 1; i >= 0; i-- {
			if !resolve.IsWordChar(className[i]) && className[i] != '\\' {
				className = className[i+1:]
				break
			}
		}
		if className != "" {
			fqn := p.resolveClassNameFromSource(className, source, file)
			if p.isEloquentModel(fqn) {
				return fqn
			}
		}
	}

	// Try resolving variables: $query = Category::query()
	// The chain resolver handles this via variable type resolution
	if strings.HasPrefix(trimmed, "$") || strings.Contains(trimmed, "->") {
		resolved := p.ResolveExpressionType(trimmed, source, pos, file)
		if resolved != "" && p.isEloquentModel(resolved) {
			return resolved
		}
	}

	return ""
}

// isDBColumn returns true if the symbol represents an actual database column
// (from IDE helper @property, migrations, database introspection, or Doctrine).
func isDBColumn(m *symbols.Symbol) bool {
	if m.Kind != symbols.KindProperty {
		return false
	}
	if !m.IsVirtual {
		return false
	}
	doc := m.DocComment
	// Migration-derived
	if strings.HasPrefix(doc, "From migration") {
		return true
	}
	// Database introspection
	if strings.HasPrefix(doc, "(database column)") {
		return true
	}
	// Doctrine ORM
	if doc == "Doctrine column" || doc == "Doctrine ID column" {
		return true
	}
	// Skip relations and accessors — everything else from @property is a column
	if strings.HasSuffix(doc, " relation") {
		return false
	}
	if strings.HasPrefix(doc, "Accessor from ") {
		return false
	}
	// IDE helper @property and other docblock-derived properties
	// These are virtual properties from class docblocks, typically DB columns
	return true
}

// completeBuilderDBColumns returns completion items for strict database column
// names only (IDE helper, migrations, database introspection, Doctrine).
func (p *Provider) completeBuilderDBColumns(modelFQN, partial, quote string) []protocol.CompletionItem {
	members := p.index.GetClassMembers(modelFQN)
	lpartial := strings.ToLower(partial)

	q := "'"
	if quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	for _, m := range members {
		if !isDBColumn(m) {
			continue
		}

		colName := strings.TrimPrefix(m.Name, "$")
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(colName), lpartial) {
			continue
		}

		insertText := colName
		if quote == "" {
			insertText = q + colName + q
		}

		items = append(items, protocol.CompletionItem{
			Label:      colName,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     m.Type,
			InsertText: insertText,
			SortText:   "0" + colName,
		})
	}
	return items
}

// completeBuilderColumns returns completion items for column names on the given model.
func (p *Provider) completeBuilderColumns(modelFQN, partial, quote string) []protocol.CompletionItem {
	members := p.index.GetClassMembers(modelFQN)
	lpartial := strings.ToLower(partial)

	q := "'"
	if quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	for _, m := range members {
		if m.Kind != symbols.KindProperty {
			continue
		}
		// Skip relation-derived virtual properties (they appear as columns otherwise)
		if m.IsVirtual && strings.HasSuffix(m.DocComment, " relation") {
			continue
		}

		colName := strings.TrimPrefix(m.Name, "$")
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(colName), lpartial) {
			continue
		}

		insertText := colName
		if quote == "" {
			insertText = q + colName + q
		}

		items = append(items, protocol.CompletionItem{
			Label:      colName,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     m.Type,
			InsertText: insertText,
			SortText:   "0" + colName,
		})
	}
	return items
}

// completeBuilderRelations returns completion items for relation names on the given model.
func (p *Provider) completeBuilderRelations(modelFQN, partial, quote string) []protocol.CompletionItem {
	members := p.index.GetClassMembers(modelFQN)
	lpartial := strings.ToLower(partial)

	q := "'"
	if quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	for _, m := range members {
		if m.Kind != symbols.KindMethod {
			continue
		}

		// Check if the return type is a relation class
		retType := m.ReturnType
		if retType == "" {
			continue
		}
		shortRet := shortClassName(retType)
		if !relationReturnTypes[shortRet] {
			continue
		}

		relName := m.Name
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(relName), lpartial) {
			continue
		}

		insertText := relName
		if quote == "" {
			insertText = q + relName + q
		}

		items = append(items, protocol.CompletionItem{
			Label:      relName,
			Kind:       protocol.CompletionItemKindMethod,
			Detail:     shortRet + " relation",
			InsertText: insertText,
			SortText:   "0" + relName,
		})
	}
	return items
}

// shortClassName extracts the short class name from a potentially qualified name.
func shortClassName(name string) string {
	if i := strings.LastIndex(name, "\\"); i >= 0 {
		return name[i+1:]
	}
	return name
}
