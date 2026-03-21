package checks

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

// UnusedImportsRule detects use statements whose imported symbol is never
// referenced in the file.
type UnusedImportsRule struct{}

func (r *UnusedImportsRule) Code() string { return "unused-import" }

func (r *UnusedImportsRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding
	lines := strings.Split(source, "\n")

	for _, u := range file.Uses {
		if isImportUsed(u, lines) {
			continue
		}

		endCol := 0
		if u.StartLine >= 0 && u.StartLine < len(lines) {
			endCol = len(lines[u.StartLine])
		}

		findings = append(findings, Finding{
			StartLine: u.StartLine,
			StartCol:  0,
			EndLine:   u.StartLine,
			EndCol:    endCol,
			Severity:  SeverityHint,
			Code:      "unused-import",
			Message:   fmt.Sprintf("Unused import '%s'", u.FullName),
		})
	}
	return findings
}

// isImportUsed checks whether the imported symbol is referenced anywhere in
// the file outside its own use statement line.
func isImportUsed(u parser.UseNode, lines []string) bool {
	alias := u.Alias
	if alias == "" {
		// Extract short name from FQN
		if idx := strings.LastIndex(u.FullName, "\\"); idx >= 0 {
			alias = u.FullName[idx+1:]
		} else {
			alias = u.FullName
		}
	}

	for i, line := range lines {
		if i == u.StartLine {
			continue
		}

		switch u.Kind {
		case "function":
			if containsWordBoundary(line, alias) {
				return true
			}
		case "const":
			if containsWordBoundary(line, alias) {
				return true
			}
		default:
			// Class/interface/trait/enum import — check code and docblocks
			if containsClassRef(line, alias) {
				return true
			}
		}
	}
	return false
}

// containsClassRef checks if a line contains a class name reference at a word
// boundary. Handles: type hints, new, ::class, instanceof, catch, extends,
// implements, PHP 8 attributes, and docblock tags.
func containsClassRef(line, name string) bool {
	trimmed := strings.TrimSpace(line)

	// Fast path: if the name doesn't appear at all, skip
	if !strings.Contains(trimmed, name) {
		return false
	}

	// Check word-boundary occurrence
	return containsWordBoundary(line, name)
}

// containsWordBoundary returns true if name appears in line surrounded by
// non-identifier characters (or at string boundaries).
func containsWordBoundary(line, name string) bool {
	start := 0
	for {
		idx := strings.Index(line[start:], name)
		if idx < 0 {
			return false
		}
		absIdx := start + idx
		before := absIdx - 1
		after := absIdx + len(name)

		leftOk := before < 0 || !isIdentChar(line[before])
		rightOk := after >= len(line) || !isIdentChar(line[after])

		if leftOk && rightOk {
			return true
		}
		start = absIdx + 1
		if start >= len(line) {
			return false
		}
	}
}

func isIdentChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}
