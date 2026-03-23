package models

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupEloquentIndex(t *testing.T, modelSource string) *symbols.Index {
	t.Helper()
	idx := symbols.NewIndex()

	// Index a minimal Eloquent Model base class
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Model.php", `<?php
namespace Illuminate\Database\Eloquent;

abstract class Model {
    public function save(): bool { return true; }
    public function delete(): bool { return true; }
}
`)

	// Index the Attribute cast class
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Casts/Attribute.php", `<?php
namespace Illuminate\Database\Eloquent\Casts;

class Attribute {
    public static function make(): static { return new static(); }
}
`)

	// Index relation stubs
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations/HasMany.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;

class HasMany {}
`)
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations/BelongsTo.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;

class BelongsTo {}
`)
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Relations/HasOne.php", `<?php
namespace Illuminate\Database\Eloquent\Relations;

class HasOne {}
`)

	// Index the Collection class
	idx.IndexFile("file:///vendor/laravel/framework/src/Illuminate/Database/Eloquent/Collection.php", `<?php
namespace Illuminate\Database\Eloquent;

class Collection {}
`)

	// Write the model source to a temp file so the analyzer can read it
	tmpDir := t.TempDir()
	modelPath := filepath.Join(tmpDir, "User.php")
	if err := os.WriteFile(modelPath, []byte(modelSource), 0644); err != nil {
		t.Fatal(err)
	}

	idx.IndexFile("file://"+modelPath, modelSource)

	return idx
}

func TestEloquentRelationDiscovery(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\HasMany;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\HasOne;

class User extends Model {
    public string $email;

    public function posts(): HasMany
    {
        return $this->hasMany(Post::class);
    }

    public function team(): BelongsTo
    {
        return $this->belongsTo(Team::class);
    }

    public function profile(): HasOne
    {
        return $this->hasOne(Profile::class);
    }
}
`
	idx := setupEloquentIndex(t, source)

	// Index related models so resolve works
	idx.IndexFile("file:///Post.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`)
	idx.IndexFile("file:///Team.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Team extends Model {}
`)
	idx.IndexFile("file:///Profile.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Profile extends Model {}
`)

	AnalyzeEloquentModels(idx, "")

	t.Run("HasMany creates Collection virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$posts")
		if sym == nil {
			t.Fatal("expected virtual property '$posts'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
		if sym.Type != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection type, got %q", sym.Type)
		}
	})

	t.Run("BelongsTo creates nullable virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$team")
		if sym == nil {
			t.Fatal("expected virtual property '$team'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
		if sym.Type != "?App\\Models\\Team" {
			t.Errorf("expected nullable Team type, got %q", sym.Type)
		}
	})

	t.Run("HasOne creates nullable virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$profile")
		if sym == nil {
			t.Fatal("expected virtual property '$profile'")
		}
		if sym.Type != "?App\\Models\\Profile" {
			t.Errorf("expected nullable Profile type, got %q", sym.Type)
		}
	})

	t.Run("real property not affected", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$email")
		if sym == nil {
			t.Fatal("expected real property '$email'")
		}
		if sym.IsVirtual {
			t.Error("real property should not be virtual")
		}
	})
}

func TestEloquentLegacyAccessor(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class User extends Model {
    public function getFullNameAttribute(): string
    {
        return $this->first_name . ' ' . $this->last_name;
    }

    public function getAgeAttribute(): int
    {
        return 25;
    }
}
`
	idx := setupEloquentIndex(t, source)
	AnalyzeEloquentModels(idx, "")

	t.Run("legacy accessor creates virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$full_name")
		if sym == nil {
			t.Fatal("expected virtual property '$full_name'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
		if sym.Type != "string" {
			t.Errorf("expected type 'string', got %q", sym.Type)
		}
	})

	t.Run("another legacy accessor", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$age")
		if sym == nil {
			t.Fatal("expected virtual property '$age'")
		}
		if sym.Type != "int" {
			t.Errorf("expected type 'int', got %q", sym.Type)
		}
	})
}

func TestEloquentModernAccessor(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Casts\Attribute;

class User extends Model {
    /**
     * @return Attribute
     */
    protected function fullName(): Attribute
    {
        return Attribute::make(
            get: fn () => $this->first_name . ' ' . $this->last_name,
        );
    }
}
`
	idx := setupEloquentIndex(t, source)
	AnalyzeEloquentModels(idx, "")

	t.Run("modern accessor creates virtual property", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$fullName")
		if sym == nil {
			t.Fatal("expected virtual property '$fullName'")
		}
		if !sym.IsVirtual {
			t.Error("expected IsVirtual")
		}
	})
}

func TestEloquentRelationWithoutReturnType(t *testing.T) {
	source := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;

class User extends Model {
    public function comments()
    {
        return $this->hasMany(Comment::class);
    }
}
`
	idx := setupEloquentIndex(t, source)
	idx.IndexFile("file:///Comment.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Comment extends Model {}
`)
	AnalyzeEloquentModels(idx, "")

	t.Run("relation detected from body without return type", func(t *testing.T) {
		sym := idx.Lookup("App\\Models\\User::$comments")
		if sym == nil {
			t.Fatal("expected virtual property '$comments'")
		}
		if sym.Type != "Illuminate\\Database\\Eloquent\\Collection" {
			t.Errorf("expected Collection type, got %q", sym.Type)
		}
	})
}

func TestSnakeCase(t *testing.T) {
	tests := []struct {
		input, expected string
	}{
		{"FullName", "full_name"},
		{"Name", "name"},
		{"ID", "i_d"},
		{"CreatedAt", "created_at"},
	}
	for _, tt := range tests {
		if got := snakeCase(tt.input); got != tt.expected {
			t.Errorf("snakeCase(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
