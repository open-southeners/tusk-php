package composer

import (
	"path/filepath"
	"testing"
)

func TestFQNToPath(t *testing.T) {
	entries := []AutoloadEntry{
		{Namespace: "App", Path: "/project/app", IsVendor: false},
		{Namespace: "App\\Models", Path: "/project/app/Models", IsVendor: false},
		{Namespace: "Tests", Path: "/project/tests", IsVendor: false},
		{Namespace: "Illuminate\\Support", Path: "/project/vendor/laravel/framework/src/Illuminate/Support", IsVendor: true},
	}

	tests := []struct {
		name string
		fqn  string
		want string
	}{
		{
			"simple class",
			"App\\Models\\User",
			filepath.Join("/project/app/Models", "User.php"),
		},
		{
			"nested namespace matched by longest prefix",
			"App\\Models\\Category",
			filepath.Join("/project/app/Models", "Category.php"),
		},
		{
			"falls back to shorter prefix",
			"App\\Services\\PaymentService",
			filepath.Join("/project/app", "Services", "PaymentService.php"),
		},
		{
			"test namespace",
			"Tests\\Unit\\UserTest",
			filepath.Join("/project/tests", "Unit", "UserTest.php"),
		},
		{
			"no match",
			"Unknown\\Class",
			"",
		},
		{
			"vendor entries ignored",
			"Illuminate\\Support\\Collection",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FQNToPath(tt.fqn, entries)
			if got != tt.want {
				t.Errorf("FQNToPath(%q) = %q, want %q", tt.fqn, got, tt.want)
			}
		})
	}
}

func TestPathToNamespace(t *testing.T) {
	entries := []AutoloadEntry{
		{Namespace: "App", Path: "/project/app", IsVendor: false},
		{Namespace: "Tests", Path: "/project/tests", IsVendor: false},
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			"direct class",
			"/project/app/User.php",
			"App",
		},
		{
			"nested class",
			"/project/app/Models/User.php",
			"App\\Models",
		},
		{
			"deeply nested",
			"/project/app/Http/Controllers/Api/UserController.php",
			"App\\Http\\Controllers\\Api",
		},
		{
			"test namespace",
			"/project/tests/Unit/UserTest.php",
			"Tests\\Unit",
		},
		{
			"no match",
			"/other/path/Foo.php",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PathToNamespace(tt.path, entries)
			if got != tt.want {
				t.Errorf("PathToNamespace(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
