package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
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

func TestVendorConfigDiscoveryFromConfigDir(t *testing.T) {
	dir, idx := setupTestProject(t)

	// Create a vendor package with a config file
	vendorConfigDir := filepath.Join(dir, "vendor", "laravel", "sanctum", "config")
	os.MkdirAll(vendorConfigDir, 0755)
	os.WriteFile(filepath.Join(vendorConfigDir, "sanctum.php"), []byte(`<?php
return [
    'guard' => ['web'],
    'expiration' => null,
    'prefix' => 'sanctum',
];
`), 0644)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// Should discover sanctum from vendor
	fields := r.ListConfigFiles()
	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["sanctum"] {
		t.Error("expected 'sanctum' from vendor config")
	}
	if !keys["app"] {
		t.Error("expected 'app' from project config")
	}
}

func TestVendorConfigDiscoveryFromServiceProvider(t *testing.T) {
	dir, idx := setupTestProject(t)

	// Create a vendor package with ServiceProvider that declares mergeConfigFrom
	pkgDir := filepath.Join(dir, "vendor", "laravel", "horizon", "src")
	os.MkdirAll(pkgDir, 0755)
	configDir := filepath.Join(dir, "vendor", "laravel", "horizon", "config")
	os.MkdirAll(configDir, 0755)

	os.WriteFile(filepath.Join(configDir, "horizon.php"), []byte(`<?php
return [
    'use' => 'default',
    'prefix' => 'horizon:',
    'waits' => ['redis:default' => 60],
];
`), 0644)

	os.WriteFile(filepath.Join(pkgDir, "HorizonServiceProvider.php"), []byte(`<?php
namespace Laravel\Horizon;

class HorizonServiceProvider {
    public function register() {
        $this->mergeConfigFrom(__DIR__.'/../config/horizon.php', 'horizon');
    }
}
`), 0644)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// Should discover horizon from ServiceProvider's mergeConfigFrom
	fields := r.ListConfigFiles()
	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["horizon"] {
		t.Error("expected 'horizon' from vendor ServiceProvider mergeConfigFrom")
	}
}

func TestVendorConfigParsing(t *testing.T) {
	dir, idx := setupTestProject(t)

	// Create a vendor package config
	vendorConfigDir := filepath.Join(dir, "vendor", "laravel", "sanctum", "config")
	os.MkdirAll(vendorConfigDir, 0755)
	os.WriteFile(filepath.Join(vendorConfigDir, "sanctum.php"), []byte(`<?php
return [
    'guard' => ['web'],
    'expiration' => null,
    'prefix' => 'sanctum',
];
`), 0644)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// Should parse vendor config file
	fields := r.ParseConfigFile("sanctum")
	if len(fields) == 0 {
		t.Fatal("expected keys from vendor sanctum config")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["guard"] {
		t.Error("expected 'guard' from sanctum config")
	}
	if !keys["expiration"] {
		t.Error("expected 'expiration' from sanctum config")
	}
	if !keys["prefix"] {
		t.Error("expected 'prefix' from sanctum config")
	}
}

func TestVendorConfigDotNotation(t *testing.T) {
	dir, idx := setupTestProject(t)

	vendorConfigDir := filepath.Join(dir, "vendor", "laravel", "sanctum", "config")
	os.MkdirAll(vendorConfigDir, 0755)
	os.WriteFile(filepath.Join(vendorConfigDir, "sanctum.php"), []byte(`<?php
return [
    'guard' => ['web'],
    'middleware' => [
        'verify_csrf_token' => true,
        'encrypt_cookies' => true,
    ],
];
`), 0644)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// config('sanctum.middleware') should drill into vendor config
	fields := r.ResolveCallReturnKeys("config('sanctum.middleware')", "")
	if len(fields) == 0 {
		t.Fatal("expected nested keys from vendor sanctum.middleware")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	if !keys["verify_csrf_token"] {
		t.Error("expected 'verify_csrf_token' from sanctum.middleware")
	}
}

func TestProjectConfigOverridesVendor(t *testing.T) {
	dir, idx := setupTestProject(t)

	// Create a vendor config AND a project config with the same name
	vendorConfigDir := filepath.Join(dir, "vendor", "test", "pkg", "config")
	os.MkdirAll(vendorConfigDir, 0755)
	os.WriteFile(filepath.Join(vendorConfigDir, "app.php"), []byte(`<?php
return ['vendor_key' => 'vendor_value'];
`), 0644)

	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// Project config/app.php should win — should NOT contain vendor_key
	fields := r.ParseConfigFile("app")
	for _, f := range fields {
		if f.Key == "vendor_key" {
			t.Error("project config should override vendor — vendor_key should not appear")
		}
	}
}

func TestVendorCacheInvalidation(t *testing.T) {
	dir, idx := setupTestProject(t)
	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	// First call discovers nothing from vendor
	fields1 := r.ListConfigFiles()

	// Add a vendor config
	vendorConfigDir := filepath.Join(dir, "vendor", "test", "new-pkg", "config")
	os.MkdirAll(vendorConfigDir, 0755)
	os.WriteFile(filepath.Join(vendorConfigDir, "newpkg.php"), []byte(`<?php
return ['key' => 'val'];
`), 0644)

	// Without invalidation, cache is stale
	fields2 := r.ListConfigFiles()
	if len(fields2) != len(fields1) {
		t.Error("expected cached result without invalidation")
	}

	// After invalidation, should discover new package
	r.InvalidateVendorCache()
	fields3 := r.ListConfigFiles()

	keys := make(map[string]bool)
	for _, f := range fields3 {
		keys[f.Key] = true
	}
	if !keys["newpkg"] {
		t.Error("expected 'newpkg' after cache invalidation")
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
