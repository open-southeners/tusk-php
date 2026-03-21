package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	tmpDir := t.TempDir()
	envPath := filepath.Join(tmpDir, ".env")
	content := `# Database config
DB_CONNECTION=mysql
DB_HOST=127.0.0.1
DB_PORT=3306
DB_DATABASE=myapp
DB_USERNAME=root
DB_PASSWORD="secret123"

APP_KEY=base64:somekey
QUOTED_SINGLE='single quoted'
`
	if err := os.WriteFile(envPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	env, err := ParseEnvFile(envPath)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"DB_CONNECTION": "mysql",
		"DB_HOST":       "127.0.0.1",
		"DB_PORT":       "3306",
		"DB_DATABASE":   "myapp",
		"DB_USERNAME":   "root",
		"DB_PASSWORD":   "secret123",
		"APP_KEY":       "base64:somekey",
		"QUOTED_SINGLE": "single quoted",
	}

	for key, expected := range tests {
		if env[key] != expected {
			t.Errorf("%s: expected %q, got %q", key, expected, env[key])
		}
	}
}

func TestParseLaravelDatabaseConfig(t *testing.T) {
	tmpDir := t.TempDir()
	envContent := `DB_CONNECTION=mysql
DB_HOST=localhost
DB_PORT=3307
DB_DATABASE=testdb
DB_USERNAME=testuser
DB_PASSWORD=testpass
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := ParseLaravelDatabaseConfig(tmpDir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Driver != "mysql" {
		t.Errorf("expected driver 'mysql', got %q", cfg.Driver)
	}
	if cfg.Host != "localhost" {
		t.Errorf("expected host 'localhost', got %q", cfg.Host)
	}
	if cfg.Port != "3307" {
		t.Errorf("expected port '3307', got %q", cfg.Port)
	}
	if cfg.Database != "testdb" {
		t.Errorf("expected database 'testdb', got %q", cfg.Database)
	}
}

func TestParseSymfonyDatabaseConfig(t *testing.T) {
	tmpDir := t.TempDir()
	envContent := `DATABASE_URL="mysql://dbuser:dbpass@127.0.0.1:3306/symfony_app"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSymfonyDatabaseConfig(tmpDir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Driver != "mysql" {
		t.Errorf("expected driver 'mysql', got %q", cfg.Driver)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("expected host '127.0.0.1', got %q", cfg.Host)
	}
	if cfg.Username != "dbuser" {
		t.Errorf("expected username 'dbuser', got %q", cfg.Username)
	}
	if cfg.Database != "symfony_app" {
		t.Errorf("expected database 'symfony_app', got %q", cfg.Database)
	}
}

func TestParseSymfonyPostgresURL(t *testing.T) {
	tmpDir := t.TempDir()
	envContent := `DATABASE_URL="postgresql://pguser:pgpass@db.host:5432/mydb?serverVersion=15"
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := ParseSymfonyDatabaseConfig(tmpDir)
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if cfg.Driver != "pgsql" {
		t.Errorf("expected driver 'pgsql', got %q", cfg.Driver)
	}
	if cfg.Port != "5432" {
		t.Errorf("expected port '5432', got %q", cfg.Port)
	}
}

func TestNormalizeDatabaseDriver(t *testing.T) {
	tests := map[string]string{
		"mysql":      "mysql",
		"mariadb":    "mysql",
		"pgsql":      "pgsql",
		"postgres":   "pgsql",
		"postgresql": "pgsql",
		"sqlite":     "sqlite",
		"sqlite3":    "sqlite",
	}
	for input, expected := range tests {
		if got := normalizeDatabaseDriver(input); got != expected {
			t.Errorf("normalizeDatabaseDriver(%q) = %q, want %q", input, got, expected)
		}
	}
}
