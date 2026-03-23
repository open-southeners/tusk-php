package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupMigrationTest(t *testing.T, migrations map[string]string) (*symbols.Index, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create database/migrations directory
	migrationsDir := filepath.Join(tmpDir, "database", "migrations")
	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write migration files
	for name, content := range migrations {
		if err := os.WriteFile(filepath.Join(migrationsDir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Setup index with Eloquent Model base
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`)

	return idx, tmpDir
}

func TestMigrationBasicColumns(t *testing.T) {
	idx, tmpDir := setupMigrationTest(t, map[string]string{
		"2024_01_01_000000_create_users_table.php": `<?php
use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

return new class extends Migration {
    public function up(): void
    {
        Schema::create('users', function (Blueprint $table) {
            $table->id();
            $table->string('name');
            $table->string('email');
            $table->integer('age');
            $table->boolean('active');
            $table->float('rating');
            $table->decimal('balance');
            $table->json('metadata');
            $table->text('bio')->nullable();
            $table->timestamps();
        });
    }
};
`,
	})

	// Index a User model
	modelPath := filepath.Join(tmpDir, "User.php")
	os.WriteFile(modelPath, []byte(`<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`), 0644)
	idx.IndexFile("file://"+modelPath, `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`)

	AnalyzeMigrations(idx, tmpDir)

	tests := map[string]string{
		"$id":       "int",
		"$name":     "string",
		"$email":    "string",
		"$age":      "int",
		"$active":   "bool",
		"$rating":   "float",
		"$balance":  "string",
		"$metadata": "array",
	}

	for propName, expectedType := range tests {
		t.Run(propName, func(t *testing.T) {
			sym := idx.Lookup("App\\Models\\User::" + propName)
			if sym == nil {
				t.Fatalf("expected virtual property %q", propName)
			}
			if sym.Type != expectedType {
				t.Errorf("expected type %q, got %q", expectedType, sym.Type)
			}
			if !sym.IsVirtual {
				t.Error("expected virtual")
			}
		})
	}

	t.Run("nullable column", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$bio")
		if sym == nil {
			t.Fatal("expected virtual property '$bio'")
		}
		if sym.Type != "?string" {
			t.Errorf("expected type '?string', got %q", sym.Type)
		}
	})

	t.Run("timestamps creates created_at", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$created_at")
		if sym == nil {
			t.Fatal("expected virtual property '$created_at'")
		}
		if sym.Type != "?\\DateTimeInterface" {
			t.Errorf("expected type '?\\DateTimeInterface', got %q", sym.Type)
		}
	})

	t.Run("timestamps creates updated_at", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$updated_at")
		if sym == nil {
			t.Fatal("expected virtual property '$updated_at'")
		}
	})
}

func TestMigrationSoftDeletes(t *testing.T) {
	idx, tmpDir := setupMigrationTest(t, map[string]string{
		"2024_01_01_000000_create_posts_table.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::create('posts', function (Blueprint $table) {
            $table->id();
            $table->string('title');
            $table->softDeletes();
        });
    }
};
`,
	})

	modelPath := filepath.Join(tmpDir, "Post.php")
	os.WriteFile(modelPath, []byte(`<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`), 0644)
	idx.IndexFile("file://"+modelPath, `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`)

	AnalyzeMigrations(idx, tmpDir)

	sym := idx.Lookup("App\\Models\\Post::$deleted_at")
	if sym == nil {
		t.Fatal("expected virtual property '$deleted_at' from softDeletes()")
	}
	if sym.Type != "?\\DateTimeInterface" {
		t.Errorf("expected type '?\\DateTimeInterface', got %q", sym.Type)
	}
}

func TestMigrationDropColumn(t *testing.T) {
	idx, tmpDir := setupMigrationTest(t, map[string]string{
		"2024_01_01_000000_create_users_table.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::create('users', function (Blueprint $table) {
            $table->id();
            $table->string('name');
            $table->string('legacy_field');
            $table->timestamps();
        });
    }
};
`,
		"2024_06_01_000000_drop_legacy_field.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::table('users', function (Blueprint $table) {
            $table->dropColumn('legacy_field');
        });
    }
};
`,
	})

	modelPath := filepath.Join(tmpDir, "User.php")
	os.WriteFile(modelPath, []byte(`<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`), 0644)
	idx.IndexFile("file://"+modelPath, `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`)

	AnalyzeMigrations(idx, tmpDir)

	t.Run("dropped column not present", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$legacy_field")
		if sym != nil {
			t.Error("expected '$legacy_field' to be dropped by later migration")
		}
	})

	t.Run("other columns still present", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$name")
		if sym == nil {
			t.Fatal("expected '$name' to still exist")
		}
	})
}

func TestMigrationOrdering(t *testing.T) {
	idx, tmpDir := setupMigrationTest(t, map[string]string{
		"2024_01_01_000000_create_users_table.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::create('users', function (Blueprint $table) {
            $table->id();
            $table->string('name');
        });
    }
};
`,
		"2024_03_01_000000_add_email_to_users.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::table('users', function (Blueprint $table) {
            $table->string('email');
        });
    }
};
`,
	})

	modelPath := filepath.Join(tmpDir, "User.php")
	os.WriteFile(modelPath, []byte(`<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`), 0644)
	idx.IndexFile("file://"+modelPath, `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {}
`)

	AnalyzeMigrations(idx, tmpDir)

	t.Run("original column exists", func(t *testing.T) {
		if idx.Lookup("App\\Models\\User::$name") == nil {
			t.Fatal("expected '$name'")
		}
	})

	t.Run("added column from later migration", func(t *testing.T) {
		if idx.Lookup("App\\Models\\User::$email") == nil {
			t.Fatal("expected '$email' from later migration")
		}
	})
}

func TestMigrationSkipsExistingProperties(t *testing.T) {
	idx, tmpDir := setupMigrationTest(t, map[string]string{
		"2024_01_01_000000_create_users_table.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::create('users', function (Blueprint $table) {
            $table->id();
            $table->string('email');
        });
    }
};
`,
	})

	// Index a User model with an existing $email property
	modelPath := filepath.Join(tmpDir, "User.php")
	modelSource := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class User extends Model {
    public string $email;
}
`
	os.WriteFile(modelPath, []byte(modelSource), 0644)
	idx.IndexFile("file://"+modelPath, modelSource)

	AnalyzeMigrations(idx, tmpDir)

	t.Run("existing property not overridden", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$email")
		if sym == nil {
			t.Fatal("expected '$email'")
		}
		if sym.IsVirtual {
			t.Error("real property should not be marked virtual")
		}
	})

	t.Run("new column added", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$id")
		if sym == nil {
			t.Fatal("expected '$id' from migration")
		}
		if !sym.IsVirtual {
			t.Error("expected virtual")
		}
	})
}

func TestMigrationForeignId(t *testing.T) {
	idx, tmpDir := setupMigrationTest(t, map[string]string{
		"2024_01_01_000000_create_posts_table.php": `<?php
return new class extends Migration {
    public function up(): void
    {
        Schema::create('posts', function (Blueprint $table) {
            $table->id();
            $table->foreignId('user_id')->constrained();
            $table->string('title');
        });
    }
};
`,
	})

	modelPath := filepath.Join(tmpDir, "Post.php")
	os.WriteFile(modelPath, []byte(`<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`), 0644)
	idx.IndexFile("file://"+modelPath, `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`)

	AnalyzeMigrations(idx, tmpDir)

	sym := idx.Lookup("App\\Models\\Post::$user_id")
	if sym == nil {
		t.Fatal("expected '$user_id' from foreignId()")
	}
	if sym.Type != "int" {
		t.Errorf("expected type 'int', got %q", sym.Type)
	}
}

func TestParseMigrationFile(t *testing.T) {
	source := `<?php
Schema::create('items', function (Blueprint $table) {
    $table->id();
    $table->string('name');
    $table->integer('quantity')->default(0);
    $table->enum('status', ['draft', 'published']);
    $table->date('published_at')->nullable();
});
`
	tableColumns := make(map[string]map[string]*MigrationColumn)
	parseMigrationFile(source, tableColumns)

	cols := tableColumns["items"]
	if cols == nil {
		t.Fatal("expected columns for 'items' table")
	}

	if cols["id"] == nil || cols["id"].Type != "int" {
		t.Error("expected id column with int type")
	}
	if cols["name"] == nil || cols["name"].Type != "string" {
		t.Error("expected name column with string type")
	}
	if cols["quantity"] == nil || cols["quantity"].Default != "0" {
		t.Errorf("expected quantity with default '0', got %+v", cols["quantity"])
	}
	if cols["status"] == nil || cols["status"].Type != "string" {
		t.Error("expected status as string (enum)")
	}
	if cols["published_at"] == nil || !cols["published_at"].Nullable {
		t.Error("expected published_at as nullable")
	}
}
