package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/open-southeners/php-lsp/internal/protocol"
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
	PHPStanEnabled     *bool    `json:"phpstanEnabled,omitempty"`
	PHPStanPath        string   `json:"phpstanPath,omitempty"`
	PHPStanLevel       string   `json:"phpstanLevel,omitempty"`
	PHPStanConfig      string   `json:"phpstanConfig,omitempty"`
	PintEnabled        *bool    `json:"pintEnabled,omitempty"`
	PintPath           string   `json:"pintPath,omitempty"`
	PintConfig         string   `json:"pintConfig,omitempty"`
	DatabaseEnabled    *bool    `json:"databaseEnabled,omitempty"`
	DiagnosticRules    map[string]bool `json:"diagnosticRules,omitempty"`
	MaxIndexFiles      int             `json:"maxIndexFiles"`
	StubsPath          string          `json:"stubsPath"`
	LogLevel           string          `json:"logLevel"`
	LogFile            string          `json:"logFile"`
}

// IsRuleEnabled returns whether a diagnostic rule is enabled.
// Rules default to enabled if not explicitly configured.
func (c *Config) IsRuleEnabled(code string) bool {
	if c.DiagnosticRules == nil {
		return true
	}
	enabled, ok := c.DiagnosticRules[code]
	if !ok {
		return true
	}
	return enabled
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

// MergeClientOptions applies client-provided initializationOptions over
// the current config. Only non-zero values from the client override.
func (c *Config) MergeClientOptions(opts *protocol.InitializationOptions) {
	if opts.PHPVersion != "" {
		c.PHPVersion = opts.PHPVersion
	}
	if opts.Framework != "" {
		c.Framework = opts.Framework
	}
	if opts.ContainerAware != nil {
		c.ContainerAware = *opts.ContainerAware
	}
	if opts.DiagnosticsEnabled != nil {
		c.DiagnosticsEnabled = *opts.DiagnosticsEnabled
	}
	if opts.PHPStanEnabled != nil {
		c.PHPStanEnabled = opts.PHPStanEnabled
	}
	if opts.PHPStanPath != "" {
		c.PHPStanPath = opts.PHPStanPath
	}
	if opts.PHPStanLevel != "" {
		c.PHPStanLevel = opts.PHPStanLevel
	}
	if opts.PHPStanConfig != "" {
		c.PHPStanConfig = opts.PHPStanConfig
	}
	if opts.PintEnabled != nil {
		c.PintEnabled = opts.PintEnabled
	}
	if opts.PintPath != "" {
		c.PintPath = opts.PintPath
	}
	if opts.PintConfig != "" {
		c.PintConfig = opts.PintConfig
	}
	if opts.DatabaseEnabled != nil {
		c.DatabaseEnabled = opts.DatabaseEnabled
	}
	if opts.MaxIndexFiles != nil {
		c.MaxIndexFiles = *opts.MaxIndexFiles
	}
	if len(opts.ExcludePaths) > 0 {
		c.ExcludePaths = opts.ExcludePaths
	}
}

// IsDatabaseEnabled returns whether database introspection is enabled (default: true).
func (c *Config) IsDatabaseEnabled() bool {
	if c.DatabaseEnabled == nil {
		return true
	}
	return *c.DatabaseEnabled
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
