package container

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/symbols"
)

func TestNewContainerAnalyzer(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	if ca == nil {
		t.Fatal("expected non-nil")
	}
	if ca.framework != "laravel" {
		t.Errorf("framework = %q", ca.framework)
	}
	if ca.bindings == nil {
		t.Error("bindings should be initialized")
	}
}

func TestResolveDependency(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "none")
	ca.bindings["cache"] = &ServiceBinding{Abstract: "cache", Concrete: "Illuminate\\Cache\\CacheManager", Singleton: true}
	ca.aliases["my-cache"] = "cache"

	t.Run("direct binding", func(t *testing.T) {
		b := ca.ResolveDependency("cache")
		if b == nil || b.Concrete != "Illuminate\\Cache\\CacheManager" {
			t.Errorf("got %v", b)
		}
	})

	t.Run("via alias", func(t *testing.T) {
		b := ca.ResolveDependency("my-cache")
		if b == nil || b.Concrete != "Illuminate\\Cache\\CacheManager" {
			t.Errorf("got %v", b)
		}
	})

	t.Run("unknown returns nil", func(t *testing.T) {
		if ca.ResolveDependency("unknown") != nil {
			t.Error("expected nil")
		}
	})
}

func TestGetBindings(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "none")
	ca.bindings["x"] = &ServiceBinding{Abstract: "x", Concrete: "X"}

	bindings := ca.GetBindings()
	if len(bindings) != 1 {
		t.Fatalf("expected 1 binding, got %d", len(bindings))
	}
	if bindings["x"].Concrete != "X" {
		t.Error("binding content mismatch")
	}

	// Verify it's a copy
	bindings["y"] = &ServiceBinding{}
	if len(ca.bindings) != 1 {
		t.Error("GetBindings should return a copy")
	}
}

func TestLaravelDefaults(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "laravel")
	ca.Analyze()

	for _, abstract := range []string{"cache", "config", "request", "db", "log", "router"} {
		if b := ca.ResolveDependency(abstract); b == nil {
			t.Errorf("expected default binding for %q", abstract)
		}
	}
}

func TestSymfonyDefaults(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "symfony")
	ca.Analyze()

	for _, abstract := range []string{
		"Psr\\Log\\LoggerInterface",
		"Doctrine\\ORM\\EntityManagerInterface",
		"Twig\\Environment",
	} {
		if b := ca.ResolveDependency(abstract); b == nil {
			t.Errorf("expected default binding for %q", abstract)
		}
	}
}

func TestParseLaravelProvider(t *testing.T) {
	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, "/tmp", "none")

	content := `<?php
namespace App\Providers;
class AppServiceProvider {
    public function register() {
        $this->app->bind('payment', 'App\Services\StripePayment');
        $this->app->singleton('mailer', 'App\Services\MailgunMailer');
    }
}
`
	ca.parseLaravelProvider("AppServiceProvider.php", content)

	if b := ca.ResolveDependency("payment"); b == nil {
		t.Error("expected payment binding")
	} else if b.Singleton {
		t.Error("payment should not be singleton")
	}

	if b := ca.ResolveDependency("mailer"); b == nil {
		t.Error("expected mailer binding")
	} else if !b.Singleton {
		t.Error("mailer should be singleton")
	}
}

func TestParseSymfonyServicesYAML(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	os.MkdirAll(configDir, 0755)

	yaml := `services:
    App\Service\PaymentService:
        class: App\Service\StripePaymentService
    App\Service\MailService:
`
	os.WriteFile(filepath.Join(configDir, "services.yaml"), []byte(yaml), 0644)

	idx := symbols.NewIndex()
	ca := NewContainerAnalyzer(idx, dir, "none")
	ca.parseSymfonyServicesYAML()

	if b := ca.ResolveDependency("App\\Service\\PaymentService"); b == nil {
		t.Error("expected PaymentService binding")
	} else if b.Concrete != "App\\Service\\StripePaymentService" {
		t.Errorf("concrete = %q", b.Concrete)
	}

	if b := ca.ResolveDependency("App\\Service\\MailService"); b == nil {
		t.Error("expected MailService binding")
	}
}

func TestCleanPHPString(t *testing.T) {
	tests := []struct{ input, want string }{
		{"'hello'", "hello"},
		{`"world"`, "world"},
		{"  'trimmed'  ", "trimmed"},
		{"ClassName::class", "ClassName"},
		{"noQuotes", "noQuotes"},
	}
	for _, tt := range tests {
		if got := cleanPHPString(tt.input); got != tt.want {
			t.Errorf("cleanPHPString(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAnalyzeConstructorInjection(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/Controller.php", `<?php
namespace App;
class Controller {
    public function __construct(
        private \Psr\Log\LoggerInterface $logger,
        private string $name
    ) {}
}
`)
	ca := NewContainerAnalyzer(idx, "/tmp", "symfony")
	ca.Analyze()

	deps := ca.AnalyzeConstructorInjection("App\\Controller")
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}

	// LoggerInterface should resolve to a concrete via Symfony defaults
	logDep := deps[0]
	if logDep.TypeHint != "Psr\\Log\\LoggerInterface" {
		t.Errorf("type hint = %q", logDep.TypeHint)
	}
	if logDep.ResolvedConcrete == "" {
		t.Error("expected resolved concrete for LoggerInterface")
	}
}

func TestParseComposerAutoload(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{
		"autoload": {
			"psr-4": {
				"App\\": "src/"
			}
		}
	}`), 0644)

	result := ParseComposerAutoload(dir)
	if result == nil || len(result.PSR4) == 0 {
		t.Error("expected PSR-4 mappings")
	}
	if result.PSR4["App\\"] != "src/" {
		t.Errorf("PSR4 = %v", result.PSR4)
	}

	t.Run("missing composer.json", func(t *testing.T) {
		if ParseComposerAutoload("/nonexistent") != nil {
			t.Error("expected nil for missing file")
		}
	})
}
