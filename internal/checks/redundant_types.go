package checks

import (
	"fmt"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// RedundantNullsafeRule detects ?-> on expressions whose type is known
// to be non-nullable. Requires a TypeResolver to resolve expression types.
type RedundantNullsafeRule struct {
	// TypeResolver resolves the type of an expression at a given line.
	// Returns the type string (e.g., "string", "?Foo", "Foo|null", "mixed").
	// If nil, the rule is a no-op.
	TypeResolver func(expr string, source string, line int, file *parser.FileNode) string
}

func (r *RedundantNullsafeRule) Code() string { return "redundant-nullsafe" }

func (r *RedundantNullsafeRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if r.TypeResolver == nil || file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding

	for i, line := range lines {
		idx := 0
		for {
			pos := strings.Index(line[idx:], "?->")
			if pos < 0 {
				break
			}
			absPos := idx + pos

			// Extract expression before ?->
			expr := strings.TrimSpace(line[:absPos])
			// Walk back to find the start of the expression
			exprStart := len(expr)
			for exprStart > 0 {
				ch := expr[exprStart-1]
				if isIdentChar(ch) || ch == '$' || ch == '>' || ch == '-' || ch == ':' || ch == '(' || ch == ')' || ch == '\\' {
					exprStart--
				} else {
					break
				}
			}
			expr = strings.TrimSpace(expr[exprStart:])

			if expr != "" {
				resolved := r.TypeResolver(expr, source, i, file)
				if resolved == "null" {
					findings = append(findings, Finding{
						StartLine: i,
						StartCol:  absPos,
						EndLine:   i,
						EndCol:    absPos + 3,
						Severity:  SeverityWarning,
						Code:      "redundant-nullsafe",
						Message:   "Expression is always null, method call will never execute",
					})
				} else if resolved != "" && !isNullable(resolved) {
					findings = append(findings, Finding{
						StartLine: i,
						StartCol:  absPos,
						EndLine:   i,
						EndCol:    absPos + 3,
						Severity:  SeverityInfo,
						Code:      "redundant-nullsafe",
						Message:   fmt.Sprintf("Redundant nullsafe operator: type '%s' is non-nullable", resolved),
					})
				}
			}
			idx = absPos + 3
		}
	}
	return findings
}

// isNullable returns true if the type can be null.
func isNullable(typ string) bool {
	if typ == "" || typ == "mixed" || typ == "null" {
		return true
	}
	if strings.HasPrefix(typ, "?") {
		return true
	}
	// Union with null
	for _, part := range strings.Split(typ, "|") {
		if strings.TrimSpace(part) == "null" {
			return true
		}
	}
	return false
}

// RedundantUnionRule detects union type declarations with duplicate members
// or types that are supertypes of other members.
type RedundantUnionRule struct{}

func (r *RedundantUnionRule) Code() string { return "redundant-union-member" }

func (r *RedundantUnionRule) Check(file *parser.FileNode, source string, index *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding

	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			// Check parameter types
			for _, p := range m.Params {
				findings = append(findings, checkUnionType(p.Type.Name, m.StartLine, index)...)
			}
			// Check return type
			findings = append(findings, checkUnionType(m.ReturnType.Name, m.StartLine, index)...)
		}
		for _, p := range cls.Properties {
			findings = append(findings, checkUnionType(p.Type.Name, p.StartLine, index)...)
		}
	}
	for _, fn := range file.Functions {
		for _, p := range fn.Params {
			findings = append(findings, checkUnionType(p.Type.Name, fn.StartLine, index)...)
		}
		findings = append(findings, checkUnionType(fn.ReturnType.Name, fn.StartLine, index)...)
	}
	return findings
}

// supertypes maps types that subsume other types.
var supertypes = map[string][]string{
	"mixed":    {"string", "int", "float", "bool", "array", "object", "null", "callable", "iterable", "void", "never"},
	"object":   {}, // any class name is subsumed
	"iterable": {"array"},
}

func checkUnionType(typeStr string, line int, index *symbols.Index) []Finding {
	if typeStr == "" {
		return nil
	}

	// Handle ?Type as Type|null
	normalized := typeStr
	if strings.HasPrefix(normalized, "?") {
		normalized = normalized[1:] + "|null"
	}

	if !strings.Contains(normalized, "|") {
		return nil
	}

	parts := strings.Split(normalized, "|")
	if len(parts) < 2 {
		return nil
	}

	// Trim whitespace from each part
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	var findings []Finding

	// Check for duplicates
	seen := make(map[string]bool)
	for _, p := range parts {
		lower := strings.ToLower(p)
		if seen[lower] {
			findings = append(findings, Finding{
				StartLine: line,
				StartCol:  0,
				EndLine:   line,
				EndCol:    0,
				Severity:  SeverityInfo,
				Code:      "redundant-union-member",
				Message:   fmt.Sprintf("Duplicate type '%s' in union", p),
			})
		}
		seen[lower] = true
	}

	// Check ?Type|null redundancy (the ? already makes it nullable)
	if strings.HasPrefix(typeStr, "?") {
		for _, p := range parts {
			if strings.ToLower(p) == "null" {
				findings = append(findings, Finding{
					StartLine: line,
					StartCol:  0,
					EndLine:   line,
					EndCol:    0,
					Severity:  SeverityInfo,
					Code:      "redundant-union-member",
					Message:   fmt.Sprintf("Redundant 'null' in union: '?' already makes '%s' nullable", typeStr),
				})
				break
			}
		}
	}

	// Check for supertype subsumption
	partSet := make(map[string]bool)
	for _, p := range parts {
		partSet[strings.ToLower(p)] = true
	}

	for _, p := range parts {
		lower := strings.ToLower(p)

		// Check if any supertype is present
		for super, subsumed := range supertypes {
			if !partSet[super] || super == lower {
				continue
			}
			// "mixed" subsumes everything
			if super == "mixed" {
				findings = append(findings, Finding{
					StartLine: line,
					StartCol:  0,
					EndLine:   line,
					EndCol:    0,
					Severity:  SeverityInfo,
					Code:      "redundant-union-member",
					Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by 'mixed'", p),
				})
				break
			}
			// Check if p is in the known subsumed list
			for _, s := range subsumed {
				if lower == s {
					findings = append(findings, Finding{
						StartLine: line,
						StartCol:  0,
						EndLine:   line,
						EndCol:    0,
						Severity:  SeverityInfo,
						Code:      "redundant-union-member",
						Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by '%s'", p, super),
					})
					break
				}
			}
			// "object" subsumes any class name
			if super == "object" && !symbols.IsPHPBuiltinType(lower) && lower != "null" {
				findings = append(findings, Finding{
					StartLine: line,
					StartCol:  0,
					EndLine:   line,
					EndCol:    0,
					Severity:  SeverityInfo,
					Code:      "redundant-union-member",
					Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by 'object'", p),
				})
			}
		}

		// Check inheritance: if both parent and child class in union
		if index != nil && !symbols.IsPHPBuiltinType(lower) && lower != "null" {
			chain := index.GetInheritanceChain(p)
			for _, parent := range chain {
				if partSet[strings.ToLower(parent)] {
					findings = append(findings, Finding{
						StartLine: line,
						StartCol:  0,
						EndLine:   line,
						EndCol:    0,
						Severity:  SeverityInfo,
						Code:      "redundant-union-member",
						Message:   fmt.Sprintf("Redundant type '%s' in union: already covered by parent '%s'", p, parent),
					})
					break
				}
			}
		}
	}

	return findings
}
