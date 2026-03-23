package checks

import (
	"fmt"
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// UnusedPrivateRule detects private methods and properties that are never
// referenced within their declaring file.
type UnusedPrivateRule struct{}

func (r *UnusedPrivateRule) Code() string { return "unused-private" }

func (r *UnusedPrivateRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding

	for _, cls := range file.Classes {
		findings = append(findings, checkUnusedPrivateMethods(cls, lines)...)
		findings = append(findings, checkUnusedPrivateProperties(cls, lines)...)
	}
	return findings
}

// magicMethods are methods that PHP calls implicitly — never flag these as unused.
var magicMethods = map[string]bool{
	"__construct": true, "__destruct": true, "__get": true, "__set": true,
	"__call": true, "__callStatic": true, "__isset": true, "__unset": true,
	"__toString": true, "__invoke": true, "__clone": true, "__debugInfo": true,
	"__serialize": true, "__unserialize": true, "__sleep": true, "__wakeup": true,
	"__set_state": true,
}

func checkUnusedPrivateMethods(cls parser.ClassNode, lines []string) []Finding {
	var findings []Finding
	for _, m := range cls.Methods {
		if m.Visibility != "private" {
			continue
		}
		if magicMethods[m.Name] {
			continue
		}
		if isMethodReferenced(m.Name, m.StartLine, cls, lines) {
			continue
		}
		endCol := 0
		if m.StartLine >= 0 && m.StartLine < len(lines) {
			endCol = len(lines[m.StartLine])
		}
		findings = append(findings, Finding{
			StartLine: m.StartLine,
			StartCol:  m.StartCol,
			EndLine:   m.StartLine,
			EndCol:    endCol,
			Severity:  SeverityInfo,
			Code:      "unused-private-method",
			Message:   fmt.Sprintf("Private method '%s::%s' is never used", cls.Name, m.Name),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

func checkUnusedPrivateProperties(cls parser.ClassNode, lines []string) []Finding {
	var findings []Finding
	for _, p := range cls.Properties {
		if p.Visibility != "private" {
			continue
		}
		propName := strings.TrimPrefix(p.Name, "$")
		if isPropertyReferenced(propName, p.StartLine, cls, lines) {
			continue
		}
		endCol := 0
		if p.StartLine >= 0 && p.StartLine < len(lines) {
			endCol = len(lines[p.StartLine])
		}
		findings = append(findings, Finding{
			StartLine: p.StartLine,
			StartCol:  p.StartCol,
			EndLine:   p.StartLine,
			EndCol:    endCol,
			Severity:  SeverityHint,
			Code:      "unused-private-property",
			Message:   fmt.Sprintf("Private property '%s::$%s' is never used", cls.Name, propName),
			Tags:      []Tag{TagUnnecessary},
		})
	}
	return findings
}

// isMethodReferenced checks if a private method name is referenced anywhere
// in the class body outside its declaration line.
func isMethodReferenced(name string, declLine int, cls parser.ClassNode, lines []string) bool {
	start := cls.StartLine
	end := endLineOf(cls, lines)

	// Patterns: ->methodName(  self::methodName(  static::methodName(
	// Also callable array: [$this, 'methodName']  [self::class, 'methodName']
	for i := start; i <= end && i < len(lines); i++ {
		if i == declLine {
			continue
		}
		line := lines[i]
		if strings.Contains(line, "->"+name) || strings.Contains(line, "::"+name) {
			return true
		}
		// Callable string: 'methodName' or "methodName" as array element
		if strings.Contains(line, "'"+name+"'") || strings.Contains(line, "\""+name+"\"") {
			return true
		}
	}
	return false
}

// isPropertyReferenced checks if a private property (without $ prefix) is
// referenced anywhere in the class body outside its declaration line.
func isPropertyReferenced(name string, declLine int, cls parser.ClassNode, lines []string) bool {
	start := cls.StartLine
	end := endLineOf(cls, lines)

	// Patterns: $this->name  self::$name  static::$name
	accessPattern := "->" + name
	staticPattern := "::$" + name

	for i := start; i <= end && i < len(lines); i++ {
		if i == declLine {
			continue
		}
		line := lines[i]
		// Instance access: ->name (check word boundary after)
		if idx := strings.Index(line, accessPattern); idx >= 0 {
			after := idx + len(accessPattern)
			if after >= len(line) || !isIdentChar(line[after]) {
				return true
			}
		}
		// Static access: ::$name
		if strings.Contains(line, staticPattern) {
			return true
		}
	}
	return false
}

// endLineOf estimates the end line of a class from the source. It looks for
// the class EndLine if available, otherwise scans for the matching closing brace.
func endLineOf(cls parser.ClassNode, lines []string) int {
	// Scan from class start to find the matching closing brace
	depth := 0
	for i := cls.StartLine; i < len(lines); i++ {
		for _, ch := range lines[i] {
			if ch == '{' {
				depth++
			} else if ch == '}' {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return len(lines) - 1
}
