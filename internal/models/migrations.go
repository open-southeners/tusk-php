package models

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

// MigrationColumn represents a column discovered from a migration file.
type MigrationColumn struct {
	Name     string
	Type     string // PHP type
	Nullable bool
	Default  string
}

// Blueprint method → PHP type mapping.
var blueprintTypeMap = map[string]string{
	"string":           "string",
	"text":             "string",
	"mediumText":       "string",
	"longText":         "string",
	"char":             "string",
	"integer":          "int",
	"tinyInteger":      "int",
	"smallInteger":     "int",
	"mediumInteger":    "int",
	"bigInteger":       "int",
	"unsignedInteger":  "int",
	"unsignedTinyInteger":   "int",
	"unsignedSmallInteger":  "int",
	"unsignedMediumInteger": "int",
	"unsignedBigInteger":    "int",
	"boolean":          "bool",
	"decimal":          "string",
	"unsignedDecimal":  "string",
	"float":            "float",
	"double":           "float",
	"date":             "\\DateTimeInterface",
	"dateTime":         "\\DateTimeInterface",
	"dateTimeTz":       "\\DateTimeInterface",
	"timestamp":        "\\DateTimeInterface",
	"timestampTz":      "\\DateTimeInterface",
	"time":             "string",
	"timeTz":           "string",
	"year":             "int",
	"json":             "array",
	"jsonb":            "array",
	"binary":           "string",
	"uuid":             "string",
	"ulid":             "string",
	"ipAddress":        "string",
	"macAddress":       "string",
	"enum":             "string",
	"set":              "string",
	"foreignId":        "int",
	"foreignUlid":      "string",
	"foreignUuid":      "string",
}

// Regex patterns for migration parsing.
var (
	// Schema::create('tablename', function (Blueprint $table) {
	schemaCreateRe = regexp.MustCompile(`Schema::create\(\s*['"]([^'"]+)['"]`)
	// Schema::table('tablename', function (Blueprint $table) {
	schemaTableRe = regexp.MustCompile(`Schema::table\(\s*['"]([^'"]+)['"]`)
	// $table->string('name') or $table->string('name', 100)
	blueprintCallRe = regexp.MustCompile(`\$table\s*->\s*(\w+)\(\s*'([^']+)'`)
	// ->nullable() chain
	nullableRe = regexp.MustCompile(`->\s*nullable\s*\(`)
	// ->default(value) chain
	defaultRe = regexp.MustCompile(`->\s*default\s*\(\s*(.+?)\s*\)`)
	// $table->dropColumn('name') or $table->dropColumn(['name', 'other'])
	dropColumnRe = regexp.MustCompile(`\$table\s*->\s*dropColumn\(\s*['"\[]`)
	dropColumnSingleRe = regexp.MustCompile(`\$table\s*->\s*dropColumn\(\s*'([^']+)'`)
	dropColumnArrayRe  = regexp.MustCompile(`'([^']+)'`)
	// $table->timestamps() — no column name arg
	timestampsRe = regexp.MustCompile(`\$table\s*->\s*timestamps\s*\(`)
	// $table->softDeletes() — no column name arg
	softDeletesRe = regexp.MustCompile(`\$table\s*->\s*softDeletes\s*\(`)
	// $table->id() — no column name arg (or optional name)
	idRe = regexp.MustCompile(`\$table\s*->\s*id\s*\((?:\s*'([^']*)')?\s*\)`)
	// $table->rememberToken()
	rememberTokenRe = regexp.MustCompile(`\$table\s*->\s*rememberToken\s*\(`)
)

// AnalyzeMigrations scans Laravel migration files and injects virtual properties
// for columns that don't already have a source from DB, IDE helper, or docblocks.
func AnalyzeMigrations(index *symbols.Index, rootPath string) {
	migrationsDir := filepath.Join(rootPath, "database", "migrations")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return
	}

	// Sort by filename (timestamped, so lexicographic order = chronological)
	var migrationFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".php") {
			migrationFiles = append(migrationFiles, filepath.Join(migrationsDir, entry.Name()))
		}
	}
	sort.Strings(migrationFiles)

	// Build table schemas from migrations
	tableColumns := make(map[string]map[string]*MigrationColumn)

	for _, path := range migrationFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parseMigrationFile(string(content), tableColumns)
	}

	// Map tables to models and inject virtual properties
	modelsByTable := buildModelTableMap(index, rootPath)

	for tableName, columns := range tableColumns {
		modelFQN, ok := modelsByTable[tableName]
		if !ok {
			continue
		}
		modelSym := index.Lookup(modelFQN)
		if modelSym == nil {
			continue
		}

		for _, col := range columns {
			propName := "$" + col.Name
			// Skip if already discovered from another source
			if index.Lookup(modelFQN+"::"+propName) != nil {
				continue
			}

			phpType := col.Type
			if col.Nullable && phpType != "mixed" {
				phpType = "?" + phpType
			}

			docComment := "From migration"
			if col.Default != "" {
				docComment += ", default: " + col.Default
			}

			index.AddVirtualMember(modelFQN, &symbols.Symbol{
				Name:       propName,
				FQN:        modelFQN + "::" + propName,
				Kind:       symbols.KindProperty,
				URI:        modelSym.URI,
				Visibility: "public",
				Type:       phpType,
				IsVirtual:  true,
				DocComment: docComment,
			})
		}
	}
}

// parseMigrationFile extracts column definitions from a single migration file.
func parseMigrationFile(source string, tableColumns map[string]map[string]*MigrationColumn) {
	lines := strings.Split(source, "\n")

	var currentTable string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect Schema::create or Schema::table
		if m := schemaCreateRe.FindStringSubmatch(trimmed); m != nil {
			currentTable = m[1]
			if tableColumns[currentTable] == nil {
				tableColumns[currentTable] = make(map[string]*MigrationColumn)
			}
			continue
		}
		if m := schemaTableRe.FindStringSubmatch(trimmed); m != nil {
			currentTable = m[1]
			if tableColumns[currentTable] == nil {
				tableColumns[currentTable] = make(map[string]*MigrationColumn)
			}
			continue
		}

		if currentTable == "" {
			continue
		}
		cols := tableColumns[currentTable]

		// Handle $table->id()
		if m := idRe.FindStringSubmatch(trimmed); m != nil {
			name := "id"
			if m[1] != "" {
				name = m[1]
			}
			cols[name] = &MigrationColumn{Name: name, Type: "int"}
			continue
		}

		// Handle $table->timestamps()
		if timestampsRe.MatchString(trimmed) {
			cols["created_at"] = &MigrationColumn{Name: "created_at", Type: "\\DateTimeInterface", Nullable: true}
			cols["updated_at"] = &MigrationColumn{Name: "updated_at", Type: "\\DateTimeInterface", Nullable: true}
			continue
		}

		// Handle $table->softDeletes()
		if softDeletesRe.MatchString(trimmed) {
			cols["deleted_at"] = &MigrationColumn{Name: "deleted_at", Type: "\\DateTimeInterface", Nullable: true}
			continue
		}

		// Handle $table->rememberToken()
		if rememberTokenRe.MatchString(trimmed) {
			cols["remember_token"] = &MigrationColumn{Name: "remember_token", Type: "string", Nullable: true}
			continue
		}

		// Handle $table->dropColumn(...)
		if strings.Contains(trimmed, "dropColumn") {
			if m := dropColumnSingleRe.FindStringSubmatch(trimmed); m != nil {
				delete(cols, m[1])
			} else if strings.Contains(trimmed, "[") {
				// Array form: dropColumn(['col1', 'col2'])
				for _, m := range dropColumnArrayRe.FindAllStringSubmatch(trimmed, -1) {
					delete(cols, m[1])
				}
			}
			continue
		}

		// Handle $table->type('name') blueprint calls
		if m := blueprintCallRe.FindStringSubmatch(trimmed); m != nil {
			method := m[1]
			colName := m[2]

			phpType, ok := blueprintTypeMap[method]
			if !ok {
				continue
			}

			col := &MigrationColumn{Name: colName, Type: phpType}

			// Check for ->nullable()
			if nullableRe.MatchString(trimmed) {
				col.Nullable = true
			}

			// Check for ->default(value)
			if dm := defaultRe.FindStringSubmatch(trimmed); dm != nil {
				col.Default = dm[1]
			}

			cols[colName] = col
		}
	}
}

// buildModelTableMap creates a mapping from table names to model FQNs.
func buildModelTableMap(index *symbols.Index, rootPath string) map[string]string {
	result := make(map[string]string)
	models := index.GetDescendants("Illuminate\\Database\\Eloquent\\Model")
	for _, model := range models {
		tableName := resolveModelTableName(index, model, rootPath)
		if tableName != "" {
			result[tableName] = model.FQN
		}
	}
	return result
}
