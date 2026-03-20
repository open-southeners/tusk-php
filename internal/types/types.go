package types

// ShapeField represents a single field in an array shape: array{name: string, age?: int}
type ShapeField struct {
	Key      string
	Type     string
	Optional bool
}

// ParseArrayShape extracts shape fields from a PHPStan array shape type string.
// Input: "array{name: string, age: int, address?: string}"
// Returns the fields, or nil if the type is not a shape.
func ParseArrayShape(typeStr string) []ShapeField {
	typeStr = trimNullable(typeStr)

	// Handle union types — find the array shape part
	if idx := findArrayShapeInUnion(typeStr); idx >= 0 {
		typeStr = typeStr[idx:]
		// Find the end of this shape
		end := findShapeEnd(typeStr)
		if end > 0 {
			typeStr = typeStr[:end]
		}
	}

	// Must start with "array{" or "list{"
	braceStart := -1
	if len(typeStr) > 6 && typeStr[:6] == "array{" {
		braceStart = 5
	} else if len(typeStr) > 5 && typeStr[:5] == "list{" {
		braceStart = 4
	}
	if braceStart < 0 {
		return nil
	}

	// Extract content between { and }
	inner := typeStr[braceStart+1:]
	if len(inner) == 0 || inner[len(inner)-1] != '}' {
		return nil
	}
	inner = inner[:len(inner)-1]
	if inner == "" {
		return nil
	}

	return parseShapeFields(inner)
}

// parseShapeFields splits "name: string, age?: int" into fields,
// respecting nested braces, angle brackets, and parentheses.
func parseShapeFields(inner string) []ShapeField {
	var fields []ShapeField
	depth := 0 // tracks {, <, (
	start := 0

	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ',':
			if depth == 0 {
				if f := parseOneField(inner[start:i]); f != nil {
					fields = append(fields, *f)
				}
				start = i + 1
			}
		}
	}
	// Last field
	if start < len(inner) {
		if f := parseOneField(inner[start:]); f != nil {
			fields = append(fields, *f)
		}
	}
	return fields
}

// parseOneField parses "name: string" or "age?: int" into a ShapeField.
func parseOneField(s string) *ShapeField {
	s = trim(s)
	if s == "" {
		return nil
	}

	// Find the colon separator (respecting nested types)
	colonIdx := -1
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ':':
			if depth == 0 {
				colonIdx = i
				goto found
			}
		}
	}
found:
	if colonIdx < 0 {
		// Positional field (no key name), e.g. "string" in array{string, int}
		return &ShapeField{Type: trim(s)}
	}

	key := trim(s[:colonIdx])
	typ := trim(s[colonIdx+1:])

	optional := false
	if len(key) > 0 && key[len(key)-1] == '?' {
		optional = true
		key = key[:len(key)-1]
	}

	// Strip quotes from key if present: 'key-name' or "key-name"
	if len(key) >= 2 && (key[0] == '\'' || key[0] == '"') && key[len(key)-1] == key[0] {
		key = key[1 : len(key)-1]
	}

	return &ShapeField{Key: key, Type: typ, Optional: optional}
}

// ExtractDocTypeString extracts a type string from a docblock value,
// handling nested braces/angles so "array{name: string, age: int} $var desc"
// correctly returns ("array{name: string, age: int}", "$var desc").
func ExtractDocTypeString(value string) (typeStr, rest string) {
	value = trim(value)
	if value == "" {
		return "", ""
	}

	depth := 0
	i := 0
	for i < len(value) {
		ch := value[i]
		switch ch {
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ' ', '\t':
			if depth == 0 {
				return value[:i], trim(value[i:])
			}
		}
		i++
	}
	return value, ""
}

func trimNullable(s string) string {
	s = trim(s)
	if len(s) > 0 && s[0] == '?' {
		return s[1:]
	}
	return s
}

func findArrayShapeInUnion(s string) int {
	// Look for "array{" or "list{" in a union like "string|array{key: int}"
	for i := 0; i < len(s)-5; i++ {
		if (s[i:i+6] == "array{" || s[i:i+5] == "list{") {
			return i
		}
	}
	return -1
}

func findShapeEnd(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
