package diagnostics

import (
	"strings"

	"github.com/open-southeners/php-lsp/internal/checks"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

// NewIndexMemberChecker returns a MemberChecker backed by the symbol index.
func NewIndexMemberChecker(index *symbols.Index) checks.MemberChecker {
	return &indexMemberChecker{index: index}
}

// indexMemberChecker implements checks.MemberChecker using the symbol index.
type indexMemberChecker struct {
	index *symbols.Index
}

func (c *indexMemberChecker) IsColumn(modelFQN, name string) bool {
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindProperty {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		if colName != name {
			continue
		}
		// Exclude relation-derived virtual properties
		if m.IsVirtual && strings.HasSuffix(m.DocComment, " relation") {
			continue
		}
		// Exclude accessors
		if m.IsVirtual && strings.HasPrefix(m.DocComment, "Accessor from ") {
			continue
		}
		return true
	}
	return false
}

func (c *indexMemberChecker) IsDBColumn(modelFQN, name string) bool {
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindProperty || !m.IsVirtual {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		if colName != name {
			continue
		}
		doc := m.DocComment
		if strings.HasPrefix(doc, "From migration") ||
			strings.HasPrefix(doc, "(database column)") ||
			doc == "Doctrine column" || doc == "Doctrine ID column" {
			return true
		}
		// Skip relations and accessors
		if strings.HasSuffix(doc, " relation") || strings.HasPrefix(doc, "Accessor from ") {
			continue
		}
		// Other virtual properties (@property from IDE helper)
		return true
	}
	return false
}

func (c *indexMemberChecker) RelatedModelFQN(modelFQN, relationName string) string {
	// Look at the virtual property for this relation — for singular relations
	// the Type is "?RelatedModelFQN", for plural it's "Collection" (lost).
	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindProperty {
			continue
		}
		propName := strings.TrimPrefix(m.Name, "$")
		if propName != relationName {
			continue
		}
		if !m.IsVirtual || !strings.HasSuffix(m.DocComment, " relation") {
			continue
		}
		typ := m.Type
		// Singular relations: "?App\Models\Product" → "App\Models\Product"
		if strings.HasPrefix(typ, "?") {
			return typ[1:]
		}
		// Plural relations: Type is "Illuminate\Database\Eloquent\Collection"
		// Can't determine related model from this — return ""
		return ""
	}
	return ""
}

func (c *indexMemberChecker) IsRelation(modelFQN, name string) bool {
	// Relation return type FQN fragments
	relationTypes := map[string]bool{
		"HasOne": true, "HasMany": true,
		"BelongsTo": true, "BelongsToMany": true,
		"MorphOne": true, "MorphMany": true,
		"MorphTo": true, "MorphToMany": true, "MorphedByMany": true,
		"HasOneThrough": true, "HasManyThrough": true,
	}

	for _, m := range c.index.GetClassMembers(modelFQN) {
		if m.Kind != symbols.KindMethod || m.Name != name {
			continue
		}
		retType := m.ReturnType
		if retType == "" {
			continue
		}
		// Extract short class name from return type
		short := retType
		if idx := strings.LastIndex(short, "\\"); idx >= 0 {
			short = short[idx+1:]
		}
		if relationTypes[short] {
			return true
		}
	}
	return false
}
