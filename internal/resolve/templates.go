package resolve

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// TemplateMapping defines the generic type parameters for a class and how
// method return types relate to those parameters.
type TemplateMapping struct {
	Params  []string          // e.g., ["TModel"] for Builder
	Methods map[string]string // method name → return type template
}

// knownTemplates maps class FQNs to their template definitions.
// This covers the core Laravel Eloquent types.
var knownTemplates = map[string]TemplateMapping{
	"Illuminate\\Database\\Eloquent\\Builder": {
		Params: []string{"TModel"},
		Methods: map[string]string{
			// Query execution — return model or collection
			"get":         "Illuminate\\Database\\Eloquent\\Collection<int, TModel>",
			"first":       "?TModel",
			"firstOrFail": "TModel",
			"find":        "?TModel",
			"findOrFail":  "TModel",
			"findMany":    "Illuminate\\Database\\Eloquent\\Collection<int, TModel>",
			"create":      "TModel",
			"firstOrNew":  "TModel",
			"firstOrCreate": "TModel",
			"updateOrCreate": "TModel",
			"sole":        "TModel",
			// Builder methods — return static (preserves Builder<TModel>)
			"where": "static", "whereIn": "static", "whereNotIn": "static",
			"whereNull": "static", "whereNotNull": "static",
			"whereBetween": "static", "whereNotBetween": "static",
			"whereDate": "static", "whereMonth": "static", "whereDay": "static",
			"whereYear": "static", "whereTime": "static", "whereColumn": "static",
			"orderBy": "static", "orderByDesc": "static",
			"latest": "static", "oldest": "static",
			"limit": "static", "take": "static", "skip": "static", "offset": "static",
			"groupBy": "static", "having": "static",
			"select": "static", "addSelect": "static",
			"distinct": "static",
			"with": "static", "without": "static",
			"has": "static", "doesntHave": "static",
			"whereHas": "static", "whereDoesntHave": "static",
			"withCount": "static",
			"join": "static", "leftJoin": "static",
			"withTrashed": "static", "onlyTrashed": "static",
			"when": "static", "unless": "static",
			"tap": "static",
		},
	},
	"Illuminate\\Database\\Eloquent\\Collection": {
		Params: []string{"TKey", "TModel"},
		Methods: map[string]string{
			"first":   "?TModel",
			"last":    "?TModel",
			"find":    "?TModel",
			"map":     "Illuminate\\Support\\Collection<TKey, mixed>",
			"pluck":   "Illuminate\\Support\\Collection",
			"all":     "array",
			"toArray": "array",
			"count":   "int",
			"filter":  "static",
			"reject":  "static",
			"unique":  "static",
			"values":  "static",
			"sortBy":  "static",
			"each":    "static",
		},
	},
	"Illuminate\\Support\\Collection": {
		Params: []string{"TKey", "TValue"},
		Methods: map[string]string{
			"first":   "?TValue",
			"last":    "?TValue",
			"map":     "static",
			"filter":  "static",
			"reject":  "static",
			"count":   "int",
			"all":     "array",
			"toArray": "array",
			"values":  "static",
			"keys":    "static",
			"each":    "static",
		},
	},
}

// ResolveTemplateReturn substitutes template parameters in a method's return
// type template. Returns an empty ResolvedType if the class has no template
// mapping or the method is unknown.
func ResolveTemplateReturn(classFQN, methodName string, typeParams []ResolvedType) ResolvedType {
	tmpl, ok := knownTemplates[classFQN]
	if !ok {
		return ResolvedType{}
	}

	retTemplate, ok := tmpl.Methods[methodName]
	if !ok {
		return ResolvedType{}
	}

	// "static" means preserve the same type with the same params
	if retTemplate == "static" {
		return ResolvedType{FQN: classFQN, Params: typeParams}
	}

	// Build param substitution map: TModel → actual type
	subst := make(map[string]ResolvedType)
	for i, paramName := range tmpl.Params {
		if i < len(typeParams) {
			subst[paramName] = typeParams[i]
		}
	}

	return substituteTemplate(retTemplate, subst)
}

// substituteTemplate parses a return type template and replaces template
// parameter names with their concrete types.
func substituteTemplate(template string, subst map[string]ResolvedType) ResolvedType {
	template = strings.TrimSpace(template)

	nullable := false
	if strings.HasPrefix(template, "?") {
		nullable = true
		template = template[1:]
	}

	// Check if the entire template is a single param name (e.g., "TModel")
	if rt, ok := subst[template]; ok {
		result := rt
		result.Nullable = result.Nullable || nullable
		return result
	}

	// Parse as generic type and substitute params recursively
	parsed := ParseGenericType(template)
	parsed.Nullable = parsed.Nullable || nullable

	// Substitute the base FQN if it's a template param
	if rt, ok := subst[parsed.FQN]; ok {
		parsed.FQN = rt.FQN
	}

	// Substitute params
	for i, p := range parsed.Params {
		if rt, ok := subst[p.FQN]; ok {
			parsed.Params[i] = rt
		}
	}

	return parsed
}

// HasTemplateMapping returns true if the class has known template mappings.
func HasTemplateMapping(classFQN string) bool {
	_, ok := knownTemplates[classFQN]
	return ok
}

// ResolveSymbolTemplateReturn resolves a method return type using the symbol's
// @template declarations and @return docblock. Falls back if the method's
// return type references a template parameter.
func (r *Resolver) ResolveSymbolTemplateReturn(sym *symbols.Symbol, methodName string, typeParams []ResolvedType) ResolvedType {
	if sym == nil || len(sym.Templates) == 0 || len(typeParams) == 0 {
		return ResolvedType{}
	}

	// Build substitution map from symbol's template params
	subst := make(map[string]ResolvedType)
	for i, tmpl := range sym.Templates {
		if i < len(typeParams) {
			subst[tmpl.Name] = typeParams[i]
		}
	}

	// Find the method and check if its return type references a template param
	member := r.FindMember(sym.FQN, methodName)
	if member == nil {
		return ResolvedType{}
	}

	retType := member.ReturnType
	if retType == "" && member.DocComment != "" {
		if doc := parser.ParseDocBlock(member.DocComment); doc != nil && doc.Return.Type != "" {
			retType = doc.Return.Type
		}
	}
	if retType == "" {
		return ResolvedType{}
	}

	return substituteTemplate(retType, subst)
}
