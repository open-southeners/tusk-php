package symbols

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
)

type SymbolKind int

const (
	KindClass SymbolKind = iota
	KindInterface
	KindTrait
	KindEnum
	KindFunction
	KindMethod
	KindProperty
	KindConstant
	KindVariable
	KindEnumCase
	KindNamespace
)

type SymbolSource int

const (
	SourceProject SymbolSource = iota
	SourceBuiltin
	SourceVendor
)

type Symbol struct {
	Name       string
	FQN        string
	Kind       SymbolKind
	Source     SymbolSource
	URI        string
	Range      protocol.Range
	Visibility string
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	Type       string
	DocComment string
	ParentFQN  string
	Params     []ParamInfo
	ReturnType string
	Children   []*Symbol
	Implements []string
	Extends    string
	BackedType string
	Value      string
}

type ParamInfo struct {
	Name         string
	Type         string
	DefaultValue string
	IsVariadic   bool
	IsReference  bool
}

type Index struct {
	mu                    sync.RWMutex
	symbols               map[string]*Symbol
	nameIndex             map[string][]string
	fileSymbols           map[string][]*Symbol
	namespaceIndex        map[string][]string
	inheritanceMap        map[string]string
	implementsMap         map[string][]string   // class → interfaces it implements
	reverseImplementsMap  map[string][]string   // interface → classes that implement it
	traitMap              map[string][]string
}

func NewIndex() *Index {
	return &Index{
		symbols:              make(map[string]*Symbol),
		nameIndex:            make(map[string][]string),
		fileSymbols:          make(map[string][]*Symbol),
		namespaceIndex:       make(map[string][]string),
		inheritanceMap:       make(map[string]string),
		implementsMap:        make(map[string][]string),
		reverseImplementsMap: make(map[string][]string),
		traitMap:             make(map[string][]string),
	}
}

func (idx *Index) IndexFile(uri string, source string) {
	idx.IndexFileWithSource(uri, source, SourceProject)
}

func (idx *Index) IndexFileWithSource(uri string, source string, src SymbolSource) {
	file := parser.ParseFile(source)
	if file == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeFileSymbols(uri)
	// Ensure the file is tracked even if it has no symbol declarations
	if _, ok := idx.fileSymbols[uri]; !ok {
		idx.fileSymbols[uri] = nil
	}
	ns := file.Namespace

	resolve := func(name string) string {
		return resolveTypeName(name, ns, file.Uses)
	}

	for _, c := range file.Classes {
		fqn := buildFQN(ns, c.Name)
		var resolvedImpls []string
		for _, impl := range c.Implements {
			resolvedImpls = append(resolvedImpls, resolve(impl))
		}
		sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: c.DocComment,
			IsAbstract: c.IsAbstract, IsFinal: c.IsFinal, IsReadonly: c.IsReadonly,
			Implements: resolvedImpls,
			Range: symRange(c.StartLine, c.StartCol, len(c.Name))}
		if c.Extends != "" {
			sym.Extends = resolve(c.Extends)
		}
		idx.addSymbolWithSource(uri, sym, src)
		if c.Extends != "" {
			idx.inheritanceMap[fqn] = sym.Extends
		}
		for _, impl := range resolvedImpls {
			idx.implementsMap[fqn] = append(idx.implementsMap[fqn], impl)
			idx.reverseImplementsMap[impl] = appendUnique(idx.reverseImplementsMap[impl], fqn)
		}
		for _, tr := range c.Traits {
			idx.traitMap[fqn] = append(idx.traitMap[fqn], resolve(tr))
		}
		for _, prop := range c.Properties {
			ps := &Symbol{Name: prop.Name, FQN: fqn + "::" + prop.Name, Kind: KindProperty, URI: uri,
				Visibility: prop.Visibility, IsStatic: prop.IsStatic, Type: resolve(prop.Type.Name),
				DocComment: prop.DocComment, ParentFQN: fqn,
				Range: symRange(prop.StartLine, prop.StartCol, len(prop.Name))}
			sym.Children = append(sym.Children, ps)
			idx.addSymbolWithSource(uri, ps, src)
		}
		for _, m := range c.Methods {
			ms := &Symbol{Name: m.Name, FQN: fqn + "::" + m.Name, Kind: KindMethod, URI: uri,
				Visibility: m.Visibility, IsStatic: m.IsStatic, IsAbstract: m.IsAbstract, IsFinal: m.IsFinal,
				ReturnType: resolve(m.ReturnType.Name),
				DocComment: m.DocComment, ParentFQN: fqn,
				Range: symRange(m.StartLine, m.StartCol, len(m.Name))}
			for _, p := range m.Params {
				ms.Params = append(ms.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), DefaultValue: p.DefaultValue, IsVariadic: p.IsVariadic, IsReference: p.IsReference})
			}
			sym.Children = append(sym.Children, ms)
			idx.addSymbolWithSource(uri, ms, src)
		}
		for _, co := range c.Constants {
			cs := &Symbol{Name: co.Name, FQN: fqn + "::" + co.Name, Kind: KindConstant, URI: uri, ParentFQN: fqn,
				Value: co.Value,
				Range: symRange(co.StartLine, co.StartCol, len(co.Name))}
			sym.Children = append(sym.Children, cs)
			idx.addSymbolWithSource(uri, cs, src)
		}
	}

	for _, iface := range file.Interfaces {
		fqn := buildFQN(ns, iface.Name)
		sym := &Symbol{Name: iface.Name, FQN: fqn, Kind: KindInterface, URI: uri, DocComment: iface.DocComment,
			Range: symRange(iface.StartLine, iface.StartCol, len(iface.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		for _, m := range iface.Methods {
			ms := &Symbol{Name: m.Name, FQN: fqn + "::" + m.Name, Kind: KindMethod, URI: uri,
				Visibility: m.Visibility, ReturnType: resolve(m.ReturnType.Name), DocComment: m.DocComment, ParentFQN: fqn,
				Range: symRange(m.StartLine, m.StartCol, len(m.Name))}
			for _, p := range m.Params {
				ms.Params = append(ms.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), IsVariadic: p.IsVariadic, IsReference: p.IsReference})
			}
			sym.Children = append(sym.Children, ms)
			idx.addSymbolWithSource(uri, ms, src)
		}
	}

	for _, tr := range file.Traits {
		fqn := buildFQN(ns, tr.Name)
		sym := &Symbol{Name: tr.Name, FQN: fqn, Kind: KindTrait, URI: uri, DocComment: tr.DocComment,
			Range: symRange(tr.StartLine, tr.StartCol, len(tr.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		for _, prop := range tr.Properties {
			ps := &Symbol{Name: prop.Name, FQN: fqn + "::" + prop.Name, Kind: KindProperty, URI: uri,
				Visibility: prop.Visibility, IsStatic: prop.IsStatic, Type: resolve(prop.Type.Name),
				DocComment: prop.DocComment, ParentFQN: fqn,
				Range: symRange(prop.StartLine, prop.StartCol, len(prop.Name))}
			sym.Children = append(sym.Children, ps)
			idx.addSymbolWithSource(uri, ps, src)
		}
		for _, m := range tr.Methods {
			ms := &Symbol{Name: m.Name, FQN: fqn + "::" + m.Name, Kind: KindMethod, URI: uri,
				Visibility: m.Visibility, IsStatic: m.IsStatic, ReturnType: resolve(m.ReturnType.Name),
				DocComment: m.DocComment, ParentFQN: fqn,
				Range: symRange(m.StartLine, m.StartCol, len(m.Name))}
			for _, p := range m.Params {
				ms.Params = append(ms.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), IsVariadic: p.IsVariadic, IsReference: p.IsReference})
			}
			sym.Children = append(sym.Children, ms)
			idx.addSymbolWithSource(uri, ms, src)
		}
	}

	for _, en := range file.Enums {
		fqn := buildFQN(ns, en.Name)
		var resolvedEnumImpls []string
		for _, impl := range en.Implements {
			resolvedEnumImpls = append(resolvedEnumImpls, resolve(impl))
		}
		sym := &Symbol{Name: en.Name, FQN: fqn, Kind: KindEnum, URI: uri, DocComment: en.DocComment,
			BackedType: en.BackedType, Implements: resolvedEnumImpls,
			Range: symRange(en.StartLine, en.StartCol, len(en.Name))}
		idx.addSymbolWithSource(uri, sym, src)
		for _, impl := range resolvedEnumImpls {
			idx.implementsMap[fqn] = append(idx.implementsMap[fqn], impl)
		}
		for _, ec := range en.Cases {
			cs := &Symbol{Name: ec.Name, FQN: fqn + "::" + ec.Name, Kind: KindEnumCase, URI: uri, ParentFQN: fqn, Value: ec.Value,
				Range: symRange(ec.StartLine, ec.StartCol, len(ec.Name))}
			sym.Children = append(sym.Children, cs)
			idx.addSymbolWithSource(uri, cs, src)
		}
		for _, m := range en.Methods {
			ms := &Symbol{Name: m.Name, FQN: fqn + "::" + m.Name, Kind: KindMethod, URI: uri,
				Visibility: m.Visibility, IsStatic: m.IsStatic, ReturnType: resolve(m.ReturnType.Name),
				DocComment: m.DocComment, ParentFQN: fqn,
				Range: symRange(m.StartLine, m.StartCol, len(m.Name))}
			for _, p := range m.Params {
				ms.Params = append(ms.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), IsVariadic: p.IsVariadic, IsReference: p.IsReference})
			}
			sym.Children = append(sym.Children, ms)
			idx.addSymbolWithSource(uri, ms, src)
		}
	}

	for _, fn := range file.Functions {
		fqn := buildFQN(ns, fn.Name)
		sym := &Symbol{Name: fn.Name, FQN: fqn, Kind: KindFunction, URI: uri, ReturnType: resolve(fn.ReturnType.Name),
			DocComment: fn.DocComment, Range: symRange(fn.StartLine, fn.StartCol, len(fn.Name))}
		for _, p := range fn.Params {
			sym.Params = append(sym.Params, ParamInfo{Name: p.Name, Type: resolve(p.Type.Name), IsVariadic: p.IsVariadic, IsReference: p.IsReference})
		}
		idx.addSymbolWithSource(uri, sym, src)
	}
}

func (idx *Index) addSymbol(uri string, sym *Symbol) {
	idx.symbols[sym.FQN] = sym
	idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	idx.fileSymbols[uri] = append(idx.fileSymbols[uri], sym)
	// Index top-level symbols by their namespace
	if sym.Kind != KindMethod && sym.Kind != KindProperty && sym.Kind != KindConstant && sym.Kind != KindEnumCase {
		ns := ""
		if i := strings.LastIndex(sym.FQN, "\\"); i >= 0 {
			ns = sym.FQN[:i]
		}
		idx.namespaceIndex[ns] = appendUnique(idx.namespaceIndex[ns], sym.FQN)
	}
}

func (idx *Index) addSymbolWithSource(uri string, sym *Symbol, src SymbolSource) {
	sym.Source = src
	idx.addSymbol(uri, sym)
}

func (idx *Index) removeFileSymbols(uri string) {
	for _, sym := range idx.fileSymbols[uri] {
		delete(idx.symbols, sym.FQN)
		if fqns, ok := idx.nameIndex[sym.Name]; ok {
			idx.nameIndex[sym.Name] = removeFromSlice(fqns, sym.FQN)
		}
		// Clean up namespace index
		if sym.Kind != KindMethod && sym.Kind != KindProperty && sym.Kind != KindConstant && sym.Kind != KindEnumCase {
			ns := ""
			if i := strings.LastIndex(sym.FQN, "\\"); i >= 0 {
				ns = sym.FQN[:i]
			}
			if fqns, ok := idx.namespaceIndex[ns]; ok {
				idx.namespaceIndex[ns] = removeFromSlice(fqns, sym.FQN)
			}
		}
		// Clean up reverse implements map
		if ifaces, ok := idx.implementsMap[sym.FQN]; ok {
			for _, iface := range ifaces {
				idx.reverseImplementsMap[iface] = removeFromSlice(idx.reverseImplementsMap[iface], sym.FQN)
			}
		}
		delete(idx.implementsMap, sym.FQN)
		delete(idx.inheritanceMap, sym.FQN)
	}
	delete(idx.fileSymbols, uri)
}

func (idx *Index) Lookup(fqn string) *Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.symbols[fqn]
}

func (idx *Index) LookupByName(name string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	for _, fqn := range idx.nameIndex[name] {
		if sym, ok := idx.symbols[fqn]; ok {
			results = append(results, sym)
		}
	}
	return results
}

func (idx *Index) SearchByPrefix(prefix string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	lp := strings.ToLower(prefix)
	for name, fqns := range idx.nameIndex {
		if prefix == "" || strings.HasPrefix(strings.ToLower(name), lp) {
			for _, fqn := range fqns {
				if sym, ok := idx.symbols[fqn]; ok {
					results = append(results, sym)
				}
			}
		}
	}
	return results
}

// SearchByFQNPrefix returns symbols whose FQN starts with the given prefix,
// plus unique namespace segments at the next level for progressive completion.
// For example, prefix "Illuminate\" returns both direct symbols in that namespace
// and child namespace names like "Foundation", "Http", "Support", etc.
func (idx *Index) SearchByFQNPrefix(prefix string) ([]*Symbol, []string) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var syms []*Symbol
	nsSeen := make(map[string]bool)
	var nsSegments []string
	lp := strings.ToLower(prefix)

	for fqn, sym := range idx.symbols {
		// Skip children (methods, properties, constants, enum cases)
		if sym.Kind == KindMethod || sym.Kind == KindProperty || sym.Kind == KindConstant || sym.Kind == KindEnumCase {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(fqn), lp) {
			continue
		}
		rest := fqn[len(prefix):]
		if sepIdx := strings.Index(rest, "\\"); sepIdx >= 0 {
			// This symbol is in a deeper namespace — extract the next segment
			seg := rest[:sepIdx]
			if seg != "" && !nsSeen[seg] {
				nsSeen[seg] = true
				nsSegments = append(nsSegments, seg)
			}
		} else {
			// Direct member of this namespace prefix
			syms = append(syms, sym)
		}
	}
	return syms, nsSegments
}

func (idx *Index) GetFileSymbols(uri string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.fileSymbols[uri]
}

func (idx *Index) GetClassMembers(classFQN string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.getClassMembersRecursive(classFQN, make(map[string]bool))
}

func (idx *Index) getClassMembersRecursive(fqn string, visited map[string]bool) []*Symbol {
	if visited[fqn] {
		return nil
	}
	visited[fqn] = true
	var members []*Symbol
	if sym, ok := idx.symbols[fqn]; ok {
		members = append(members, sym.Children...)
	}
	if parent, ok := idx.inheritanceMap[fqn]; ok {
		members = append(members, idx.getClassMembersRecursive(parent, visited)...)
	}
	if traits, ok := idx.traitMap[fqn]; ok {
		for _, tr := range traits {
			members = append(members, idx.getClassMembersRecursive(tr, visited)...)
		}
	}
	return members
}

func (idx *Index) GetInheritanceChain(classFQN string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var chain []string
	visited := make(map[string]bool)
	current := classFQN
	for {
		if visited[current] {
			break
		}
		visited[current] = true
		parent, ok := idx.inheritanceMap[current]
		if !ok || parent == "" {
			break
		}
		chain = append(chain, parent)
		current = parent
	}
	return chain
}

func (idx *Index) GetImplementors(ifaceFQN string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	for _, classFQN := range idx.reverseImplementsMap[ifaceFQN] {
		if sym, ok := idx.symbols[classFQN]; ok {
			results = append(results, sym)
		}
	}
	return results
}

func (idx *Index) GetImplementedInterfaces(classFQN string) []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.implementsMap[classFQN]
}

func (idx *Index) GetNamespaceMembers(ns string) []*Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var results []*Symbol
	for _, fqn := range idx.namespaceIndex[ns] {
		if sym, ok := idx.symbols[fqn]; ok {
			results = append(results, sym)
		}
	}
	return results
}

// RegisterBuiltins populates the index with PHP built-in symbols.
func (idx *Index) RegisterBuiltins() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	builtins := []struct {
		Name   string
		Params []ParamInfo
		Ret    string
		Doc    string
	}{
		{"array_map", []ParamInfo{{Name: "$callback", Type: "?callable"}, {Name: "$array", Type: "array"}}, "array", "Applies callback to elements"},
		{"array_filter", []ParamInfo{{Name: "$array", Type: "array"}, {Name: "$callback", Type: "?callable"}}, "array", "Filters elements using callback"},
		{"array_reduce", []ParamInfo{{Name: "$array", Type: "array"}, {Name: "$callback", Type: "callable"}, {Name: "$initial", Type: "mixed"}}, "mixed", "Reduces array to single value"},
		{"array_keys", []ParamInfo{{Name: "$array", Type: "array"}}, "array", "Return all keys of an array"},
		{"array_values", []ParamInfo{{Name: "$array", Type: "array"}}, "array", "Return all values of an array"},
		{"array_merge", []ParamInfo{{Name: "$arrays", Type: "array", IsVariadic: true}}, "array", "Merge arrays"},
		{"array_push", []ParamInfo{{Name: "$array", Type: "array", IsReference: true}, {Name: "$values", Type: "mixed", IsVariadic: true}}, "int", "Push onto end of array"},
		{"array_pop", []ParamInfo{{Name: "$array", Type: "array", IsReference: true}}, "mixed", "Pop last element"},
		{"array_unique", []ParamInfo{{Name: "$array", Type: "array"}}, "array", "Remove duplicates"},
		{"array_search", []ParamInfo{{Name: "$needle", Type: "mixed"}, {Name: "$haystack", Type: "array"}}, "int|string|false", "Search for value"},
		{"in_array", []ParamInfo{{Name: "$needle", Type: "mixed"}, {Name: "$haystack", Type: "array"}}, "bool", "Check if value exists in array"},
		{"count", []ParamInfo{{Name: "$value", Type: "Countable|array"}}, "int", "Count elements"},
		{"isset", []ParamInfo{{Name: "$var", Type: "mixed"}}, "bool", "Check if variable is set"},
		{"empty", []ParamInfo{{Name: "$var", Type: "mixed"}}, "bool", "Check if variable is empty"},
		{"strlen", []ParamInfo{{Name: "$string", Type: "string"}}, "int", "Get string length"},
		{"str_contains", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "bool", "Check if string contains substring"},
		{"str_starts_with", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "bool", "Check if string starts with"},
		{"str_ends_with", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "bool", "Check if string ends with"},
		{"substr", []ParamInfo{{Name: "$string", Type: "string"}, {Name: "$offset", Type: "int"}}, "string", "Return part of string"},
		{"strpos", []ParamInfo{{Name: "$haystack", Type: "string"}, {Name: "$needle", Type: "string"}}, "int|false", "Find position of substring"},
		{"str_replace", []ParamInfo{{Name: "$search", Type: "string|array"}, {Name: "$replace", Type: "string|array"}, {Name: "$subject", Type: "string|array"}}, "string|array", "Replace occurrences"},
		{"explode", []ParamInfo{{Name: "$separator", Type: "string"}, {Name: "$string", Type: "string"}}, "array", "Split string by separator"},
		{"implode", []ParamInfo{{Name: "$separator", Type: "string"}, {Name: "$array", Type: "array"}}, "string", "Join array elements"},
		{"sprintf", []ParamInfo{{Name: "$format", Type: "string"}, {Name: "$values", Type: "mixed", IsVariadic: true}}, "string", "Return formatted string"},
		{"json_encode", []ParamInfo{{Name: "$value", Type: "mixed"}}, "string|false", "JSON encode a value"},
		{"json_decode", []ParamInfo{{Name: "$json", Type: "string"}, {Name: "$associative", Type: "?bool"}}, "mixed", "Decode JSON string"},
		{"file_get_contents", []ParamInfo{{Name: "$filename", Type: "string"}}, "string|false", "Read entire file"},
		{"file_put_contents", []ParamInfo{{Name: "$filename", Type: "string"}, {Name: "$data", Type: "mixed"}}, "int|false", "Write data to file"},
		{"file_exists", []ParamInfo{{Name: "$filename", Type: "string"}}, "bool", "Check if file exists"},
		{"var_dump", []ParamInfo{{Name: "$value", Type: "mixed", IsVariadic: true}}, "void", "Dump variable info"},
		{"print_r", []ParamInfo{{Name: "$value", Type: "mixed"}, {Name: "$return", Type: "bool"}}, "string|true", "Print human-readable info"},
		{"class_exists", []ParamInfo{{Name: "$class", Type: "string"}}, "bool", "Check if class is defined"},
		{"preg_match", []ParamInfo{{Name: "$pattern", Type: "string"}, {Name: "$subject", Type: "string"}, {Name: "$matches", Type: "array", IsReference: true}}, "int|false", "Perform regex match"},
		{"abs", []ParamInfo{{Name: "$num", Type: "int|float"}}, "int|float", "Absolute value"},
		{"round", []ParamInfo{{Name: "$num", Type: "int|float"}, {Name: "$precision", Type: "int"}}, "float", "Round a float"},
		{"max", []ParamInfo{{Name: "$value", Type: "mixed", IsVariadic: true}}, "mixed", "Find highest value"},
		{"min", []ParamInfo{{Name: "$value", Type: "mixed", IsVariadic: true}}, "mixed", "Find lowest value"},
		{"time", nil, "int", "Return current Unix timestamp"},
		{"date", []ParamInfo{{Name: "$format", Type: "string"}, {Name: "$timestamp", Type: "?int"}}, "string", "Format a local time/date"},
		{"is_string", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is string"},
		{"is_int", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is int"},
		{"is_array", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if type is array"},
		{"is_null", []ParamInfo{{Name: "$value", Type: "mixed"}}, "bool", "Check if variable is null"},
		{"intval", []ParamInfo{{Name: "$value", Type: "mixed"}}, "int", "Get integer value"},
		{"strval", []ParamInfo{{Name: "$value", Type: "mixed"}}, "string", "Get string value"},
		{"trim", []ParamInfo{{Name: "$string", Type: "string"}}, "string", "Strip whitespace"},
		{"strtolower", []ParamInfo{{Name: "$string", Type: "string"}}, "string", "Make lowercase"},
		{"strtoupper", []ParamInfo{{Name: "$string", Type: "string"}}, "string", "Make uppercase"},
	}

	for _, fn := range builtins {
		sym := &Symbol{Name: fn.Name, FQN: fn.Name, Kind: KindFunction, Source: SourceBuiltin, URI: "builtin", ReturnType: fn.Ret, DocComment: fn.Doc, Params: fn.Params}
		idx.symbols[sym.FQN] = sym
		idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	}

	builtinClasses := []struct{ Name, Doc string }{
		{"stdClass", "Generic empty class"},
		{"Exception", "Base class for all exceptions"},
		{"DateTime", "Representation of date and time"},
		{"DateTimeImmutable", "Immutable date and time"},
		{"Fiber", "Full-stack interruptible functions (PHP 8.1+)"},
		{"WeakMap", "Maps objects as keys to arbitrary values"},
		{"Generator", "Generator objects returned from generators"},
		{"Closure", "Class used to represent anonymous functions"},
		{"ArrayObject", "Allows objects to work as arrays"},
	}
	for _, cls := range builtinClasses {
		sym := &Symbol{Name: cls.Name, FQN: cls.Name, Kind: KindClass, Source: SourceBuiltin, URI: "builtin", DocComment: cls.Doc}
		idx.symbols[sym.FQN] = sym
		idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	}
}

func symRange(line, col, nameLen int) protocol.Range {
	start := protocol.Position{Line: line, Character: col}
	end := protocol.Position{Line: line, Character: col + nameLen}
	return protocol.Range{Start: start, End: end}
}

func buildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

var phpBuiltinTypes = map[string]bool{
	"string": true, "int": true, "float": true, "bool": true, "array": true,
	"object": true, "callable": true, "iterable": true, "void": true, "never": true,
	"null": true, "false": true, "true": true, "mixed": true, "self": true,
	"static": true, "parent": true, "resource": true,
}

// IsPHPBuiltinType returns true if the name is a PHP primitive/built-in type.
func IsPHPBuiltinType(name string) bool {
	return phpBuiltinTypes[strings.ToLower(name)]
}

func resolveTypeName(name string, currentNs string, uses []parser.UseNode) string {
	if name == "" {
		return ""
	}
	// Strip nullable prefix for resolution, then re-add
	prefix := ""
	if strings.HasPrefix(name, "?") {
		prefix = "?"
		name = name[1:]
	}
	// Handle union/intersection types
	if strings.ContainsAny(name, "|&") {
		var parts []string
		for _, part := range splitTypeExpr(name) {
			parts = append(parts, resolveTypeName(part, currentNs, uses))
		}
		return prefix + strings.Join(parts, "|")
	}
	// Built-in types are never namespace-qualified
	if phpBuiltinTypes[strings.ToLower(name)] {
		return prefix + name
	}
	if strings.HasPrefix(name, "\\") {
		return prefix + strings.TrimPrefix(name, "\\")
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return prefix + u.FullName + "\\" + parts[1]
			}
			return prefix + u.FullName
		}
	}
	if currentNs != "" {
		return prefix + currentNs + "\\" + name
	}
	return prefix + name
}

func splitTypeExpr(name string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(name); i++ {
		if name[i] == '|' || name[i] == '&' {
			if i > start {
				parts = append(parts, name[start:i])
			}
			start = i + 1
		}
	}
	if start < len(name) {
		parts = append(parts, name[start:])
	}
	return parts
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func removeFromSlice(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}

// GetAllFileURIs returns all file URIs that have been indexed.
func (idx *Index) GetAllFileURIs() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	uris := make([]string, 0, len(idx.fileSymbols))
	for uri := range idx.fileSymbols {
		uris = append(uris, uri)
	}
	return uris
}

func URIToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	return filepath.Clean(path)
}

// PickBestStandalone selects the most appropriate symbol when multiple symbols
// share the same short name and the word appears in standalone context (not
// after -> or ::). Prefers functions over classes over constants over
// methods/properties. Among equally-ranked kinds, prefers an exact case match.
func PickBestStandalone(syms []*Symbol, word string) *Symbol {
	var best *Symbol
	bestRank := 999
	bestExact := false

	for _, s := range syms {
		r := standaloneRank(s)
		exact := s.Name == word
		if r < bestRank || (r == bestRank && exact && !bestExact) {
			best = s
			bestRank = r
			bestExact = exact
		}
	}
	return best
}

func standaloneRank(s *Symbol) int {
	switch s.Kind {
	case KindFunction:
		return 0
	case KindClass, KindInterface, KindEnum, KindTrait:
		return 1
	case KindConstant, KindEnumCase:
		return 2
	default: // KindMethod, KindProperty
		return 3
	}
}
