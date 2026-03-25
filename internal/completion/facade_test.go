package completion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

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

func setupFacadeCompletion(t *testing.T) *Provider {
	t.Helper()
	dir := t.TempDir()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	// Base Facade class
	facadeSrc := `<?php
namespace Illuminate\Support\Facades;

abstract class Facade {
	protected static function getFacadeAccessor() {}
	public static function shouldReceive(): \Mockery\Expectation {}
}
`
	facadeURI := writeTempPHP(t, dir, "Facade.php", facadeSrc)
	idx.IndexFile(facadeURI, facadeSrc)

	// Cache facade with @method static annotations
	cacheSrc := `<?php
namespace Illuminate\Support\Facades;

/**
 * @method static mixed get(string $key, mixed $default = null)
 * @method static bool put(string $key, mixed $value, int $ttl = 0)
 * @method static bool forget(string $key)
 * @method static bool has(string $key)
 */
class Cache extends Facade {
	protected static function getFacadeAccessor()
	{
		return 'cache';
	}
}
`
	cacheURI := writeTempPHP(t, dir, "Cache.php", cacheSrc)
	idx.IndexFile(cacheURI, cacheSrc)

	// Concrete CacheManager
	idx.IndexFile("file:///CacheManager.php", `<?php
namespace Illuminate\Cache;

class CacheManager {
	public function get(string $key, mixed $default = null): mixed {}
	public function put(string $key, mixed $value, int $ttl = 0): bool {}
	public function forget(string $key): bool {}
	public function has(string $key): bool {}
	public function store(string $name = null): \Illuminate\Cache\Repository {}
}
`)

	// A facade WITHOUT @method annotations — relies on concrete resolution
	bareSrc := `<?php
namespace App\Facades;

use Illuminate\Support\Facades\Facade;

class Payment extends Facade {
	protected static function getFacadeAccessor()
	{
		return 'payment';
	}
}
`
	bareURI := writeTempPHP(t, dir, "Payment.php", bareSrc)
	idx.IndexFile(bareURI, bareSrc)

	idx.IndexFile("file:///PaymentService.php", `<?php
namespace App\Services;

class PaymentService {
	public function charge(float $amount): bool {}
	public function refund(string $transactionId): bool {}
}
`)

	ca := container.NewContainerAnalyzer(idx, dir, "laravel")
	ca.Analyze()
	// Add custom binding for Payment facade
	bindings := ca.GetBindings()
	_ = bindings
	// Access internal state to add binding (via Analyze's defaults + manual)
	// We need to add this after Analyze
	ca.ResolveDependency("_force_init_") // no-op, just ensures defaults are loaded

	return NewProvider(idx, ca, "laravel")
}

func TestCompleteFacadeWithDocblockAnnotations(t *testing.T) {
	p := setupFacadeCompletion(t)
	source := `<?php
use Illuminate\Support\Facades\Cache;
Cache::`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 7})
	labels := collectLabels(items)

	// @method static methods should appear
	for _, name := range []string{"get", "put", "forget", "has"} {
		if !labels[name] {
			t.Errorf("expected @method static %s in Cache:: completions, got: %v", name, labels)
		}
	}
	// Real static methods from Facade base should also appear
	if !labels["shouldReceive"] {
		t.Errorf("expected inherited static method shouldReceive, got: %v", labels)
	}
}

func TestCompleteFacadeConcreteResolution(t *testing.T) {
	p := setupFacadeCompletion(t)

	// The Cache facade has @method static annotations, but the concrete class
	// has a store() method not in the annotations. With facade resolution,
	// concrete methods should also appear.
	source := `<?php
use Illuminate\Support\Facades\Cache;
Cache::`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 7})
	labels := collectLabels(items)

	if !labels["store"] {
		t.Errorf("expected concrete CacheManager method store via facade resolution, got: %v", labels)
	}
}

func TestCompleteDocMethodStaticFlag(t *testing.T) {
	// Verify @method static properly sets IsStatic on virtual members
	idx := symbols.NewIndex()
	idx.IndexFile("file:///test_class.php", `<?php
namespace App;

/**
 * @method static string staticMethod()
 * @method string instanceMethod()
 */
class Foo {}
`)

	members := idx.GetClassMembers(`App\Foo`)
	for _, m := range members {
		if m.Name == "staticMethod" && !m.IsStatic {
			t.Error("expected @method static to set IsStatic=true")
		}
		if m.Name == "instanceMethod" && m.IsStatic {
			t.Error("expected @method without static to have IsStatic=false")
		}
	}
}
