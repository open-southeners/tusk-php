package resolve

import "strings"

// ResolvedType represents a type with optional generic parameters.
// For example, Collection<int, Category> is {FQN: "Illuminate\...\Collection", Params: [{FQN: "int"}, {FQN: "App\Models\Category"}]}.
type ResolvedType struct {
	FQN      string         // base type FQN, e.g., "Illuminate\Database\Eloquent\Collection"
	Params   []ResolvedType // generic type parameters
	Nullable bool           // true if prefixed with ?
}

// BaseFQN returns just the FQN without generic parameters.
func (rt ResolvedType) BaseFQN() string {
	return rt.FQN
}

// IsGeneric returns true if the type has generic parameters.
func (rt ResolvedType) IsGeneric() bool {
	return len(rt.Params) > 0
}

// IsEmpty returns true if the type has no FQN.
func (rt ResolvedType) IsEmpty() bool {
	return rt.FQN == ""
}

// String serializes the type back to a string like "Collection<int, Category>".
func (rt ResolvedType) String() string {
	if rt.FQN == "" {
		return ""
	}
	var sb strings.Builder
	if rt.Nullable {
		sb.WriteByte('?')
	}
	sb.WriteString(rt.FQN)
	if len(rt.Params) > 0 {
		sb.WriteByte('<')
		for i, p := range rt.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(p.String())
		}
		sb.WriteByte('>')
	}
	return sb.String()
}

// ParseGenericType parses a type string like "Collection<int, Category>" into
// a ResolvedType. Handles nested generics and nullable prefix.
func ParseGenericType(raw string) ResolvedType {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ResolvedType{}
	}

	nullable := false
	if strings.HasPrefix(raw, "?") {
		nullable = true
		raw = raw[1:]
	}

	// Strip leading backslash
	raw = strings.TrimPrefix(raw, "\\")

	// Find the opening < for generic params
	openIdx := strings.IndexByte(raw, '<')
	if openIdx < 0 {
		return ResolvedType{FQN: raw, Nullable: nullable}
	}

	// Verify there's a matching closing >
	if raw[len(raw)-1] != '>' {
		// Malformed — return base type only
		return ResolvedType{FQN: raw[:openIdx], Nullable: nullable}
	}

	baseFQN := raw[:openIdx]
	paramsStr := raw[openIdx+1 : len(raw)-1]

	// Split params by comma, respecting nested < >
	params := splitGenericParams(paramsStr)
	var resolved []ResolvedType
	for _, p := range params {
		resolved = append(resolved, ParseGenericType(p))
	}

	return ResolvedType{FQN: baseFQN, Params: resolved, Nullable: nullable}
}

// splitGenericParams splits "int, Collection<int, Model>" into ["int", "Collection<int, Model>"]
// respecting nested angle brackets.
func splitGenericParams(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}
