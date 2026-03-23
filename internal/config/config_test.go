package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PHPVersion != "8.5" {
		t.Errorf("PHPVersion = %q", cfg.PHPVersion)
	}
	if cfg.Framework != "auto" {
		t.Errorf("Framework = %q", cfg.Framework)
	}
	if !cfg.DiagnosticsEnabled {
		t.Error("DiagnosticsEnabled should default to true")
	}
	if !cfg.ContainerAware {
		t.Error("ContainerAware should default to true")
	}
	if cfg.MaxIndexFiles != 10000 {
		t.Errorf("MaxIndexFiles = %d", cfg.MaxIndexFiles)
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".php-lsp.json")
		os.WriteFile(path, []byte(`{"phpVersion":"8.3","framework":"laravel"}`), 0644)
		cfg, err := LoadFromFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PHPVersion != "8.3" {
			t.Errorf("PHPVersion = %q", cfg.PHPVersion)
		}
		if cfg.Framework != "laravel" {
			t.Errorf("Framework = %q", cfg.Framework)
		}
		// Defaults preserved for unset fields
		if !cfg.DiagnosticsEnabled {
			t.Error("DiagnosticsEnabled should still be true")
		}
	})

	t.Run("missing file returns defaults", func(t *testing.T) {
		cfg, err := LoadFromFile("/nonexistent/.php-lsp.json")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PHPVersion != "8.5" {
			t.Errorf("expected default PHPVersion, got %q", cfg.PHPVersion)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".php-lsp.json")
		os.WriteFile(path, []byte(`{invalid`), 0644)
		_, err := LoadFromFile(path)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestMergeClientOptions(t *testing.T) {
	cfg := DefaultConfig()

	boolTrue := true
	boolFalse := false
	maxFiles := 5000

	opts := &protocol.InitializationOptions{
		PHPVersion:         "8.2",
		Framework:          "symfony",
		ContainerAware:     &boolFalse,
		DiagnosticsEnabled: &boolTrue,
		PHPStanEnabled:     &boolTrue,
		PHPStanPath:        "/usr/bin/phpstan",
		PHPStanLevel:       "9",
		PHPStanConfig:      "phpstan.neon",
		PintEnabled:        &boolFalse,
		PintPath:           "/usr/bin/pint",
		PintConfig:         "pint.json",
		DatabaseEnabled:    &boolTrue,
		MaxIndexFiles:      &maxFiles,
		ExcludePaths:       []string{"vendor"},
	}
	cfg.MergeClientOptions(opts)

	if cfg.PHPVersion != "8.2" {
		t.Errorf("PHPVersion = %q", cfg.PHPVersion)
	}
	if cfg.Framework != "symfony" {
		t.Errorf("Framework = %q", cfg.Framework)
	}
	if cfg.ContainerAware {
		t.Error("ContainerAware should be false")
	}
	if cfg.MaxIndexFiles != 5000 {
		t.Errorf("MaxIndexFiles = %d", cfg.MaxIndexFiles)
	}

	t.Run("zero values don't override", func(t *testing.T) {
		cfg2 := DefaultConfig()
		cfg2.MergeClientOptions(&protocol.InitializationOptions{})
		if cfg2.PHPVersion != "8.5" {
			t.Error("empty string shouldn't override PHPVersion")
		}
	})
}

func TestIsRuleEnabled(t *testing.T) {
	t.Run("nil map returns true", func(t *testing.T) {
		cfg := &Config{}
		if !cfg.IsRuleEnabled("unused-import") {
			t.Error("nil map should return true")
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]bool{"unused-import": true}}
		if !cfg.IsRuleEnabled("unused-import") {
			t.Error("explicit true should return true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]bool{"unused-import": false}}
		if cfg.IsRuleEnabled("unused-import") {
			t.Error("explicit false should return false")
		}
	})

	t.Run("missing key returns true", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]bool{"other-rule": false}}
		if !cfg.IsRuleEnabled("unused-import") {
			t.Error("missing key should default to true")
		}
	})
}

func TestIsDatabaseEnabled(t *testing.T) {
	t.Run("nil returns true", func(t *testing.T) {
		cfg := &Config{}
		if !cfg.IsDatabaseEnabled() {
			t.Error("nil should return true")
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		b := true
		cfg := &Config{DatabaseEnabled: &b}
		if !cfg.IsDatabaseEnabled() {
			t.Error("should be true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		b := false
		cfg := &Config{DatabaseEnabled: &b}
		if cfg.IsDatabaseEnabled() {
			t.Error("should be false")
		}
	})
}

func TestDetectFramework(t *testing.T) {
	t.Run("laravel with artisan", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php"), 0644)
		os.MkdirAll(filepath.Join(dir, "app", "Providers"), 0755)
		os.WriteFile(filepath.Join(dir, "app", "Providers", "AppServiceProvider.php"), []byte("<?php"), 0644)
		if f := DetectFramework(dir); f != "laravel" {
			t.Errorf("expected laravel, got %q", f)
		}
	})

	t.Run("symfony with bin/console", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "bin"), 0755)
		os.WriteFile(filepath.Join(dir, "bin", "console"), []byte("#!/usr/bin/env php"), 0644)
		os.MkdirAll(filepath.Join(dir, "config", "packages"), 0755)
		if f := DetectFramework(dir); f != "symfony" {
			t.Errorf("expected symfony, got %q", f)
		}
	})

	t.Run("laravel from composer.json", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"laravel/framework":"^11.0"}}`), 0644)
		if f := DetectFramework(dir); f != "laravel" {
			t.Errorf("expected laravel, got %q", f)
		}
	})

	t.Run("no framework", func(t *testing.T) {
		dir := t.TempDir()
		if f := DetectFramework(dir); f != "none" {
			t.Errorf("expected none, got %q", f)
		}
	})
}
