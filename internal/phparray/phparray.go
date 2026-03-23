// Package phparray provides shared utilities for parsing PHP array literals
// into structured shape fields. Used by completion, hover, and models packages.
package phparray

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/types"
)

// ParseLiteralToShape parses a PHP array literal string (e.g. "['key' => 'value']")
// into ShapeFields, preserving nested structure.
func ParseLiteralToShape(arrayText string) []types.ShapeField {
	arrayText = strings.TrimSpace(arrayText)
	if len(arrayText) < 2 || arrayText[0] != '[' || arrayText[len(arrayText)-1] != ']' {
		return nil
	}
	return ParseLiteralEntries(arrayText[1 : len(arrayText)-1])
}

// ParseLiteralEntries parses comma-separated 'key' => value entries,
// respecting nested brackets, strings, and parentheses.
func ParseLiteralEntries(content string) []types.ShapeField {
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
				if f := ParseLiteralEntry(content[start:i]); f != nil {
					fields = append(fields, *f)
				}
				start = i + 1
			}
		}
	}
	if start < len(content) {
		if f := ParseLiteralEntry(content[start:]); f != nil {
			fields = append(fields, *f)
		}
	}
	return fields
}

// ParseLiteralEntry parses a single "'key' => value" entry into a ShapeField.
func ParseLiteralEntry(entry string) *types.ShapeField {
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
	valueType := InferValueType(valuePart)
	return &types.ShapeField{Key: keyPart, Type: valueType}
}

// InferValueType infers a PHP type string for a literal value expression.
// For nested arrays, recursively builds "array{key: type, ...}" shape strings.
func InferValueType(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ",")
	value = strings.TrimSpace(value)
	if value == "" {
		return "mixed"
	}
	// Nested array
	if strings.HasPrefix(value, "[") {
		nested := ParseLiteralToShape(value)
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
	// String literal
	if len(value) >= 2 && (value[0] == '\'' || value[0] == '"') {
		return "string"
	}
	// Boolean
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	// Null
	if lower == "null" {
		return "null"
	}
	// Numeric
	if len(value) > 0 && (value[0] >= '0' && value[0] <= '9' || value[0] == '-') {
		if strings.ContainsAny(value, ".eE") {
			return "float"
		}
		return "int"
	}
	return "mixed"
}

// CollectArrayLiteral collects the full text of a PHP array literal starting
// from startLine, tracking bracket depth. Handles strings but not comments.
func CollectArrayLiteral(lines []string, startLine int) string {
	var sb strings.Builder
	depth := 0
	started := false
	inString := byte(0)

	for i := startLine; i < len(lines) && i < startLine+100; i++ {
		line := lines[i]
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

// CollectReturnArray collects an array literal from a PHP return statement,
// skipping block comments (/* ... */) and line comments (// ...).
// More robust than CollectArrayLiteral — use for config files.
func CollectReturnArray(lines []string, startLine int) string {
	var sb strings.Builder
	depth := 0
	started := false
	inBlockComment := false
	inString := byte(0)

	for i := startLine; i < len(lines) && i < startLine+500; i++ {
		line := lines[i]
		for j := 0; j < len(line); j++ {
			ch := line[j]

			if inBlockComment {
				if ch == '*' && j+1 < len(line) && line[j+1] == '/' {
					inBlockComment = false
					j++
				}
				continue
			}

			if inString != 0 {
				if ch == inString && (j == 0 || line[j-1] != '\\') {
					inString = 0
				}
				if started {
					sb.WriteByte(ch)
				}
				continue
			}

			if ch == '/' && j+1 < len(line) {
				if line[j+1] == '/' {
					break
				}
				if line[j+1] == '*' {
					inBlockComment = true
					j++
					continue
				}
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
