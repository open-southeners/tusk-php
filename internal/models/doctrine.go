package models

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

// Doctrine SQL type → PHP type mapping.
var doctrineTypeMap = map[string]string{
	"string":          "string",
	"text":            "string",
	"integer":         "int",
	"smallint":        "int",
	"bigint":          "int",
	"boolean":         "bool",
	"decimal":         "string",
	"float":           "float",
	"datetime":        "\\DateTimeInterface",
	"datetime_immutable": "\\DateTimeImmutable",
	"datetimetz":      "\\DateTimeInterface",
	"date":            "\\DateTimeInterface",
	"date_immutable":  "\\DateTimeImmutable",
	"time":            "\\DateTimeInterface",
	"time_immutable":  "\\DateTimeImmutable",
	"array":           "array",
	"simple_array":    "array",
	"json":            "array",
	"json_array":      "array",
	"object":          "object",
	"binary":          "string",
	"blob":            "string",
	"guid":            "string",
}

// Regexes for PHP 8 attribute syntax.
var (
	// Matches #[ORM\Entity(...)] or #[ORM\Entity]
	ormEntityRe = regexp.MustCompile(`#\[\s*(?:ORM\\)?Entity\b(?:\(([^)]*)\))?`)
	// Matches #[ORM\Column(...)] with named args
	ormColumnRe = regexp.MustCompile(`#\[\s*(?:ORM\\)?Column\b\(([^)]*)\)`)
	// Matches #[ORM\Id]
	ormIdRe = regexp.MustCompile(`#\[\s*(?:ORM\\)?Id\b`)
	// Matches #[ORM\GeneratedValue]
	ormGeneratedValueRe = regexp.MustCompile(`#\[\s*(?:ORM\\)?GeneratedValue\b`)
	// Matches #[ORM\OneToOne(...)], #[ORM\OneToMany(...)], #[ORM\ManyToOne(...)], #[ORM\ManyToMany(...)]
	ormRelationRe = regexp.MustCompile(`#\[\s*(?:ORM\\)?(OneToOne|OneToMany|ManyToOne|ManyToMany)\b\(([^)]*)\)`)
	// Matches #[ORM\Table(name: 'tablename')]
	ormTableRe = regexp.MustCompile(`#\[\s*(?:ORM\\)?Table\b\(([^)]*)\)`)

	// Annotation syntax: @ORM\Column(type="string")
	ormColumnAnnotRe  = regexp.MustCompile(`@ORM\\Column\(([^)]*)\)`)
	ormEntityAnnotRe  = regexp.MustCompile(`@ORM\\Entity\b(?:\(([^)]*)\))?`)
	ormRelationAnnotRe = regexp.MustCompile(`@ORM\\(OneToOne|OneToMany|ManyToOne|ManyToMany)\(([^)]*)\)`)

	// Matches a PHP property declaration line: visibility [static] [readonly] [?Type] $name
	phpPropertyDeclRe = regexp.MustCompile(`(?:public|protected|private)\s+(?:static\s+)?(?:readonly\s+)?(?:(\??\S+)\s+)?(\$\w+)`)
	ormIdAnnotRe      = regexp.MustCompile(`@ORM\\Id\b`)

	// Extract named argument values
	typeArgRe       = regexp.MustCompile(`type\s*[:=]\s*['"]([^'"]+)['"]`)
	targetEntityRe  = regexp.MustCompile(`targetEntity\s*[:=]\s*([A-Za-z_\\]+)::class`)
	repoClassRe     = regexp.MustCompile(`repositoryClass\s*[:=]\s*([A-Za-z_\\]+)::class`)
	nameArgRe       = regexp.MustCompile(`name\s*[:=]\s*['"]([^'"]+)['"]`)
)

// Singular Doctrine relations (result is nullable entity).
var doctrineSingularRelations = map[string]bool{
	"OneToOne": true, "ManyToOne": true,
}

// Plural Doctrine relations (result is Collection).
var doctrinePluralRelations = map[string]bool{
	"OneToMany": true, "ManyToMany": true,
}

// AnalyzeDoctrineEntities scans all classes that have ORM\Entity attributes
// and injects virtual members for columns, relations, and repository magic methods.
func AnalyzeDoctrineEntities(index *symbols.Index, rootPath string) {
	uris := index.GetAllFileURIs()
	for _, uri := range uris {
		path := symbols.URIToPath(uri)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		source := string(content)

		// Quick check: does this file mention ORM\Entity?
		if !strings.Contains(source, "ORM\\Entity") && !strings.Contains(source, "ORM\\Column") {
			continue
		}

		file := parser.ParseFile(source)
		if file == nil {
			continue
		}

		for i := range file.Classes {
			cls := &file.Classes[i]
			fqn := file.Namespace + "\\" + cls.Name
			if file.Namespace == "" {
				fqn = cls.Name
			}

			sym := index.Lookup(fqn)
			if sym == nil {
				continue
			}

			resolve := func(name string) string {
				return resolveWithUses(name, file.Namespace, file.Uses)
			}

			// Check for ORM\Entity in class docblock (annotation syntax) or source around class declaration
			classSource := extractClassSource(source, cls)
			if !isDoctrineEntity(classSource, cls.DocComment) {
				continue
			}

			// Try PHP 8 attributes on properties first, then XML fallback
			foundAttrs := analyzeDoctrineAttributes(index, sym, source, cls, resolve)

			if !foundAttrs {
				// XML mapping fallback
				analyzeDoctrineXML(index, sym, rootPath, fqn, cls, resolve)
			}

			// Repository magic methods
			repoFQN := extractRepositoryClass(classSource, cls.DocComment, resolve)
			if repoFQN != "" {
				injectRepositoryMagicMethods(index, repoFQN, fqn, sym)
			}
		}
	}
}

// isDoctrineEntity checks if the class has an ORM\Entity attribute or annotation.
func isDoctrineEntity(classSource, docComment string) bool {
	if ormEntityRe.MatchString(classSource) {
		return true
	}
	if docComment != "" && ormEntityAnnotRe.MatchString(docComment) {
		return true
	}
	return false
}

// extractClassSource returns the source text from the class declaration line to the end.
func extractClassSource(source string, cls *parser.ClassNode) string {
	lines := strings.Split(source, "\n")
	// Look a few lines before the class declaration for attributes
	start := cls.StartLine - 5
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

// analyzeDoctrineAttributes scans source text for PHP 8 ORM attributes on properties.
// Works directly on source lines because the parser may not handle ::class inside attributes.
// Returns true if any ORM attributes were found.
func analyzeDoctrineAttributes(index *symbols.Index, entitySym *symbols.Symbol, source string, cls *parser.ClassNode, resolve func(string) string) bool {
	lines := strings.Split(source, "\n")
	found := false

	// Scan lines within the class body for property declarations
	for i := cls.StartLine; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])

		// Stop at closing brace of class
		if line == "}" {
			break
		}

		// Find property declarations
		m := phpPropertyDeclRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		propTypeHint := m[1] // may be empty
		propName := m[2]     // includes $

		// Gather attribute lines above this property (up to 6 lines back)
		attrStart := i - 6
		if attrStart < cls.StartLine {
			attrStart = cls.StartLine
		}
		attrBlock := strings.Join(lines[attrStart:i+1], "\n")

		// Check for #[ORM\Column]
		if cm := ormColumnRe.FindStringSubmatch(attrBlock); cm != nil {
			found = true
			args := cm[1]
			phpType := resolveColumnType(args, propTypeHint)

			isId := ormIdRe.MatchString(attrBlock)
			isGenerated := ormGeneratedValueRe.MatchString(attrBlock)

			existing := index.Lookup(entitySym.FQN + "::" + propName)
			if existing != nil && existing.Type != "" && existing.Type != "mixed" {
				// Already has a good type, just update readonly if needed
				if isId || isGenerated {
					existing.IsReadonly = true
				}
				continue
			}

			if existing != nil {
				existing.Type = phpType
				if isId || isGenerated {
					existing.IsReadonly = true
				}
			} else {
				vis := "private"
				if strings.Contains(line, "public") {
					vis = "public"
				} else if strings.Contains(line, "protected") {
					vis = "protected"
				}
				index.AddVirtualMember(entitySym.FQN, &symbols.Symbol{
					Name:       propName,
					FQN:        entitySym.FQN + "::" + propName,
					Kind:       symbols.KindProperty,
					URI:        entitySym.URI,
					Visibility: vis,
					Type:       phpType,
					IsVirtual:  true,
					IsReadonly: isId || isGenerated,
					DocComment: "Doctrine column",
				})
			}
			continue
		}

		// Check for ORM relation attributes
		if rm := ormRelationRe.FindStringSubmatch(attrBlock); rm != nil {
			found = true
			relType := rm[1]
			args := rm[2]
			injectDoctrineRelation(index, entitySym, strings.TrimPrefix(propName, "$"), relType, args, resolve)
			continue
		}
	}

	return found
}

// resolveColumnType maps Doctrine column type to PHP type.
func resolveColumnType(args, propType string) string {
	// If the property already has a PHP type hint, prefer it
	if propType != "" && propType != "mixed" {
		return propType
	}
	// Extract type from attribute args
	if m := typeArgRe.FindStringSubmatch(args); m != nil {
		if phpType, ok := doctrineTypeMap[m[1]]; ok {
			return phpType
		}
	}
	return "mixed"
}

// injectDoctrineRelation creates a virtual property for a Doctrine relation.
func injectDoctrineRelation(index *symbols.Index, entitySym *symbols.Symbol, propName, relType, args string, resolve func(string) string) {
	if !strings.HasPrefix(propName, "$") {
		propName = "$" + propName
	}

	// Extract target entity
	targetEntity := ""
	if m := targetEntityRe.FindStringSubmatch(args); m != nil {
		targetEntity = resolve(m[1])
	}

	var propType string
	if doctrineSingularRelations[relType] {
		if targetEntity != "" {
			propType = "?" + targetEntity
		} else {
			propType = "mixed"
		}
	} else if doctrinePluralRelations[relType] {
		propType = "Doctrine\\Common\\Collections\\Collection"
	} else {
		propType = "mixed"
	}

	existing := index.Lookup(entitySym.FQN + "::" + propName)
	if existing != nil {
		// Update type if missing
		if existing.Type == "" || existing.Type == "mixed" {
			existing.Type = propType
		}
		return
	}

	index.AddVirtualMember(entitySym.FQN, &symbols.Symbol{
		Name:       propName,
		FQN:        entitySym.FQN + "::" + propName,
		Kind:       symbols.KindProperty,
		URI:        entitySym.URI,
		Visibility: "private",
		Type:       propType,
		IsVirtual:  true,
		DocComment: relType + " relation",
	})
}

// extractRepositoryClass extracts the repository class FQN from #[ORM\Entity(repositoryClass: ...)]
func extractRepositoryClass(classSource, docComment string, resolve func(string) string) string {
	// Check attribute syntax
	if m := ormEntityRe.FindStringSubmatch(classSource); m != nil && m[1] != "" {
		if rm := repoClassRe.FindStringSubmatch(m[1]); rm != nil {
			return resolve(rm[1])
		}
	}
	// Check annotation syntax
	if docComment != "" {
		if m := ormEntityAnnotRe.FindStringSubmatch(docComment); m != nil && m[1] != "" {
			if rm := repoClassRe.FindStringSubmatch(m[1]); rm != nil {
				return resolve(rm[1])
			}
		}
	}
	return ""
}

// injectRepositoryMagicMethods injects findBy*, findOneBy*, countBy* methods
// into a repository class based on the entity's mapped columns.
func injectRepositoryMagicMethods(index *symbols.Index, repoFQN, entityFQN string, entitySym *symbols.Symbol) {
	repoSym := index.Lookup(repoFQN)
	if repoSym == nil {
		return
	}

	// Inject base EntityRepository methods
	baseMethods := []struct {
		name       string
		returnType string
		params     []symbols.ParamInfo
	}{
		{"find", "?" + entityFQN, []symbols.ParamInfo{{Name: "$id", Type: "mixed"}}},
		{"findAll", "array", nil},
		{"findBy", "array", []symbols.ParamInfo{{Name: "$criteria", Type: "array"}, {Name: "$orderBy", Type: "?array"}, {Name: "$limit", Type: "?int"}, {Name: "$offset", Type: "?int"}}},
		{"findOneBy", "?" + entityFQN, []symbols.ParamInfo{{Name: "$criteria", Type: "array"}}},
		{"count", "int", []symbols.ParamInfo{{Name: "$criteria", Type: "array"}}},
	}

	for _, bm := range baseMethods {
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       bm.name,
			FQN:        repoFQN + "::" + bm.name,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: bm.returnType,
			Params:     bm.params,
			IsVirtual:  true,
			DocComment: "EntityRepository method",
		})
	}

	// Collect column names from entity's children
	members := index.GetClassMembers(entityFQN)
	for _, m := range members {
		if m.Kind != symbols.KindProperty {
			continue
		}
		colName := strings.TrimPrefix(m.Name, "$")
		ucName := ucFirst(colName)

		// findByColumnName
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       "findBy" + ucName,
			FQN:        repoFQN + "::findBy" + ucName,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: "array",
			Params:     []symbols.ParamInfo{{Name: "$value", Type: m.Type}},
			IsVirtual:  true,
			DocComment: "Magic finder for " + colName,
		})

		// findOneByColumnName
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       "findOneBy" + ucName,
			FQN:        repoFQN + "::findOneBy" + ucName,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: "?" + entityFQN,
			Params:     []symbols.ParamInfo{{Name: "$value", Type: m.Type}},
			IsVirtual:  true,
			DocComment: "Magic finder for " + colName,
		})

		// countByColumnName
		index.AddVirtualMember(repoFQN, &symbols.Symbol{
			Name:       "countBy" + ucName,
			FQN:        repoFQN + "::countBy" + ucName,
			Kind:       symbols.KindMethod,
			URI:        repoSym.URI,
			Visibility: "public",
			ReturnType: "int",
			Params:     []symbols.ParamInfo{{Name: "$value", Type: m.Type}},
			IsVirtual:  true,
			DocComment: "Magic counter for " + colName,
		})
	}
}

// --- XML Mapping Fallback ---

// analyzeDoctrineXML looks for XML mapping files in config/doctrine/ and parses them.
func analyzeDoctrineXML(index *symbols.Index, entitySym *symbols.Symbol, rootPath, entityFQN string, cls *parser.ClassNode, resolve func(string) string) {
	if rootPath == "" {
		return
	}

	// Look for XML mapping files
	xmlPaths := []string{
		filepath.Join(rootPath, "config", "doctrine"),
		filepath.Join(rootPath, "config", "packages", "doctrine"),
	}

	for _, dir := range xmlPaths {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".orm.xml") {
				continue
			}
			xmlPath := filepath.Join(dir, entry.Name())
			parseDoctrineXMLMapping(index, entitySym, entityFQN, xmlPath, resolve)
		}
	}
}

// XML mapping structures
type doctrineMappingXML struct {
	XMLName xml.Name           `xml:"doctrine-mapping"`
	Entity  *doctrineEntityXML `xml:"entity"`
}

type doctrineEntityXML struct {
	Name       string                 `xml:"name,attr"`
	Table      string                 `xml:"table,attr"`
	Fields     []doctrineFieldXML     `xml:"field"`
	Id         []doctrineFieldXML     `xml:"id"`
	OneToOne   []doctrineRelationXML  `xml:"one-to-one"`
	OneToMany  []doctrineRelationXML  `xml:"one-to-many"`
	ManyToOne  []doctrineRelationXML  `xml:"many-to-one"`
	ManyToMany []doctrineRelationXML  `xml:"many-to-many"`
}

type doctrineFieldXML struct {
	Name   string `xml:"name,attr"`
	Type   string `xml:"type,attr"`
	Column string `xml:"column,attr"`
}

type doctrineRelationXML struct {
	Field        string `xml:"field,attr"`
	TargetEntity string `xml:"target-entity,attr"`
	MappedBy     string `xml:"mapped-by,attr"`
}

func parseDoctrineXMLMapping(index *symbols.Index, entitySym *symbols.Symbol, entityFQN, xmlPath string, resolve func(string) string) {
	data, err := os.ReadFile(xmlPath)
	if err != nil {
		return
	}

	var mapping doctrineMappingXML
	if err := xml.Unmarshal(data, &mapping); err != nil {
		return
	}

	if mapping.Entity == nil || mapping.Entity.Name != entityFQN {
		return
	}

	entity := mapping.Entity

	// Process ID fields
	for _, field := range entity.Id {
		phpType := "mixed"
		if t, ok := doctrineTypeMap[field.Type]; ok {
			phpType = t
		}
		propName := "$" + field.Name
		index.AddVirtualMember(entityFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        entityFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        entitySym.URI,
			Visibility: "private",
			Type:       phpType,
			IsVirtual:  true,
			IsReadonly: true,
			DocComment: "Doctrine ID column",
		})
	}

	// Process regular fields
	for _, field := range entity.Fields {
		phpType := "mixed"
		if t, ok := doctrineTypeMap[field.Type]; ok {
			phpType = t
		}
		propName := "$" + field.Name
		index.AddVirtualMember(entityFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        entityFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        entitySym.URI,
			Visibility: "private",
			Type:       phpType,
			IsVirtual:  true,
			DocComment: "Doctrine column",
		})
	}

	// Process relations
	for _, rel := range entity.OneToOne {
		target := resolve(rel.TargetEntity)
		propName := "$" + rel.Field
		index.AddVirtualMember(entityFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        entityFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        entitySym.URI,
			Visibility: "private",
			Type:       "?" + target,
			IsVirtual:  true,
			DocComment: "OneToOne relation",
		})
	}
	for _, rel := range entity.ManyToOne {
		target := resolve(rel.TargetEntity)
		propName := "$" + rel.Field
		index.AddVirtualMember(entityFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        entityFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        entitySym.URI,
			Visibility: "private",
			Type:       "?" + target,
			IsVirtual:  true,
			DocComment: "ManyToOne relation",
		})
	}
	for _, rel := range entity.OneToMany {
		propName := "$" + rel.Field
		index.AddVirtualMember(entityFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        entityFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        entitySym.URI,
			Visibility: "private",
			Type:       "Doctrine\\Common\\Collections\\Collection",
			IsVirtual:  true,
			DocComment: "OneToMany relation",
		})
	}
	for _, rel := range entity.ManyToMany {
		propName := "$" + rel.Field
		index.AddVirtualMember(entityFQN, &symbols.Symbol{
			Name:       propName,
			FQN:        entityFQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        entitySym.URI,
			Visibility: "private",
			Type:       "Doctrine\\Common\\Collections\\Collection",
			IsVirtual:  true,
			DocComment: "ManyToMany relation",
		})
	}
}
