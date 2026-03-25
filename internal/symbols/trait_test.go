package symbols

import (
	"testing"
)

func TestTraitMembersResolvedOnClass(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///trait.php", `<?php
namespace App;

trait Loggable {
	public string $logPrefix = '';

	public function log(string $message): void {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class UserService {
	use Loggable;

	public function doWork(): void {}
}
`)

	members := idx.GetClassMembers(`App\UserService`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	if !found["doWork"] {
		t.Error("expected own method doWork")
	}
	if !found["log"] {
		t.Error("expected trait method log")
	}
	if !found["$logPrefix"] {
		t.Error("expected trait property $logPrefix")
	}
}

func TestNestedTraitMembers(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///base_trait.php", `<?php
namespace App;

trait HasTimestamps {
	public function touchTimestamp(): void {}
	public string $updatedAt = '';
}
`)
	idx.IndexFile("file:///middle_trait.php", `<?php
namespace App;

trait Auditable {
	use HasTimestamps;

	public function getAuditLog(): array {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class Order {
	use Auditable;

	public function total(): float {}
}
`)

	members := idx.GetClassMembers(`App\Order`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	if !found["total"] {
		t.Error("expected own method total")
	}
	if !found["getAuditLog"] {
		t.Error("expected method from Auditable trait")
	}
	if !found["touchTimestamp"] {
		t.Error("expected method from nested HasTimestamps trait")
	}
	if !found["$updatedAt"] {
		t.Error("expected property from nested HasTimestamps trait")
	}
}

func TestDeeplyNestedTraitMembers(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", `<?php
namespace App;

trait TraitA {
	public function fromA(): void {}
}
`)
	idx.IndexFile("file:///b.php", `<?php
namespace App;

trait TraitB {
	use TraitA;
	public function fromB(): void {}
}
`)
	idx.IndexFile("file:///c.php", `<?php
namespace App;

trait TraitC {
	use TraitB;
	public function fromC(): void {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class MyClass {
	use TraitC;
	public function own(): void {}
}
`)

	members := idx.GetClassMembers(`App\MyClass`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	for _, name := range []string{"own", "fromC", "fromB", "fromA"} {
		if !found[name] {
			t.Errorf("expected method %s from nested trait chain", name)
		}
	}
}

func TestMultipleTraitsOnClass(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///t1.php", `<?php
namespace App;

trait Searchable {
	public function search(string $query): array {}
}
`)
	idx.IndexFile("file:///t2.php", `<?php
namespace App;

trait Cacheable {
	public function cache(int $ttl): void {}
}
`)
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class ProductRepository {
	use Searchable, Cacheable;

	public function findById(int $id): ?Product {}
}
`)

	members := idx.GetClassMembers(`App\ProductRepository`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	for _, name := range []string{"findById", "search", "cache"} {
		if !found[name] {
			t.Errorf("expected method %s", name)
		}
	}
}

func TestTraitWithInheritance(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///trait.php", `<?php
namespace App;

trait SoftDeletes {
	public function restore(): void {}
}
`)
	idx.IndexFile("file:///base.php", `<?php
namespace App;

class Model {
	public function save(): bool {}
}
`)
	idx.IndexFile("file:///child.php", `<?php
namespace App;

class User extends Model {
	use SoftDeletes;

	public function fullName(): string {}
}
`)

	members := idx.GetClassMembers(`App\User`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}

	for _, name := range []string{"fullName", "restore", "save"} {
		if !found[name] {
			t.Errorf("expected method %s from trait+inheritance chain", name)
		}
	}
}

func TestTraitMapCleanupOnReindex(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///trait.php", `<?php
namespace App;

trait OldTrait {
	public function oldMethod(): void {}
}

trait NewTrait {
	public function newMethod(): void {}
}
`)

	// First index: class uses OldTrait
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class Foo {
	use OldTrait;
}
`)

	members := idx.GetClassMembers(`App\Foo`)
	found := map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}
	if !found["oldMethod"] {
		t.Fatal("expected oldMethod before re-index")
	}

	// Re-index: class now uses NewTrait instead
	idx.IndexFile("file:///class.php", `<?php
namespace App;

class Foo {
	use NewTrait;
}
`)

	members = idx.GetClassMembers(`App\Foo`)
	found = map[string]bool{}
	for _, m := range members {
		found[m.Name] = true
	}
	if found["oldMethod"] {
		t.Error("oldMethod should not appear after re-index removed OldTrait")
	}
	if !found["newMethod"] {
		t.Error("expected newMethod after re-index added NewTrait")
	}
}
