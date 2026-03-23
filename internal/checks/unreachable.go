package checks

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// UnreachableCodeRule detects statements after unconditional return, throw,
// exit/die, continue, or break within the same block scope.
type UnreachableCodeRule struct{}

func (r *UnreachableCodeRule) Code() string { return "unreachable-code" }

func (r *UnreachableCodeRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil {
		return nil
	}
	lines := strings.Split(source, "\n")
	var findings []Finding

	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if m.EndLine <= m.StartLine {
				continue
			}
			findings = append(findings, findUnreachable(lines, m.StartLine, m.EndLine)...)
		}
	}
	return findings
}

// terminalKeywords are statements that unconditionally transfer control.
var terminalKeywords = []string{"return", "throw", "exit", "die"}

// loopTerminals only apply inside loop/switch scopes.
var loopTerminals = []string{"continue", "break"}

// findUnreachable scans lines within a method body for code after terminal
// statements at the same brace depth.
func findUnreachable(lines []string, startLine, endLine int) []Finding {
	if startLine < 0 || endLine >= len(lines) {
		return nil
	}

	var findings []Finding

	// Find the opening brace of the method body
	bodyStart := -1
	for i := startLine; i <= endLine && i < len(lines); i++ {
		if strings.Contains(lines[i], "{") {
			bodyStart = i
			break
		}
	}
	if bodyStart < 0 {
		return nil
	}

	// Scan line by line tracking brace depth relative to the method body.
	// depth=0 means we're at the method body level.
	depth := 0
	inTerminal := false
	terminalDepth := -1
	unreachableStart := -1

	for i := bodyStart; i <= endLine && i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Track brace depth changes on this line
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")

		prevDepth := depth

		// Process opens first, then closes for this line
		// But we need to handle the line's content at the depth BEFORE closes
		contentDepth := depth + openBraces

		// If we're tracking unreachable code and hit a close brace at the
		// terminal's depth, the unreachable region ends.
		if inTerminal && unreachableStart >= 0 {
			// Check if this line is just a closing brace
			if trimmed == "}" || trimmed == "};" {
				// End of block — stop tracking
				depth = depth + openBraces - closeBraces
				if depth <= terminalDepth {
					if unreachableStart > 0 {
						findings = append(findings, Finding{
							StartLine: unreachableStart,
							StartCol:  0,
							EndLine:   i - 1,
							EndCol:    len(lines[i-1]),
							Severity:  SeverityWarning,
							Code:      "unreachable-code",
							Message:   "Unreachable code detected",
						})
					}
					inTerminal = false
					unreachableStart = -1
				}
				continue
			}
		}

		depth = depth + openBraces - closeBraces
		_ = contentDepth
		_ = prevDepth

		// Skip empty/comment lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			continue
		}

		// If we found a terminal statement and now see real code at same depth
		if inTerminal && depth >= terminalDepth {
			if trimmed == "}" || trimmed == "};" {
				// Close of the block — end the unreachable region
				if unreachableStart >= 0 {
					findings = append(findings, Finding{
						StartLine: unreachableStart,
						StartCol:  0,
						EndLine:   i - 1,
						EndCol:    len(lines[i-1]),
						Severity:  SeverityWarning,
						Code:      "unreachable-code",
						Message:   "Unreachable code detected",
					})
				}
				inTerminal = false
				unreachableStart = -1
				continue
			}
			// This is actual unreachable code
			if unreachableStart < 0 {
				unreachableStart = i
			}
			continue
		}

		// If depth dropped below terminal depth, the block ended
		if inTerminal && depth < terminalDepth {
			if unreachableStart >= 0 {
				findings = append(findings, Finding{
					StartLine: unreachableStart,
					StartCol:  0,
					EndLine:   i - 1,
					EndCol:    len(lines[i-1]),
					Severity:  SeverityWarning,
					Code:      "unreachable-code",
					Message:   "Unreachable code detected",
				})
			}
			inTerminal = false
			unreachableStart = -1
		}

		// Check if this line contains a terminal statement
		if !inTerminal && isTerminalLine(trimmed) {
			inTerminal = true
			terminalDepth = depth
		}
	}

	return findings
}

// isTerminalLine checks if a trimmed line starts with a terminal keyword.
func isTerminalLine(trimmed string) bool {
	for _, kw := range terminalKeywords {
		if strings.HasPrefix(trimmed, kw+" ") || strings.HasPrefix(trimmed, kw+";") || trimmed == kw {
			return true
		}
		// Handle "return;" or "throw new Exception();"
		if strings.HasPrefix(trimmed, kw+"(") {
			return true
		}
	}
	for _, kw := range loopTerminals {
		if strings.HasPrefix(trimmed, kw+";") || strings.HasPrefix(trimmed, kw+" ") || trimmed == kw {
			return true
		}
	}
	return false
}
