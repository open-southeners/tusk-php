package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupTraitProvider() *Provider {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///traits/HasTimestamps.php", `<?php
namespace App\Concerns;

trait HasTimestamps {
	public function touchTimestamp(): void {}
	public string $updatedAt = '';
}
`)
	idx.IndexFile("file:///traits/Auditable.php", `<?php
namespace App\Concerns;

trait Auditable {
	use HasTimestamps;

	public function getAuditLog(): array {}
}
`)
	idx.IndexFile("file:///traits/FullAudit.php", `<?php
namespace App\Concerns;

trait FullAudit {
	use Auditable;

	public function auditSummary(): string {}
}
`)
	idx.IndexFile("file:///models/Order.php", `<?php
namespace App\Models;

use App\Concerns\FullAudit;

class Order {
	use FullAudit;

	public function total(): float {}
}
`)
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

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	return NewProvider(idx, ca, "none")
}

func TestHoverTraitMethodOnInstance(t *testing.T) {
	p := setupTraitProvider()
	source := `<?php
use App\Models\Order;
$order = new Order();
$order->getAuditLog();
`
	pos := charPosOf(t, source, "getAuditLog", "$order->getAuditLog")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on trait method getAuditLog")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function getAuditLog") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
}

func TestHoverDeeplyNestedTraitMethod(t *testing.T) {
	p := setupTraitProvider()
	source := `<?php
use App\Models\Order;
$order = new Order();
$order->touchTimestamp();
`
	pos := charPosOf(t, source, "touchTimestamp", "$order->touchTimestamp")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on deeply nested trait method touchTimestamp")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function touchTimestamp") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
}

func TestHoverNestedTraitProperty(t *testing.T) {
	p := setupTraitProvider()
	source := `<?php
use App\Models\Order;
$order = new Order();
$order->updatedAt;
`
	pos := charPosOf(t, source, "updatedAt", "$order->updatedAt")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on nested trait property updatedAt")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "updatedAt") {
		t.Errorf("expected property name in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "string") {
		t.Errorf("expected property type string, got:\n%s", val)
	}
}

func TestHoverTraitMethodWithInheritance(t *testing.T) {
	p := setupTraitProvider()
	source := `<?php
use App\Models\User;
$u = new User();
$u->touchTimestamp();
`
	pos := charPosOf(t, source, "touchTimestamp", "$u->touchTimestamp")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on trait method through inheritance+trait chain")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function touchTimestamp") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
}

func TestHoverInheritedMethodStillWorksWithTrait(t *testing.T) {
	p := setupTraitProvider()
	source := `<?php
use App\Models\User;
$u = new User();
$u->save();
`
	pos := charPosOf(t, source, "save", "$u->save")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on inherited method save")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "function save") {
		t.Errorf("expected method signature, got:\n%s", val)
	}
}

func TestHoverTraitNameItself(t *testing.T) {
	p := setupTraitProvider()
	source := `<?php
namespace App\Concerns;

trait HasTimestamps {
	public function touchTimestamp(): void {}
}
`
	pos := protocol.Position{Line: 3, Character: 6} // "HasTimestamps"
	hover := p.GetHover("file:///traits/HasTimestamps.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on trait name")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "trait HasTimestamps") {
		t.Errorf("expected 'trait HasTimestamps' in hover, got:\n%s", val)
	}
}
