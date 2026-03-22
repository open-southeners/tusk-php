package checks

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

// Builder method categories — mirrors the lists in completion/columns.go.
var builderColumnMethods = map[string]bool{
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

var builderDBColumnMethods = map[string]bool{
	"get": true,
}

var builderRelationMethods = map[string]bool{
	"with": true, "without": true,
	"has": true, "doesntHave": true,
	"whereHas": true, "whereDoesntHave": true,
	"withCount": true, "withSum": true, "withAvg": true,
	"withMin": true, "withMax": true, "withExists": true,
	"load": true, "loadMissing": true, "loadCount": true,
}

// ModelResolverFunc resolves the Eloquent model FQN from a builder chain
// expression. Returns "" if the model cannot be determined.
type ModelResolverFunc func(prefix string, method string, source string, line int, file *parser.FileNode) string

// MemberChecker provides methods to verify columns and relations on a model.
type MemberChecker interface {
	IsColumn(modelFQN string, name string) bool
	IsDBColumn(modelFQN string, name string) bool
	IsRelation(modelFQN string, name string) bool
}

// InvalidBuilderArgRule detects string arguments in Builder method calls that
// reference columns or relations that don't exist on the resolved model.
type InvalidBuilderArgRule struct {
	ModelResolver ModelResolverFunc
	Members       MemberChecker
}

func (r *InvalidBuilderArgRule) Code() string { return "invalid-builder-arg" }

func (r *InvalidBuilderArgRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if r.ModelResolver == nil || r.Members == nil || file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding

	for i, line := range lines {
		findings = append(findings, r.checkLine(line, i, source, file)...)
	}
	return findings
}

// Matches ->method('arg') or ::method('arg') with a completed single-quoted string.
var directArgSingleRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*'([^']*)'`)

// Matches ->method("arg") or ::method("arg") with a completed double-quoted string.
var directArgDoubleRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*"([^"]*)"`)

// Matches string literals inside array arguments: ['val1', 'val2']
var arrayArgRe = regexp.MustCompile(`(?:->|::)(\w+)\s*\(\s*\[([^\]]*)\]`)

// Matches individual single-quoted strings.
var singleQuotedRe = regexp.MustCompile(`'([^']*)'`)

// Matches individual double-quoted strings.
var doubleQuotedRe = regexp.MustCompile(`"([^"]*)"`)


func (r *InvalidBuilderArgRule) checkLine(line string, lineNum int, source string, file *parser.FileNode) []Finding {
	var findings []Finding

	// Check direct string arguments: ->where('column_name', or ->where("column_name",
	for _, re := range []*regexp.Regexp{directArgSingleRe, directArgDoubleRe} {
		for _, match := range re.FindAllStringSubmatchIndex(line, -1) {
			method := line[match[2]:match[3]]
			argValue := line[match[4]:match[5]]
			argStart := match[4]
			argEnd := match[5]

			if f := r.validateArg(method, argValue, lineNum, argStart, argEnd, line, source, file); f != nil {
				findings = append(findings, *f)
			}
		}
	}

	// Check array arguments: ->with(['products', 'category'])
	for _, match := range arrayArgRe.FindAllStringSubmatchIndex(line, -1) {
		method := line[match[2]:match[3]]
		arrayContent := line[match[4]:match[5]]

		// Extract each quoted string from the array
		for _, re := range []*regexp.Regexp{singleQuotedRe, doubleQuotedRe} {
			for _, strMatch := range re.FindAllStringSubmatchIndex(arrayContent, -1) {
				argValue := arrayContent[strMatch[2]:strMatch[3]]
				absStart := match[4] + strMatch[2]
				absEnd := match[4] + strMatch[3]

				cleaned := stripAlias(argValue)
				if f := r.validateArg(method, cleaned, lineNum, absStart, absEnd, line, source, file); f != nil {
					findings = append(findings, *f)
				}
			}
		}
	}

	return findings
}

func (r *InvalidBuilderArgRule) validateArg(method, argValue string, lineNum, startCol, endCol int, line, source string, file *parser.FileNode) *Finding {
	if argValue == "" {
		return nil
	}

	// Strip " as alias" (e.g., "products as product_count")
	argValue = stripAlias(argValue)

	// For dot-notation in column methods (e.g., orderBy('relation.column')),
	// skip validation — it requires join context we don't have.
	isRelMethod := builderRelationMethods[method]
	isColMethod := builderColumnMethods[method]
	isDBColMethod := builderDBColumnMethods[method]

	if !isRelMethod && !isColMethod && !isDBColMethod {
		return nil
	}

	// For dot-notation, validate the first segment only
	segments := strings.SplitN(argValue, ".", 2)
	checkName := segments[0]

	// Column methods with dot-notation are join-qualified — skip
	if len(segments) > 1 && !isRelMethod {
		return nil
	}

	// Handle closure-based array syntax: 'products' => function($q) {...}
	// The regex captures 'products' before '=>', strip anything after =>
	if idx := strings.Index(checkName, " =>"); idx >= 0 {
		checkName = checkName[:idx]
	}
	checkName = strings.TrimSpace(checkName)

	// Resolve the model from the line prefix up to the method call
	prefix := extractPrefixBefore(line, method)
	modelFQN := r.ModelResolver(prefix, method, source, lineNum, file)
	if modelFQN == "" {
		return nil
	}

	var msg string
	if isRelMethod {
		if !r.Members.IsRelation(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown relation '%s' on model '%s'", checkName, short)
		}
	} else if isDBColMethod {
		if !r.Members.IsDBColumn(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown column '%s' on model '%s'", checkName, short)
		}
	} else if isColMethod {
		if !r.Members.IsColumn(modelFQN, checkName) {
			short := shortName(modelFQN)
			msg = fmt.Sprintf("Unknown column '%s' on model '%s'", checkName, short)
		}
	}

	if msg == "" {
		return nil
	}

	code := "unknown-column"
	if isRelMethod {
		code = "unknown-relation"
	}

	return &Finding{
		StartLine: lineNum,
		StartCol:  startCol,
		EndLine:   lineNum,
		EndCol:    endCol,
		Severity:  SeverityWarning,
		Code:      code,
		Message:   msg,
	}
}

// extractPrefixBefore returns the portion of the line before the method call
// pattern, including the -> or :: operator.
func extractPrefixBefore(line, method string) string {
	patterns := []string{"->" + method + "(", "::" + method + "("}
	bestIdx := -1
	for _, pat := range patterns {
		idx := strings.LastIndex(line, pat)
		if idx > bestIdx {
			bestIdx = idx
		}
	}
	if bestIdx < 0 {
		return ""
	}
	return line[:bestIdx]
}

// stripAlias removes " as alias" from a string (e.g., "products as cnt" → "products").
func stripAlias(s string) string {
	if idx := strings.Index(s, " as "); idx >= 0 {
		return s[:idx]
	}
	return s
}

func shortName(fqn string) string {
	if idx := strings.LastIndex(fqn, "\\"); idx >= 0 {
		return fqn[idx+1:]
	}
	return fqn
}
