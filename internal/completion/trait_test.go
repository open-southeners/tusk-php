package completion

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupTraitCompletion() *Provider {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	// Level 0: base trait
	idx.IndexFile("file:///traits/HasTimestamps.php", `<?php
namespace App\Concerns;

trait HasTimestamps {
	public function touchTimestamp(): void {}
	public string $updatedAt = '';
}
`)
	// Level 1: trait using another trait
	idx.IndexFile("file:///traits/Auditable.php", `<?php
namespace App\Concerns;

trait Auditable {
	use HasTimestamps;

	public function getAuditLog(): array {}
}
`)
	// Level 2: trait using the level-1 trait
	idx.IndexFile("file:///traits/FullAudit.php", `<?php
namespace App\Concerns;

trait FullAudit {
	use Auditable;

	public function auditSummary(): string {}
}
`)
	// Class using the deeply nested trait
	idx.IndexFile("file:///models/Order.php", `<?php
namespace App\Models;

use App\Concerns\FullAudit;

class Order {
	use FullAudit;

	public function total(): float {}
}
`)
	// Class using multiple traits
	idx.IndexFile("file:///traits/Searchable.php", `<?php
namespace App\Concerns;

trait Searchable {
	public static function search(string $query): array {}
}
`)
	idx.IndexFile("file:///models/Product.php", `<?php
namespace App\Models;

use App\Concerns\Searchable;
use App\Concerns\HasTimestamps;

class Product {
	use Searchable, HasTimestamps;

	public function price(): float {}
}
`)
	// Class with trait + inheritance
	idx.IndexFile("file:///models/Model.php", `<?php
namespace App\Models;

class Model {
	public function save(): bool {}
}
`)
	idx.IndexFile("file:///models/User.php", `<?php
namespace App\Models;

use App\Concerns\Auditable;

class User extends Model {
	use Auditable;

	public function fullName(): string {}
}
`)

	return NewProvider(idx, nil, "none")
}

func TestCompleteTraitMethodsOnInstance(t *testing.T) {
	p := setupTraitCompletion()
	source := `<?php
use App\Models\Order;
$order = new Order();
$order->`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 8})
	labels := collectLabels(items)

	expected := []string{"total", "auditSummary", "getAuditLog", "touchTimestamp"}
	for _, name := range expected {
		if !labels[name] {
			t.Errorf("expected method %s in completions, got labels: %v", name, labels)
		}
	}
}

func TestCompleteNestedTraitPropertiesOnInstance(t *testing.T) {
	p := setupTraitCompletion()
	source := `<?php
use App\Models\Order;
$order = new Order();
$order->u`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 9})
	labels := collectLabels(items)

	if !labels["updatedAt"] {
		t.Errorf("expected property updatedAt from nested trait, got: %v", labels)
	}
}

func TestCompleteMultipleTraitMembers(t *testing.T) {
	p := setupTraitCompletion()
	source := `<?php
use App\Models\Product;
$p = new Product();
$p->`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 4})
	labels := collectLabels(items)

	expected := []string{"price", "touchTimestamp"}
	for _, name := range expected {
		if !labels[name] {
			t.Errorf("expected method %s from trait, got labels: %v", name, labels)
		}
	}
}

func TestCompleteStaticTraitMethods(t *testing.T) {
	p := setupTraitCompletion()
	source := `<?php
use App\Models\Product;
Product::`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 9})
	labels := collectLabels(items)

	if !labels["search"] {
		t.Errorf("expected static method search from Searchable trait, got: %v", labels)
	}
}

func TestCompleteTraitPlusInheritance(t *testing.T) {
	p := setupTraitCompletion()
	source := `<?php
use App\Models\User;
$u = new User();
$u->`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 4})
	labels := collectLabels(items)

	expected := []string{"fullName", "getAuditLog", "touchTimestamp", "save"}
	for _, name := range expected {
		if !labels[name] {
			t.Errorf("expected method %s from trait+inheritance, got labels: %v", name, labels)
		}
	}
}
