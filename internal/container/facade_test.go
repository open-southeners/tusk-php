package container

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestParseFacadeAccessorReturn(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			"simple string",
			`protected static function getFacadeAccessor() { return 'cache'; }`,
			"cache",
		},
		{
			"double quotes",
			`protected static function getFacadeAccessor() { return "auth"; }`,
			"auth",
		},
		{
			"multiline",
			`protected static function getFacadeAccessor()
    {
        return 'db';
    }`,
			"db",
		},
		{
			"with return type",
			`protected static function getFacadeAccessor(): string { return 'events'; }`,
			"events",
		},
		{
			"no match",
			`public function something() { return 'foo'; }`,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFacadeAccessorReturn(tt.source)
			if got != tt.want {
				t.Errorf("parseFacadeAccessorReturn() = %q, want %q", got, tt.want)
			}
		})
	}
}

// writeTempPHP creates a temp PHP file and returns its file:// URI.
func writeTempPHP(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return "file://" + path
}

func TestResolveFacade(t *testing.T) {
	dir := t.TempDir()
	idx := symbols.NewIndex()

	facadeURI := writeTempPHP(t, dir, "Facade.php", `<?php
namespace Illuminate\Support\Facades;

abstract class Facade {
	protected static function getFacadeAccessor() {}
}
`)
	idx.IndexFile(facadeURI, `<?php
namespace Illuminate\Support\Facades;

abstract class Facade {
	protected static function getFacadeAccessor() {}
}
`)

	cacheSrc := `<?php
namespace Illuminate\Support\Facades;

class Cache extends Facade {
	protected static function getFacadeAccessor()
	{
		return 'cache';
	}
}
`
	cacheURI := writeTempPHP(t, dir, "Cache.php", cacheSrc)
	idx.IndexFile(cacheURI, cacheSrc)

	idx.IndexFile("file:///CacheManager.php", `<?php
namespace Illuminate\Cache;

class CacheManager {
	public function get(string $key, mixed $default = null): mixed {}
	public function put(string $key, mixed $value, int $ttl = 0): bool {}
}
`)

	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()

	concrete := ca.ResolveFacade(`Illuminate\Support\Facades\Cache`)
	if concrete != `Illuminate\Cache\CacheManager` {
		t.Errorf("ResolveFacade(Cache) = %q, want Illuminate\\Cache\\CacheManager", concrete)
	}
}

func TestResolveFacadeNonFacade(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///service.php", `<?php
namespace App;

class UserService {
	public function find(int $id): ?User {}
}
`)

	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()

	if concrete := ca.ResolveFacade(`App\UserService`); concrete != "" {
		t.Errorf("expected empty for non-facade, got %q", concrete)
	}
}

func TestResolveFacadeWithCustomBinding(t *testing.T) {
	dir := t.TempDir()
	idx := symbols.NewIndex()

	facadeURI := writeTempPHP(t, dir, "Facade.php", `<?php
namespace Illuminate\Support\Facades;

abstract class Facade {
	protected static function getFacadeAccessor() {}
}
`)
	idx.IndexFile(facadeURI, `<?php
namespace Illuminate\Support\Facades;

abstract class Facade {
	protected static function getFacadeAccessor() {}
}
`)

	myFacadeSrc := `<?php
namespace App\Facades;

use Illuminate\Support\Facades\Facade;

class MyFacade extends Facade {
	protected static function getFacadeAccessor()
	{
		return 'my.service';
	}
}
`
	myFacadeURI := writeTempPHP(t, dir, "MyFacade.php", myFacadeSrc)
	idx.IndexFile(myFacadeURI, myFacadeSrc)

	idx.IndexFile("file:///MyService.php", `<?php
namespace App\Services;

class MyService {
	public function doWork(): void {}
}
`)

	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()
	// Register a custom binding
	ca.mu.Lock()
	ca.bindings["my.service"] = &ServiceBinding{Abstract: "my.service", Concrete: `App\Services\MyService`}
	ca.mu.Unlock()

	concrete := ca.ResolveFacade(`App\Facades\MyFacade`)
	if concrete != `App\Services\MyService` {
		t.Errorf("ResolveFacade(MyFacade) = %q, want App\\Services\\MyService", concrete)
	}
}
