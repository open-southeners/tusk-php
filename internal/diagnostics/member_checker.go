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
