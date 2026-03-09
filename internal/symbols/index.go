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

type Symbol struct {
	Name       string
	FQN        string
	Kind       SymbolKind
	URI        string
	Range      protocol.Range
	Visibility string
	IsStatic   bool
	Type       string
	DocComment string
	ParentFQN  string
	Params     []ParamInfo
	ReturnType string
	Children   []*Symbol
}

type ParamInfo struct {
	Name         string
	Type         string
	DefaultValue string
	IsVariadic   bool
	IsReference  bool
}

type Index struct {
	mu             sync.RWMutex
	symbols        map[string]*Symbol
	nameIndex      map[string][]string
	fileSymbols    map[string][]*Symbol
	namespaceIndex map[string][]string
	inheritanceMap map[string]string
	implementsMap  map[string][]string
	traitMap       map[string][]string
}

func NewIndex() *Index {
	return &Index{
		symbols:        make(map[string]*Symbol),
		nameIndex:      make(map[string][]string),
		fileSymbols:    make(map[string][]*Symbol),
		namespaceIndex: make(map[string][]string),
		inheritanceMap: make(map[string]string),
		implementsMap:  make(map[string][]string),
		traitMap:       make(map[string][]string),
	}
}

func (idx *Index) IndexFile(uri string, source string) {
	file := parser.ParseFile(source)
	if file == nil {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.removeFileSymbols(uri)
	ns := file.Namespace

	for _, c := range file.Classes {
		fqn := buildFQN(ns, c.Name)
		sym := &Symbol{Name: c.Name, FQN: fqn, Kind: KindClass, URI: uri, DocComment: c.DocComment,
			Range: protocol.Range{Start: protocol.Position{Line: c.StartLine, Character: c.StartCol}}}
		idx.addSymbol(uri, sym)
		if c.Extends != "" {
			idx.inheritanceMap[fqn] = resolveTypeName(c.Extends, ns, file.Uses)
		}
		for _, impl := range c.Implements {
			idx.implementsMap[fqn] = append(idx.implementsMap[fqn], resolveTypeName(impl, ns, file.Uses))
		}
		for _, tr := range c.Traits {
			idx.traitMap[fqn] = append(idx.traitMap[fqn], resolveTypeName(tr, ns, file.Uses))
		}
		for _, prop := range c.Properties {
			ps := &Symbol{Name: prop.Name, FQN: fqn + "::" + prop.Name, Kind: KindProperty, URI: uri,
				Visibility: prop.Visibility, IsStatic: prop.IsStatic, Type: prop.Type.Name,
				DocComment: prop.DocComment, ParentFQN: fqn,
				Range: protocol.Range{Start: protocol.Position{Line: prop.StartLine}}}
			sym.Children = append(sym.Children, ps)
			idx.addSymbol(uri, ps)
		}
		for _, m := range c.Methods {
			ms := &Symbol{Name: m.Name, FQN: fqn + "::" + m.Name, Kind: KindMethod, URI: uri,
				Visibility: m.Visibility, IsStatic: m.IsStatic, ReturnType: m.ReturnType.Name,
				DocComment: m.DocComment, ParentFQN: fqn,
				Range: protocol.Range{Start: protocol.Position{Line: m.StartLine}}}
			for _, p := range m.Params {
				ms.Params = append(ms.Params, ParamInfo{Name: p.Name, Type: p.Type.Name, IsVariadic: p.IsVariadic, IsReference: p.IsReference})
			}
			sym.Children = append(sym.Children, ms)
			idx.addSymbol(uri, ms)
		}
		for _, co := range c.Constants {
			cs := &Symbol{Name: co.Name, FQN: fqn + "::" + co.Name, Kind: KindConstant, URI: uri, ParentFQN: fqn,
				Range: protocol.Range{Start: protocol.Position{Line: co.StartLine}}}
			sym.Children = append(sym.Children, cs)
			idx.addSymbol(uri, cs)
		}
	}

	for _, iface := range file.Interfaces {
		fqn := buildFQN(ns, iface.Name)
		sym := &Symbol{Name: iface.Name, FQN: fqn, Kind: KindInterface, URI: uri, DocComment: iface.DocComment,
			Range: protocol.Range{Start: protocol.Position{Line: iface.StartLine}}}
		idx.addSymbol(uri, sym)
		for _, m := range iface.Methods {
			ms := &Symbol{Name: m.Name, FQN: fqn + "::" + m.Name, Kind: KindMethod, URI: uri,
				Visibility: m.Visibility, ReturnType: m.ReturnType.Name, DocComment: m.DocComment, ParentFQN: fqn}
			sym.Children = append(sym.Children, ms)
			idx.addSymbol(uri, ms)
		}
	}

	for _, tr := range file.Traits {
		fqn := buildFQN(ns, tr.Name)
		sym := &Symbol{Name: tr.Name, FQN: fqn, Kind: KindTrait, URI: uri, DocComment: tr.DocComment}
		idx.addSymbol(uri, sym)
	}

	for _, en := range file.Enums {
		fqn := buildFQN(ns, en.Name)
		sym := &Symbol{Name: en.Name, FQN: fqn, Kind: KindEnum, URI: uri, DocComment: en.DocComment}
		idx.addSymbol(uri, sym)
		for _, ec := range en.Cases {
			cs := &Symbol{Name: ec.Name, FQN: fqn + "::" + ec.Name, Kind: KindEnumCase, URI: uri, ParentFQN: fqn}
			sym.Children = append(sym.Children, cs)
			idx.addSymbol(uri, cs)
		}
	}

	for _, fn := range file.Functions {
		fqn := buildFQN(ns, fn.Name)
		sym := &Symbol{Name: fn.Name, FQN: fqn, Kind: KindFunction, URI: uri, ReturnType: fn.ReturnType.Name,
			DocComment: fn.DocComment, Range: protocol.Range{Start: protocol.Position{Line: fn.StartLine}}}
		for _, p := range fn.Params {
			sym.Params = append(sym.Params, ParamInfo{Name: p.Name, Type: p.Type.Name, IsVariadic: p.IsVariadic, IsReference: p.IsReference})
		}
		idx.addSymbol(uri, sym)
	}
}

func (idx *Index) addSymbol(uri string, sym *Symbol) {
	idx.symbols[sym.FQN] = sym
	idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	idx.fileSymbols[uri] = append(idx.fileSymbols[uri], sym)
}

func (idx *Index) removeFileSymbols(uri string) {
	for _, sym := range idx.fileSymbols[uri] {
		delete(idx.symbols, sym.FQN)
		if fqns, ok := idx.nameIndex[sym.Name]; ok {
			idx.nameIndex[sym.Name] = removeFromSlice(fqns, sym.FQN)
		}
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
	for classFQN, ifaces := range idx.implementsMap {
		for _, iface := range ifaces {
			if iface == ifaceFQN {
				if sym, ok := idx.symbols[classFQN]; ok {
					results = append(results, sym)
				}
			}
		}
	}
	return results
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
		sym := &Symbol{Name: fn.Name, FQN: fn.Name, Kind: KindFunction, URI: "builtin", ReturnType: fn.Ret, DocComment: fn.Doc, Params: fn.Params}
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
		sym := &Symbol{Name: cls.Name, FQN: cls.Name, Kind: KindClass, URI: "builtin", DocComment: cls.Doc}
		idx.symbols[sym.FQN] = sym
		idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	}
}

func buildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

func resolveTypeName(name string, currentNs string, uses []parser.UseNode) string {
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	if currentNs != "" {
		return currentNs + "\\" + name
	}
	return name
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

func URIToPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	return filepath.Clean(path)
}
