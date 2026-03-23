package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestParseRealWorldLaravelConfig(t *testing.T) {
	// Simulate a real Laravel config/database.php with block comments and env() calls
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "config", "database.php"), []byte(`<?php

use Illuminate\Support\Str;

return [

    /*
    |--------------------------------------------------------------------------
    | Default Database Connection Name
    |--------------------------------------------------------------------------
    |
    | Here you may specify which of the database connections below you wish
    | to use as your default connection for database operations.
    |
    */

    'default' => env('DB_CONNECTION', 'sqlite'),

    /*
    |--------------------------------------------------------------------------
    | Database Connections
    |--------------------------------------------------------------------------
    */

    'connections' => [

        'sqlite' => [
            'driver' => 'sqlite',
            'url' => env('DB_URL'),
            'database' => env('DB_DATABASE', database_path('database.sqlite')),
            'prefix' => '',
            'foreign_key_constraints' => env('DB_FOREIGN_KEYS', true),
        ],

        'mysql' => [
            'driver' => 'mysql',
            'url' => env('DB_URL'),
            'host' => env('DB_HOST', '127.0.0.1'),
            'port' => env('DB_PORT', '3306'),
            'database' => env('DB_DATABASE', 'laravel'),
            'username' => env('DB_USERNAME', 'root'),
            'password' => env('DB_PASSWORD', ''),
        ],

    ],

    // Redis configuration
    'redis' => [
        'client' => env('REDIS_CLIENT', 'phpredis'),
        'default' => [
            'url' => env('REDIS_URL'),
            'host' => env('REDIS_HOST', '127.0.0.1'),
            'port' => env('REDIS_PORT', '6379'),
        ],
    ],

    'migrations' => [
        'table' => 'migrations',
        'update_date_on_publish' => true,
    ],

];
`), 0644)

	idx := symbols.NewIndex()
	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	t.Run("top level keys", func(t *testing.T) {
		fields := r.ParseConfigFile("database")
		if len(fields) == 0 {
			t.Fatal("expected keys from real-world config file")
		}

		keys := make(map[string]bool)
		for _, f := range fields {
			keys[f.Key] = true
		}

		if !keys["default"] {
			t.Error("expected 'default' key")
		}
		if !keys["connections"] {
			t.Error("expected 'connections' key")
		}
		if !keys["redis"] {
			t.Error("expected 'redis' key")
		}
		if !keys["migrations"] {
			t.Error("expected 'migrations' key")
		}
	})

	t.Run("nested connection keys", func(t *testing.T) {
		fields := r.ResolveCallReturnKeys("config('database.connections')", "")
		if len(fields) == 0 {
			t.Fatal("expected connection keys")
		}

		keys := make(map[string]bool)
		for _, f := range fields {
			keys[f.Key] = true
		}

		if !keys["sqlite"] {
			t.Error("expected 'sqlite' connection")
		}
		if !keys["mysql"] {
			t.Error("expected 'mysql' connection")
		}
	})

	t.Run("deep nested mysql keys", func(t *testing.T) {
		fields := r.ResolveCallReturnKeys("config('database.connections.mysql')", "")
		if len(fields) == 0 {
			t.Fatal("expected mysql connection keys")
		}

		keys := make(map[string]bool)
		for _, f := range fields {
			keys[f.Key] = true
		}

		if !keys["driver"] {
			t.Error("expected 'driver'")
		}
		if !keys["host"] {
			t.Error("expected 'host'")
		}
		if !keys["port"] {
			t.Error("expected 'port'")
		}
		if !keys["database"] {
			t.Error("expected 'database'")
		}
		if !keys["username"] {
			t.Error("expected 'username'")
		}
	})

	t.Run("line comment section parsed", func(t *testing.T) {
		// 'redis' section is preceded by a // comment, should still be parsed
		fields := r.ResolveCallReturnKeys("config('database.redis')", "")
		if len(fields) == 0 {
			t.Fatal("expected redis keys")
		}

		keys := make(map[string]bool)
		for _, f := range fields {
			keys[f.Key] = true
		}

		if !keys["client"] {
			t.Error("expected 'client' in redis config")
		}
		if !keys["default"] {
			t.Error("expected 'default' in redis config")
		}
	})
}

func TestParseConfigWithEnvCalls(t *testing.T) {
	// env() calls should be treated as mixed type, not crash the parser
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "config"), 0755)
	os.WriteFile(filepath.Join(dir, "config", "app.php"), []byte(`<?php
return [
    'name' => env('APP_NAME', 'Laravel'),
    'env' => env('APP_ENV', 'production'),
    'debug' => (bool) env('APP_DEBUG', false),
    'url' => env('APP_URL', 'http://localhost'),
    'timezone' => 'UTC',
    'locale' => 'en',
    'key' => env('APP_KEY'),
    'providers' => [
        // some providers
    ],
];
`), 0644)

	idx := symbols.NewIndex()
	r := NewFrameworkArrayResolver(idx, dir, "laravel")

	fields := r.ParseConfigFile("app")
	if len(fields) == 0 {
		t.Fatal("expected keys from config with env() calls")
	}

	keys := make(map[string]bool)
	for _, f := range fields {
		keys[f.Key] = true
	}

	for _, expected := range []string{"name", "env", "debug", "url", "timezone", "locale", "key", "providers"} {
		if !keys[expected] {
			t.Errorf("expected '%s' key", expected)
		}
	}
}
