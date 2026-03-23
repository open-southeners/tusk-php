package models

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/phparray"
	"github.com/open-southeners/tusk-php/internal/symbols"
	"github.com/open-southeners/tusk-php/internal/types"
)

// FrameworkArrayResolver provides array key suggestions for framework-specific
// patterns like Laravel config(), $request->validated(), $model->toArray().
type FrameworkArrayResolver struct {
	index        *symbols.Index
	rootPath     string
	framework    string
	vendorConfigs map[string]string // config key → absolute file path (cached)
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

// ListConfigFiles returns all available config file names as shape fields,
// including vendor package configs that haven't been published yet.
func (r *FrameworkArrayResolver) ListConfigFiles() []types.ShapeField {
	seen := make(map[string]bool)
	var fields []types.ShapeField

	// 1. Project configs (highest priority)
	configDir := filepath.Join(r.rootPath, "config")
	if entries, err := os.ReadDir(configDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".php") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".php")
			seen[name] = true
			fields = append(fields, types.ShapeField{Key: name, Type: "array"})
		}
	}

	// 2. Vendor package configs
	r.ensureVendorConfigs()
	for key := range r.vendorConfigs {
		if !seen[key] {
			seen[key] = true
			fields = append(fields, types.ShapeField{Key: key, Type: "array"})
		}
	}

	return fields
}

// ParseConfigFile parses a config file and returns its top-level shape fields.
// Checks the project's config/ directory first, then falls back to vendor configs.
func (r *FrameworkArrayResolver) ParseConfigFile(name string) []types.ShapeField {
	// 1. Try project config
	configPath := filepath.Join(r.rootPath, "config", name+".php")
	if fields := parseConfigFileAt(configPath); fields != nil {
		return fields
	}

	// 2. Fall back to vendor config
	r.ensureVendorConfigs()
	if vendorPath, ok := r.vendorConfigs[name]; ok {
		return parseConfigFileAt(vendorPath)
	}

	return nil
}

// parseConfigFileAt parses a single PHP config file that returns an array.
func parseConfigFileAt(configPath string) []types.ShapeField {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}

	source := string(content)
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "return ") || strings.HasPrefix(trimmed, "return[") {
			arrayText := phparray.CollectReturnArray(lines, i)
			if arrayText != "" {
				return phparray.ParseLiteralToShape(arrayText)
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

// --- Vendor Config Discovery ---

// ensureVendorConfigs lazily discovers vendor package configs and caches them.
func (r *FrameworkArrayResolver) ensureVendorConfigs() {
	if r.vendorConfigs != nil {
		return
	}
	r.vendorConfigs = make(map[string]string)

	vendorDir := filepath.Join(r.rootPath, "vendor")
	if _, err := os.Stat(vendorDir); err != nil {
		return
	}

	// Strategy 1: Parse ServiceProvider files for mergeConfigFrom() calls.
	// This is the most accurate approach — packages declare their config key explicitly.
	r.discoverFromServiceProviders(vendorDir)

	// Strategy 2: Scan common config file locations in vendor packages.
	// Catches packages that don't use mergeConfigFrom or that we missed.
	r.discoverFromConfigDirs(vendorDir)
}

// discoverFromServiceProviders finds mergeConfigFrom() calls in vendor service providers.
// Pattern: $this->mergeConfigFrom(__DIR__.'/../config/sanctum.php', 'sanctum')
// Pattern: $this->mergeConfigFrom(path, 'key')
func (r *FrameworkArrayResolver) discoverFromServiceProviders(vendorDir string) {
	// Walk vendor looking for *ServiceProvider.php files
	filepath.Walk(vendorDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, "ServiceProvider.php") {
			return nil
		}
		// Skip test files and stubs
		if strings.Contains(path, "test") || strings.Contains(path, "stub") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		r.parseMergeConfigFrom(string(content), filepath.Dir(path))
		return nil
	})
}

// parseMergeConfigFrom extracts config key and file path from mergeConfigFrom() calls.
func (r *FrameworkArrayResolver) parseMergeConfigFrom(source, providerDir string) {
	lines := strings.Split(source, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Match: $this->mergeConfigFrom(path, 'key')
		// Match: $this->mergeConfigFrom(path, "key")
		idx := strings.Index(trimmed, "mergeConfigFrom(")
		if idx < 0 {
			continue
		}
		argsPart := trimmed[idx+len("mergeConfigFrom("):]
		closeParen := strings.Index(argsPart, ")")
		if closeParen < 0 {
			continue
		}
		args := argsPart[:closeParen]

		// Split args on comma (respecting strings)
		commaIdx := findTopLevelComma(args)
		if commaIdx < 0 {
			continue
		}

		pathArg := strings.TrimSpace(args[:commaIdx])
		keyArg := strings.TrimSpace(args[commaIdx+1:])

		// Extract the config key from the second argument
		configKey := stripQuotes(keyArg)
		if configKey == "" {
			continue
		}

		// Already have a project config for this key — skip
		projectPath := filepath.Join(r.rootPath, "config", configKey+".php")
		if _, err := os.Stat(projectPath); err == nil {
			continue
		}

		// Already discovered — skip
		if _, ok := r.vendorConfigs[configKey]; ok {
			continue
		}

		// Resolve the file path from the first argument
		configFile := resolveConfigPath(pathArg, providerDir)
		if configFile != "" {
			if _, err := os.Stat(configFile); err == nil {
				r.vendorConfigs[configKey] = configFile
			}
		}
	}
}

// discoverFromConfigDirs scans common config directories in vendor packages.
func (r *FrameworkArrayResolver) discoverFromConfigDirs(vendorDir string) {
	// Scan vendor/{org}/{package}/config/*.php
	orgDirs, _ := os.ReadDir(vendorDir)
	for _, orgDir := range orgDirs {
		if !orgDir.IsDir() || orgDir.Name() == "composer" || orgDir.Name() == "bin" {
			continue
		}
		orgPath := filepath.Join(vendorDir, orgDir.Name())
		pkgDirs, _ := os.ReadDir(orgPath)
		for _, pkgDir := range pkgDirs {
			if !pkgDir.IsDir() {
				continue
			}
			// Check config/ directory
			for _, configDirName := range []string{"config", filepath.Join("src", "config")} {
				configDir := filepath.Join(orgPath, pkgDir.Name(), configDirName)
				entries, err := os.ReadDir(configDir)
				if err != nil {
					continue
				}
				for _, entry := range entries {
					if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".php") {
						continue
					}
					key := strings.TrimSuffix(entry.Name(), ".php")
					// Don't override project configs or already-discovered keys
					projectPath := filepath.Join(r.rootPath, "config", key+".php")
					if _, err := os.Stat(projectPath); err == nil {
						continue
					}
					if _, ok := r.vendorConfigs[key]; ok {
						continue
					}
					r.vendorConfigs[key] = filepath.Join(configDir, entry.Name())
				}
			}
		}
	}
}

// InvalidateVendorCache clears the cached vendor configs, forcing re-discovery.
func (r *FrameworkArrayResolver) InvalidateVendorCache() {
	r.vendorConfigs = nil
}

// resolveConfigPath resolves a PHP path expression like __DIR__.'/../config/foo.php'
// to an absolute file path.
func resolveConfigPath(pathExpr, providerDir string) string {
	pathExpr = strings.TrimSpace(pathExpr)

	// Handle __DIR__.'/path' or __DIR__ . '/path'
	if strings.Contains(pathExpr, "__DIR__") {
		// Remove __DIR__ and the concatenation operator
		pathExpr = strings.Replace(pathExpr, "__DIR__", "", 1)
		pathExpr = strings.TrimSpace(pathExpr)
		pathExpr = strings.TrimPrefix(pathExpr, ".")
		pathExpr = strings.TrimSpace(pathExpr)
		pathExpr = stripQuotes(pathExpr)
		if pathExpr == "" {
			return ""
		}
		return filepath.Clean(filepath.Join(providerDir, pathExpr))
	}

	// Simple quoted string path
	clean := stripQuotes(pathExpr)
	if clean != "" && filepath.IsAbs(clean) {
		return clean
	}
	return ""
}

func findTopLevelComma(s string) int {
	depth := 0
	inString := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString != 0 {
			if ch == inString && (i == 0 || s[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case ',':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func stripQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return ""
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
					arrayText := phparray.CollectReturnArray(lines, i)
					if arrayText != "" {
						// For rules, keys are validation field names, types are rule strings
						raw := phparray.ParseLiteralToShape(arrayText)
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
