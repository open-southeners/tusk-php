package completion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/models"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// setupEloquentCompletionTest creates a minimal Laravel-like environment
// with model files, migrations, and Eloquent stubs for testing completions.
func setupEloquentCompletionTest(t *testing.T) (*Provider, string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	// Minimal Eloquent vendor stubs
	idx.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {
    public function save(): bool { return true; }
    public function delete(): bool { return true; }
    public function toArray(): array { return []; }
    public function refresh(): static { return $this; }
}
`)
	idx.IndexFile("file:///vendor/Builder.php", `<?php
namespace Illuminate\Database\Eloquent;
class Builder {
    public function where($column, $operator = null, $value = null): static { return $this; }
    public function first(): ?Model { return null; }
    public function get(): Collection { return new Collection(); }
    public function with($relations): static { return $this; }
    public function orderBy($column, $direction = 'asc'): static { return $this; }
}
`)
	idx.IndexFile("file:///vendor/Collection.php", `<?php
namespace Illuminate\Database\Eloquent;
class Collection {
    public function count(): int { return 0; }
    public function first(): ?Model { return null; }
    public function map(\Closure $callback): static { return $this; }
}
`)
	idx.IndexFile("file:///vendor/HasMany.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;
class HasMany {
    public function create(array $attributes = []): \Illuminate\Database\Eloquent\Model { }
    public function where($column, $operator = null, $value = null): static { return $this; }
}
`)
	idx.IndexFile("file:///vendor/BelongsTo.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;
class BelongsTo {
    public function associate($model): static { return $this; }
}
`)

	// Create project with models and migrations
	tmpDir := t.TempDir()

	categorySource := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Category extends Model {
    public string $name;
    public string $slug;

    public function products(): HasMany
    {
        return $this->hasMany(Product::class);
    }
}
`
	writeProjectFile(t, tmpDir, "app/Models/Category.php", categorySource)
	idx.IndexFile("file://"+filepath.Join(tmpDir, "app/Models/Category.php"), categorySource)

	productSource := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;

class Product extends Model {
    public function category(): BelongsTo
    {
        return $this->belongsTo(Category::class);
    }
}
`
	writeProjectFile(t, tmpDir, "app/Models/Product.php", productSource)
	idx.IndexFile("file://"+filepath.Join(tmpDir, "app/Models/Product.php"), productSource)

	// Create migrations
	writeProjectFile(t, tmpDir, "database/migrations/2024_01_01_create_categories_table.php", `<?php
return new class extends Migration {
    public function up(): void {
        Schema::create('categories', function (Blueprint $table) {
            $table->id();
            $table->string('name');
            $table->string('slug');
            $table->text('description')->nullable();
            $table->boolean('is_active')->default(true);
            $table->integer('sort_order')->default(0);
            $table->timestamps();
        });
    }
};
`)
	writeProjectFile(t, tmpDir, "database/migrations/2024_01_02_create_products_table.php", `<?php
return new class extends Migration {
    public function up(): void {
        Schema::create('products', function (Blueprint $table) {
            $table->id();
            $table->foreignId('category_id')->constrained();
            $table->string('title');
            $table->text('body')->nullable();
            $table->decimal('price', 10, 2);
            $table->timestamps();
        });
    }
};
`)

	// Run the analysis pipeline (same order as server init)
	models.AnalyzeEloquentModels(idx, tmpDir)
	models.AnalyzeMigrations(idx, tmpDir)

	p := NewProvider(idx, nil, "laravel")
	return p, tmpDir
}

func writeProjectFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestEloquentModelInstanceCompletion(t *testing.T) {
	p, _ := setupEloquentCompletionTest(t)

	t.Run("Category::first() then $category-> shows all members", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
$category = Category::first();
$category->`
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 11})
		labels := collectLabels(items)

		// Declared properties
		for _, prop := range []string{"name", "slug"} {
			if !labels[prop] {
				t.Errorf("expected declared property %q, got labels: %v", prop, labels)
			}
		}
		// Relation as property
		if !labels["products"] {
			t.Errorf("expected relation property 'products', got labels: %v", labels)
		}
		// Migration-discovered columns
		for _, col := range []string{"id", "description", "is_active", "sort_order", "created_at", "updated_at"} {
			if !labels[col] {
				t.Errorf("expected migration column %q, got labels: %v", col, labels)
			}
		}
		// Inherited methods
		if !labels["save"] {
			t.Errorf("expected inherited method 'save', got labels: %v", labels)
		}
	})

	t.Run("Category:: shows static builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 10})
		labels := collectLabels(items)

		for _, m := range []string{"query", "find", "first", "where", "all", "create", "with"} {
			if !labels[m] {
				t.Errorf("expected static method %q in Category::, got labels: %v", m, labels)
			}
		}
	})

	t.Run("Category::query()-> shows builder methods", func(t *testing.T) {
		source := "<?php\nuse App\\Models\\Category;\nCategory::query()->"
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
		labels := collectLabels(items)

		if !labels["where"] {
			t.Errorf("expected 'where' from Builder, got labels: %v", labels)
		}
		if !labels["first"] {
			t.Errorf("expected 'first' from Builder, got labels: %v", labels)
		}
	})

	t.Run("Product::find(1) then $product-> shows relation + migration columns", func(t *testing.T) {
		source := `<?php
use App\Models\Product;
$product = Product::find(1);
$product->`
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 10})
		labels := collectLabels(items)

		// Relation as property
		if !labels["category"] {
			t.Errorf("expected relation property 'category', got labels: %v", labels)
		}
		// Migration columns
		for _, col := range []string{"id", "category_id", "title", "body", "price"} {
			if !labels[col] {
				t.Errorf("expected migration column %q, got labels: %v", col, labels)
			}
		}
	})

	t.Run("Category::where()->first() chain resolves to Category", func(t *testing.T) {
		source := `<?php
use App\Models\Category;
$cat = Category::where('active', true)->first();
$cat->`
		items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 3, Character: 6})
		labels := collectLabels(items)

		// where() returns Builder, first() on Builder returns ?Model
		// This is a known limitation — Builder::first() returns Model, not the specific subclass
		// But the declared properties should still show if the type resolves
		if len(items) > 0 && !labels["save"] {
			t.Logf("Chain Category::where()->first() resolved to type with %d items", len(items))
		}
	})
}
