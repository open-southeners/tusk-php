package models

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	// Database drivers
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

const connectTimeout = 5 * time.Second

// SchemaColumn represents a single column from the database schema.
type SchemaColumn struct {
	Name       string
	DataType   string
	IsNullable bool
	ColumnType string // full type, e.g. "tinyint(1)"
}

// SchemaCache holds cached schema results per table.
type SchemaCache struct {
	mu     sync.RWMutex
	tables map[string][]SchemaColumn // tableName → columns
}

func NewSchemaCache() *SchemaCache {
	return &SchemaCache{tables: make(map[string][]SchemaColumn)}
}

func (sc *SchemaCache) Get(table string) ([]SchemaColumn, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	cols, ok := sc.tables[table]
	return cols, ok
}

func (sc *SchemaCache) Set(table string, cols []SchemaColumn) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.tables[table] = cols
}

func (sc *SchemaCache) Invalidate() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.tables = make(map[string][]SchemaColumn)
}

// SQL type → PHP type mapping.
var sqlTypeMap = map[string]string{
	"varchar":    "string",
	"char":       "string",
	"text":       "string",
	"tinytext":   "string",
	"mediumtext": "string",
	"longtext":   "string",
	"enum":       "string",
	"set":        "string",
	"int":        "int",
	"integer":    "int",
	"bigint":     "int",
	"smallint":   "int",
	"tinyint":    "int",
	"mediumint":  "int",
	"serial":     "int",
	"bigserial":  "int",
	"decimal":    "string",
	"numeric":    "string",
	"float":      "float",
	"double":     "float",
	"real":       "float",
	"datetime":   "\\DateTimeInterface",
	"timestamp":  "\\DateTimeInterface",
	"date":       "\\DateTimeInterface",
	"time":       "string",
	"json":       "array",
	"jsonb":      "array",
	"blob":       "string",
	"binary":     "string",
	"varbinary":  "string",
	"bytea":      "string",
	"uuid":       "string",
	"boolean":    "bool",
	"bool":       "bool",
}

// tinyint(1) detection pattern
var tinyint1Re = regexp.MustCompile(`(?i)tinyint\(\s*1\s*\)`)

// mapSQLTypeToPhp converts a SQL column type to PHP type.
func mapSQLTypeToPhp(dataType, columnType string) string {
	dt := strings.ToLower(dataType)
	// tinyint(1) is a boolean in PHP
	if dt == "tinyint" && tinyint1Re.MatchString(columnType) {
		return "bool"
	}
	if phpType, ok := sqlTypeMap[dt]; ok {
		return phpType
	}
	return "mixed"
}

// AnalyzeDatabaseSchema connects to the project's database and injects virtual
// properties for model/entity columns discovered from the schema.
func AnalyzeDatabaseSchema(index *symbols.Index, rootPath, framework string, cfg *config.Config, logger *log.Logger, cache *SchemaCache) {
	if cfg != nil && !cfg.IsDatabaseEnabled() {
		return
	}

	var dbCfg *config.DatabaseConfig
	switch framework {
	case "laravel":
		dbCfg = config.ParseLaravelDatabaseConfig(rootPath)
	case "symfony":
		dbCfg = config.ParseSymfonyDatabaseConfig(rootPath)
	default:
		return
	}

	if dbCfg == nil || dbCfg.Database == "" {
		return
	}

	db, err := openDatabase(dbCfg)
	if err != nil {
		// Silently fail — database may not be running
		if logger != nil {
			logger.Printf("Database connection failed (will use fallback sources): %v", err)
		}
		return
	}
	defer db.Close()

	if logger != nil {
		logger.Printf("Connected to %s database: %s", dbCfg.Driver, dbCfg.Database)
	}

	switch framework {
	case "laravel":
		injectLaravelSchemaProperties(index, db, dbCfg, rootPath, cache)
	case "symfony":
		injectSymfonySchemaProperties(index, db, dbCfg, rootPath, cache)
	}
}

func openDatabase(cfg *config.DatabaseConfig) (*sql.DB, error) {
	var dsn, driverName string

	switch cfg.Driver {
	case "mysql":
		driverName = "mysql"
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?timeout=%s",
			cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
			connectTimeout.String())
	case "pgsql":
		driverName = "postgres"
		dsn = fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=5",
			cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.Database)
	case "sqlite":
		driverName = "sqlite"
		dsn = cfg.Database
	default:
		return nil, fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}

	// Verify connection with timeout
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	// Single connection — we just query schema then close
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)

	return db, nil
}

func queryColumns(db *sql.DB, dbName, tableName string) ([]SchemaColumn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_TYPE
		 FROM INFORMATION_SCHEMA.COLUMNS
		 WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
		 ORDER BY ORDINAL_POSITION`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []SchemaColumn
	for rows.Next() {
		var c SchemaColumn
		var nullable string
		if err := rows.Scan(&c.Name, &c.DataType, &nullable, &c.ColumnType); err != nil {
			continue
		}
		c.IsNullable = nullable == "YES"
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// queryColumnsPostgres uses PostgreSQL's information_schema with $1/$2 params.
func queryColumnsPostgres(db *sql.DB, dbName, tableName string) ([]SchemaColumn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable, udt_name
		 FROM information_schema.columns
		 WHERE table_catalog = $1 AND table_name = $2
		 ORDER BY ordinal_position`, dbName, tableName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []SchemaColumn
	for rows.Next() {
		var c SchemaColumn
		var nullable string
		if err := rows.Scan(&c.Name, &c.DataType, &nullable, &c.ColumnType); err != nil {
			continue
		}
		c.IsNullable = strings.ToUpper(nullable) == "YES"
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// queryColumnsSQLite uses PRAGMA to get column info.
func queryColumnsSQLite(db *sql.DB, tableName string) ([]SchemaColumn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []SchemaColumn
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dflt, &pk); err != nil {
			continue
		}
		cols = append(cols, SchemaColumn{
			Name:       name,
			DataType:   strings.ToLower(dataType),
			IsNullable: notNull == 0 && pk == 0,
			ColumnType: dataType,
		})
	}
	return cols, rows.Err()
}

func getTableColumns(db *sql.DB, cfg *config.DatabaseConfig, tableName string, cache *SchemaCache) []SchemaColumn {
	if cols, ok := cache.Get(tableName); ok {
		return cols
	}

	var cols []SchemaColumn
	var err error
	switch cfg.Driver {
	case "mysql":
		cols, err = queryColumns(db, cfg.Database, tableName)
	case "pgsql":
		cols, err = queryColumnsPostgres(db, cfg.Database, tableName)
	case "sqlite":
		cols, err = queryColumnsSQLite(db, tableName)
	}

	if err != nil || cols == nil {
		return nil
	}

	cache.Set(tableName, cols)
	return cols
}

// injectLaravelSchemaProperties resolves table names for Laravel models and injects columns.
func injectLaravelSchemaProperties(index *symbols.Index, db *sql.DB, cfg *config.DatabaseConfig, rootPath string, cache *SchemaCache) {
	models := index.GetDescendants("Illuminate\\Database\\Eloquent\\Model")
	for _, model := range models {
		tableName := resolveModelTableName(index, model, rootPath)
		if tableName == "" {
			continue
		}

		cols := getTableColumns(db, cfg, tableName, cache)
		injectColumnsAsProperties(index, model, cols)
	}
}

// injectSymfonySchemaProperties resolves table names for Doctrine entities and injects columns.
func injectSymfonySchemaProperties(index *symbols.Index, db *sql.DB, cfg *config.DatabaseConfig, rootPath string, cache *SchemaCache) {
	uris := index.GetAllFileURIs()
	for _, uri := range uris {
		path := symbols.URIToPath(uri)
		content, err := readFileQuick(path)
		if err != nil || !strings.Contains(content, "ORM\\Entity") {
			continue
		}

		file := parser.ParseFile(content)
		if file == nil {
			continue
		}

		for _, cls := range file.Classes {
			fqn := file.Namespace + "\\" + cls.Name
			if file.Namespace == "" {
				fqn = cls.Name
			}
			sym := index.Lookup(fqn)
			if sym == nil {
				continue
			}

			tableName := resolveDoctrineTableName(content, &cls, fqn)
			if tableName == "" {
				continue
			}

			cols := getTableColumns(db, cfg, tableName, cache)
			injectColumnsAsProperties(index, sym, cols)
		}
	}
}

func injectColumnsAsProperties(index *symbols.Index, classSym *symbols.Symbol, cols []SchemaColumn) {
	for _, col := range cols {
		propName := "$" + col.Name
		phpType := mapSQLTypeToPhp(col.DataType, col.ColumnType)
		if col.IsNullable && phpType != "mixed" {
			phpType = "?" + phpType
		}

		existing := index.Lookup(classSym.FQN + "::" + propName)
		if existing != nil {
			// Don't overwrite — other sources have priority
			continue
		}

		index.AddVirtualMember(classSym.FQN, &symbols.Symbol{
			Name:       propName,
			FQN:        classSym.FQN + "::" + propName,
			Kind:       symbols.KindProperty,
			URI:        classSym.URI,
			Visibility: "public",
			Type:       phpType,
			IsVirtual:  true,
			DocComment: fmt.Sprintf("(database column) %s — %s", col.DataType, col.ColumnType),
		})
	}
}

// resolveModelTableName determines the database table name for a Laravel Eloquent model.
func resolveModelTableName(index *symbols.Index, model *symbols.Symbol, rootPath string) string {
	// Check for explicit $table property
	tableProp := index.Lookup(model.FQN + "::$table")
	if tableProp != nil && tableProp.Value != "" {
		return strings.Trim(tableProp.Value, "'\"")
	}

	// Read source file and look for $table = 'name'
	path := symbols.URIToPath(model.URI)
	content, err := readFileQuick(path)
	if err == nil {
		if m := tablePropertyRe.FindStringSubmatch(content); m != nil {
			return m[1]
		}
	}

	// Convention: snake_case plural of class name
	return pluralize(toSnakeCase(model.Name))
}

// resolveDoctrineTableName determines the table name for a Doctrine entity.
func resolveDoctrineTableName(source string, cls *parser.ClassNode, fqn string) string {
	// Check #[ORM\Table(name: 'tablename')]
	if m := ormTableRe.FindStringSubmatch(source); m != nil {
		if nm := nameArgRe.FindStringSubmatch(m[1]); nm != nil {
			return nm[1]
		}
	}
	// Convention: snake_case of class name
	return toSnakeCase(cls.Name)
}

var tablePropertyRe = regexp.MustCompile(`\$table\s*=\s*['"]([^'"]+)['"]`)

// toSnakeCase converts CamelCase to snake_case.
func toSnakeCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// pluralize applies simple English pluralization rules.
func pluralize(s string) string {
	if s == "" {
		return s
	}
	if strings.HasSuffix(s, "y") && len(s) > 1 && !isVowel(s[len(s)-2]) {
		return s[:len(s)-1] + "ies"
	}
	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "sh") ||
		strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "x") ||
		strings.HasSuffix(s, "z") {
		return s + "es"
	}
	return s + "s"
}

func isVowel(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

func readFileQuick(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
