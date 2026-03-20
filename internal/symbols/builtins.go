package symbols

type builtinFunction struct {
	Name   string
	Params []ParamInfo
	Ret    string
	Doc    string
}

var builtinFunctions = []builtinFunction{
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

var builtinClasses = []struct{ Name, Doc string }{
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

// RegisterBuiltins populates the index with PHP built-in symbols.
func (idx *Index) RegisterBuiltins() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	for _, fn := range builtinFunctions {
		sym := &Symbol{Name: fn.Name, FQN: fn.Name, Kind: KindFunction, Source: SourceBuiltin, URI: "builtin", ReturnType: fn.Ret, DocComment: fn.Doc, Params: fn.Params}
		idx.symbols[sym.FQN] = sym
		idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	}

	for _, cls := range builtinClasses {
		sym := &Symbol{Name: cls.Name, FQN: cls.Name, Kind: KindClass, Source: SourceBuiltin, URI: "builtin", DocComment: cls.Doc}
		idx.symbols[sym.FQN] = sym
		idx.nameIndex[sym.Name] = appendUnique(idx.nameIndex[sym.Name], sym.FQN)
	}
}
