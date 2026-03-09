package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds the LSP server configuration.
type Config struct {
	PHPVersion         string   `json:"phpVersion"`
	Framework          string   `json:"framework"`
	ComposerPath       string   `json:"composerPath"`
	IncludePaths       []string `json:"includePaths"`
	ExcludePaths       []string `json:"excludePaths"`
	ContainerAware     bool     `json:"containerAware"`
	DiagnosticsEnabled bool     `json:"diagnosticsEnabled"`
	MaxIndexFiles      int      `json:"maxIndexFiles"`
	StubsPath          string   `json:"stubsPath"`
	LogLevel           string   `json:"logLevel"`
	LogFile            string   `json:"logFile"`
}

func DefaultConfig() *Config {
	return &Config{
		PHPVersion:         "8.5",
		Framework:          "auto",
		IncludePaths:       []string{"src", "app", "lib"},
		ExcludePaths:       []string{"vendor", "node_modules", ".git", "storage", "var/cache"},
		ContainerAware:     true,
		DiagnosticsEnabled: true,
		MaxIndexFiles:      10000,
		LogLevel:           "info",
		LogFile:            "",
	}
}

func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func DetectFramework(rootPath string) string {
	if fileExists(filepath.Join(rootPath, "artisan")) &&
		fileExists(filepath.Join(rootPath, "app", "Providers", "AppServiceProvider.php")) {
		return "laravel"
	}
	if fileExists(filepath.Join(rootPath, "bin", "console")) &&
		(dirExists(filepath.Join(rootPath, "config", "packages")) ||
			fileExists(filepath.Join(rootPath, "symfony.lock"))) {
		return "symfony"
	}
	composerPath := filepath.Join(rootPath, "composer.json")
	if data, err := os.ReadFile(composerPath); err == nil {
		var composer struct {
			Require map[string]string `json:"require"`
		}
		if json.Unmarshal(data, &composer) == nil {
			if _, ok := composer.Require["laravel/framework"]; ok {
				return "laravel"
			}
			if _, ok := composer.Require["symfony/framework-bundle"]; ok {
				return "symfony"
			}
		}
	}
	return "none"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
