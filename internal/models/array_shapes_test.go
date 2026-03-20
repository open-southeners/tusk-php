package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/php-lsp/internal/symbols"
)

func setupTestProject(t *testing.T) (string, *symbols.Index) {
	t.Helper()
	dir := t.TempDir()

	// Create config/app.php
	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "config", "app.php"), []byte(`<?php
return [
    'name' => 'My App',
    'debug' => true,
    'url' => 'http://localhost',
    'timezone' => 'UTC',
    'database' => [
        'host' => 'localhost',
        'port' => 3306,
    ],
];
`), 0644)

	// Create config/database.php
	os.WriteFile(filepath.Join(dir, "config", "database.php"), []byte(`<?php
return [
    'default' => 'mysql',
    'connections' => [
        'mysql' => [
            'host' => 'localhost',
            'port' => 3306,
            'database' => 'myapp',
        ],
    ],
];
`), 0644)

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	return dir, idx
}

func TestLaravelConfigTopLevelKeys(t *testing.T) {
	dir, idx := setupTestProject(t)
	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// config('app') should return top-level keys from config/app.php
	fields := r.ResolveCallReturnKeys("config('app')", "")
	if len(fields) == 0 {
		t.Fatal("expected keys from config/app.php")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["name"] {
		t.Error("expected 'name' key")
	}
	if !keys["debug"] {
		t.Error("expected 'debug' key")
	}
	if !keys["url"] {
		t.Error("expected 'url' key")
	}
	if !keys["database"] {
		t.Error("expected 'database' key")
	}
}

func TestLaravelConfigNestedKeys(t *testing.T) {
	dir, idx := setupTestProject(t)
	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// config('app.database') should drill into the nested array
	fields := r.ResolveCallReturnKeys("config('app.database')", "")
	if len(fields) == 0 {
		t.Fatal("expected nested keys from config/app.php database section")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["host"] {
		t.Error("expected 'host' from database section")
	}
	if !keys["port"] {
		t.Error("expected 'port' from database section")
	}
}

func TestLaravelConfigListFiles(t *testing.T) {
	dir, idx := setupTestProject(t)
	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// config() with no specific file — list available config files
	fields := r.ListConfigFiles()
	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["app"] {
		t.Error("expected 'app' config file")
	}
	if !keys["database"] {
		t.Error("expected 'database' config file")
	}
}

func TestLaravelFormRequestKeys(t *testing.T) {
	dir, idx := setupTestProject(t)

	// Create a form request with rules()
	os.MkdirAll(filepath.Join(dir, "app", "Http", "Requests"), 0755)
	requestPath := filepath.Join(dir, "app", "Http", "Requests", "StoreUserRequest.php")
	os.WriteFile(requestPath, []byte(`<?php
namespace App\Http\Requests;

class StoreUserRequest {
    public function rules(): array {
        return [
            'name' => 'required|string',
            'email' => 'required|email',
            'password' => 'required|min:8',
        ];
    }
}
`), 0644)

	idx.IndexFileWithSource("file://"+requestPath, string(mustReadFile(t, requestPath)), symbols.SourceProject)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")
	fields := r.ResolveMethodReturnKeys("App\\Http\\Requests\\StoreUserRequest", "validated")

	if len(fields) == 0 {
		t.Fatal("expected keys from form request rules()")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["name"] {
		t.Error("expected 'name' from rules()")
	}
	if !keys["email"] {
		t.Error("expected 'email' from rules()")
	}
	if !keys["password"] {
		t.Error("expected 'password' from rules()")
	}
}

func TestLaravelModelToArrayKeys(t *testing.T) {
	dir, idx := setupTestProject(t)

	idx.IndexFileWithSource("file:///user.php", `<?php
namespace App\Models;
class User {
    public string $name;
    public string $email;
    public int $age;
}
`, symbols.SourceProject)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")
	fields := r.ResolveMethodReturnKeys("App\\Models\\User", "toArray")

	if len(fields) == 0 {
		t.Fatal("expected keys from model properties")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["name"] {
		t.Error("expected 'name' property")
	}
	if !keys["email"] {
		t.Error("expected 'email' property")
	}
	if !keys["age"] {
		t.Error("expected 'age' property")
	}
}

func TestNonLaravelReturnsNil(t *testing.T) {
	dir, idx := setupTestProject(t)
	r := NewFrameworkArrayResolver(idx, dir, "none")

	fields := r.ResolveCallReturnKeys("config('app')", "")
	if fields != nil {
		t.Error("expected nil for non-laravel framework")
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
