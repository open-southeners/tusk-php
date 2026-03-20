package models

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
	"github.com/open-southeners/php-lsp/internal/types"
)

// FrameworkArrayResolver provides array key suggestions for framework-specific
// patterns like Laravel config(), $request->validated(), $model->toArray().
type FrameworkArrayResolver struct {
	index     *symbols.Index
	rootPath  string
	framework string
}

// NewFrameworkArrayResolver creates a resolver for the given framework.
func NewFrameworkArrayResolver(index *symbols.Index, rootPath, framework string) *FrameworkArrayResolver {
	return &FrameworkArrayResolver{index: index, rootPath: rootPath, framework: framework}
}

// ResolveCallReturnKeys resolves array keys for a call expression result.
// expr is the call expression (e.g. "config('app')", "$request->validated()").
// source is the current file content for context.
func (r *FrameworkArrayResolver) ResolveCallReturnKeys(expr, source string) []types.ShapeField {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, ";")

	switch r.framework {
	case "laravel":
		return r.resolveLaravelCall(expr, source)
	case "symfony":
		return r.resolveSymfonyCall(expr, source)
	}
	return nil
}

// ResolveMethodReturnKeys resolves array keys for $var->method() based on the
// class type and method name.
func (r *FrameworkArrayResolver) ResolveMethodReturnKeys(classFQN, methodName string) []types.ShapeField {
	switch r.framework {
	case "laravel":
		return r.resolveLaravelMethodKeys(classFQN, methodName)
	}
	return nil
}

// --- Laravel ---

func (r *FrameworkArrayResolver) resolveLaravelCall(expr, source string) []types.ShapeField {
	// config('app') → keys from config/app.php
	if strings.HasPrefix(expr, "config(") {
		return r.resolveLaravelConfig(expr)
	}
	return nil
}

func (r *FrameworkArrayResolver) resolveLaravelConfig(expr string) []types.ShapeField {
	// Extract the config key: config('app') → "app", config('app.name') → "app"
	arg := extractFirstStringArg(expr)
	if arg == "" {
		// config() with no args — list top-level config file names
		return r.ListConfigFiles()
	}

	// Split on dots: 'database.connections.mysql' → ['database', 'connections', 'mysql']
	parts := strings.Split(arg, ".")
	configFile := parts[0]

	// Parse the config file
	keys := r.ParseConfigFile(configFile)
	if keys == nil {
		return nil
	}

	// Drill into nested keys via dot segments
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

	return keys
}

// ListConfigFiles returns all available config file names as shape fields.
func (r *FrameworkArrayResolver) ListConfigFiles() []types.ShapeField {
	configDir := filepath.Join(r.rootPath, "config")
	entries, err := os.ReadDir(configDir)
	if err != nil {
		return nil
	}
	var fields []types.ShapeField
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".php") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".php")
		fields = append(fields, types.ShapeField{Key: name, Type: "array"})
	}
	return fields
}

// ParseConfigFile parses a config file and returns its top-level shape fields.
func (r *FrameworkArrayResolver) ParseConfigFile(name string) []types.ShapeField {
	configPath := filepath.Join(r.rootPath, "config", name+".php")
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	source := string(content)
	// Config files return an array: return [ 'key' => value, ... ];
	// Find the return statement and extract keys from the array literal
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "return ") || strings.HasPrefix(trimmed, "return[") {
			// Collect the array literal from the return statement
			arrayText := collectReturnArray(lines, i)
			if arrayText != "" {
				return parseLiteralToShape(arrayText)
			}
		}
	}
	return nil
}

func (r *FrameworkArrayResolver) resolveLaravelMethodKeys(classFQN, methodName string) []types.ShapeField {
	// $request->validated() → keys from FormRequest rules()
	if methodName == "validated" || methodName == "safe" {
		return r.resolveFormRequestKeys(classFQN)
	}

	// $model->toArray() → keys from model properties
	if methodName == "toArray" {
		return r.resolveModelToArrayKeys(classFQN)
	}

	return nil
}

func (r *FrameworkArrayResolver) resolveFormRequestKeys(classFQN string) []types.ShapeField {
	// The form request class should have a rules() method returning an array
	// where keys are the field names
	members := r.index.GetClassMembers(classFQN)
	for _, m := range members {
		if m.Name == "rules" && m.Kind == symbols.KindMethod {
			// Read the source file and find rules() return array
			if m.URI == "" || m.URI == "builtin" {
				continue
			}
			path := strings.TrimPrefix(m.URI, "file://")
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			return extractRulesKeys(string(content))
		}
	}

	// Walk inheritance chain
	chain := r.index.GetInheritanceChain(classFQN)
	for _, parent := range chain {
		members := r.index.GetClassMembers(parent)
		for _, m := range members {
			if m.Name == "rules" && m.Kind == symbols.KindMethod {
				if m.URI == "" || m.URI == "builtin" {
					continue
				}
				path := strings.TrimPrefix(m.URI, "file://")
				content, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				return extractRulesKeys(string(content))
			}
		}
	}
	return nil
}

func (r *FrameworkArrayResolver) resolveModelToArrayKeys(classFQN string) []types.ShapeField {
	sym := r.index.Lookup(classFQN)
	if sym == nil {
		return nil
	}
	var fields []types.ShapeField
	seen := make(map[string]bool)

	// Collect all properties (including inherited)
	for _, member := range r.index.GetClassMembers(classFQN) {
		if member.Kind != symbols.KindProperty {
			continue
		}
		name := strings.TrimPrefix(member.Name, "$")
		if seen[name] {
			continue
		}
		seen[name] = true
		typ := member.Type
		if typ == "" {
			typ = "mixed"
		}
		fields = append(fields, types.ShapeField{Key: name, Type: typ})
	}
	return fields
}

// --- Symfony ---

func (r *FrameworkArrayResolver) resolveSymfonyCall(expr, source string) []types.ShapeField {
	// $container->getParameter('key') — we'd need to parse services.yaml parameters
	// For now, return nil (future implementation)
	return nil
}

// --- Helpers ---

func extractFirstStringArg(expr string) string {
	openParen := strings.Index(expr, "(")
	if openParen < 0 {
		return ""
	}
	closeParen := strings.LastIndex(expr, ")")
	if closeParen <= openParen {
		return ""
	}
	arg := strings.TrimSpace(expr[openParen+1 : closeParen])
	// Strip quotes
	if len(arg) >= 2 && (arg[0] == '\'' || arg[0] == '"') && arg[len(arg)-1] == arg[0] {
		return arg[1 : len(arg)-1]
	}
	return ""
}

// collectReturnArray collects an array literal from a return statement.
func collectReturnArray(lines []string, startLine int) string {
	var sb strings.Builder
	depth := 0
	started := false

	for i := startLine; i < len(lines) && i < startLine+500; i++ {
		line := lines[i]
		inString := byte(0)
		for j := 0; j < len(line); j++ {
			ch := line[j]
			if inString != 0 {
				if ch == inString && (j == 0 || line[j-1] != '\\') {
					inString = 0
				}
				if started {
					sb.WriteByte(ch)
				}
				continue
			}
			switch ch {
			case '\'', '"':
				inString = ch
				if started {
					sb.WriteByte(ch)
				}
			case '[':
				depth++
				started = true
				sb.WriteByte(ch)
			case ']':
				if started {
					depth--
					sb.WriteByte(ch)
					if depth == 0 {
						return sb.String()
					}
				}
			default:
				if started {
					sb.WriteByte(ch)
				}
			}
		}
		if started {
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}

// parseLiteralToShape parses a PHP array literal string into ShapeFields.
func parseLiteralToShape(arrayText string) []types.ShapeField {
	arrayText = strings.TrimSpace(arrayText)
	if len(arrayText) < 2 || arrayText[0] != '[' || arrayText[len(arrayText)-1] != ']' {
		return nil
	}
	return parseLiteralEntries(arrayText[1 : len(arrayText)-1])
}

func parseLiteralEntries(content string) []types.ShapeField {
	var fields []types.ShapeField
	depth := 0
	inString := byte(0)
	start := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString != 0 {
			if ch == inString && (i == 0 || content[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ',':
			if depth == 0 {
				if f := parseLiteralEntry(content[start:i]); f != nil {
					fields = append(fields, *f)
				}
				start = i + 1
			}
		}
	}
	if start < len(content) {
		if f := parseLiteralEntry(content[start:]); f != nil {
			fields = append(fields, *f)
		}
	}
	return fields
}

func parseLiteralEntry(entry string) *types.ShapeField {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil
	}
	arrowIdx := -1
	depth := 0
	inString := byte(0)
	for i := 0; i < len(entry)-1; i++ {
		ch := entry[i]
		if inString != 0 {
			if ch == inString && (i == 0 || entry[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case '=':
			if depth == 0 && i+1 < len(entry) && entry[i+1] == '>' {
				arrowIdx = i
				goto found
			}
		}
	}
found:
	if arrowIdx < 0 {
		return nil
	}
	keyPart := strings.TrimSpace(entry[:arrowIdx])
	valuePart := strings.TrimSpace(entry[arrowIdx+2:])
	if len(keyPart) >= 2 && (keyPart[0] == '\'' || keyPart[0] == '"') && keyPart[len(keyPart)-1] == keyPart[0] {
		keyPart = keyPart[1 : len(keyPart)-1]
	} else {
		return nil
	}
	valueType := inferValueType(valuePart)
	return &types.ShapeField{Key: keyPart, Type: valueType}
}

func inferValueType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ",")
	value = strings.TrimSpace(value)
	if value == "" {
		return "mixed"
	}
	if strings.HasPrefix(value, "[") {
		nested := parseLiteralToShape(value)
		if len(nested) > 0 {
			var parts []string
			for _, f := range nested {
				if f.Key != "" {
					parts = append(parts, f.Key+": "+f.Type)
				}
			}
			if len(parts) > 0 {
				return "array{" + strings.Join(parts, ", ") + "}"
			}
		}
		return "array"
	}
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
		return "string"
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	if lower == "null" {
		return "null"
	}
	if len(value) > 0 && (value[0] >= '0' && value[0] <= '9' || value[0] == '-') {
		if strings.ContainsAny(value, ".eE") {
			return "float"
		}
		return "int"
	}
	return "mixed"
}

// extractRulesKeys parses a FormRequest class source and extracts field names
// from the rules() method's return array.
func extractRulesKeys(source string) []types.ShapeField {
	file := parser.ParseFile(source)
	if file == nil {
		return nil
	}

	// Find the rules() method
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if m.Name != "rules" {
				continue
			}
			// Find the return statement in rules() body
			lines := strings.Split(source, "\n")
			inMethod := false
			depth := 0
			for i := m.StartLine; i < len(lines); i++ {
				line := lines[i]
				for _, ch := range line {
					if ch == '{' {
						depth++
						inMethod = true
					} else if ch == '}' {
						depth--
						if inMethod && depth == 0 {
							goto doneRules
						}
					}
				}
				trimmed := strings.TrimSpace(line)
				if inMethod && (strings.HasPrefix(trimmed, "return ") || strings.HasPrefix(trimmed, "return[")) {
					arrayText := collectReturnArray(lines, i)
					if arrayText != "" {
						// For rules, keys are validation field names, types are rule strings
						raw := parseLiteralToShape(arrayText)
						var fields []types.ShapeField
						for _, f := range raw {
							fields = append(fields, types.ShapeField{
								Key:  f.Key,
								Type: "mixed",
							})
						}
						return fields
					}
				}
			}
		doneRules:
		}
	}
	return nil
}
