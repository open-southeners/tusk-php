package config

import (
	"net/url"
	"os"
	"strings"
)

// DatabaseConfig holds parsed database connection parameters.
type DatabaseConfig struct {
	Driver   string // mysql, pgsql, sqlite
	Host     string
	Port     string
	Database string
	Username string
	Password string
}

// ParseEnvFile reads a .env file and returns key-value pairs.
// Supports simple KEY=VALUE, quoted values, and comments.
func ParseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseEnvString(string(data)), nil
}

func parseEnvString(content string) map[string]string {
	env := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		// Strip surrounding quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}
		env[key] = value
	}
	return env
}

// ParseLaravelDatabaseConfig extracts database config from .env files in a Laravel project.
func ParseLaravelDatabaseConfig(rootPath string) *DatabaseConfig {
	env := loadEnvFiles(rootPath, ".env", ".env.local")
	if len(env) == 0 {
		return nil
	}

	driver := env["DB_CONNECTION"]
	if driver == "" {
		return nil
	}

	return &DatabaseConfig{
		Driver:   normalizeDatabaseDriver(driver),
		Host:     envOrDefault(env, "DB_HOST", "127.0.0.1"),
		Port:     envOrDefault(env, "DB_PORT", defaultPort(driver)),
		Database: env["DB_DATABASE"],
		Username: env["DB_USERNAME"],
		Password: env["DB_PASSWORD"],
	}
}

// ParseSymfonyDatabaseConfig extracts database config from Symfony .env files.
func ParseSymfonyDatabaseConfig(rootPath string) *DatabaseConfig {
	env := loadEnvFiles(rootPath, ".env", ".env.local")
	if len(env) == 0 {
		return nil
	}

	dbURL := env["DATABASE_URL"]
	if dbURL == "" {
		return nil
	}

	return parseDatabaseURL(dbURL)
}

// parseDatabaseURL parses a DATABASE_URL like mysql://user:pass@host:port/dbname
func parseDatabaseURL(rawURL string) *DatabaseConfig {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	cfg := &DatabaseConfig{
		Driver:   normalizeDatabaseDriver(u.Scheme),
		Host:     u.Hostname(),
		Port:     u.Port(),
		Database: strings.TrimPrefix(u.Path, "/"),
	}

	if u.User != nil {
		cfg.Username = u.User.Username()
		cfg.Password, _ = u.User.Password()
	}

	if cfg.Port == "" {
		cfg.Port = defaultPort(cfg.Driver)
	}

	return cfg
}

func loadEnvFiles(rootPath string, filenames ...string) map[string]string {
	merged := make(map[string]string)
	for _, name := range filenames {
		path := rootPath + "/" + name
		if env, err := ParseEnvFile(path); err == nil {
			for k, v := range env {
				merged[k] = v
			}
		}
	}
	return merged
}

func envOrDefault(env map[string]string, key, def string) string {
	if v, ok := env[key]; ok && v != "" {
		return v
	}
	return def
}

func normalizeDatabaseDriver(driver string) string {
	switch strings.ToLower(driver) {
	case "mysql", "mariadb":
		return "mysql"
	case "pgsql", "postgres", "postgresql":
		return "pgsql"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return driver
	}
}

func defaultPort(driver string) string {
	switch normalizeDatabaseDriver(driver) {
	case "mysql":
		return "3306"
	case "pgsql":
		return "5432"
	default:
		return ""
	}
}
