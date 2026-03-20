package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// AutoloadEntry represents a single autoload entry (PSR-4 directory or file).
type AutoloadEntry struct {
	Namespace string
	Path      string // Absolute path to the directory or file
	IsVendor  bool
	IsFile    bool
}

type composerJSON struct {
	Autoload    autoloadBlock         `json:"autoload"`
	AutoloadDev autoloadBlock         `json:"autoload-dev"`
}

type autoloadBlock struct {
	PSR4  map[string]interface{} `json:"psr-4"`
	Files []string               `json:"files"`
}

type installedJSON struct {
	Packages []installedPackage `json:"packages"`
}

type installedPackage struct {
	Name        string        `json:"name"`
	InstallPath string        `json:"install-path"`
	Autoload    autoloadBlock `json:"autoload"`
}

// GetAutoloadPaths returns all PSR-4 namespace-to-directory mappings from the
// project's own composer.json (autoload + autoload-dev) and from every
// installed vendor package listed in vendor/composer/installed.json.
func GetAutoloadPaths(rootPath string) []AutoloadEntry {
	var entries []AutoloadEntry

	// 1. Project's own autoload mappings
	projectComposer := filepath.Join(rootPath, "composer.json")
	if data, err := os.ReadFile(projectComposer); err == nil {
		var cj composerJSON
		if json.Unmarshal(data, &cj) == nil {
			entries = append(entries, parsePSR4(cj.Autoload.PSR4, rootPath, false)...)
			entries = append(entries, parsePSR4(cj.AutoloadDev.PSR4, rootPath, false)...)
			entries = append(entries, parseFiles(cj.Autoload.Files, rootPath, false)...)
			entries = append(entries, parseFiles(cj.AutoloadDev.Files, rootPath, false)...)
		}
	}

	// 2. Vendor package autoload mappings from installed.json
	composerDir := filepath.Join(rootPath, "vendor", "composer")
	installedPath := filepath.Join(composerDir, "installed.json")
	data, err := os.ReadFile(installedPath)
	if err != nil {
		return entries
	}

	// Composer v2 format: {"packages": [...]}
	// Composer v1 format: [...]
	var packages []installedPackage
	var v2 installedJSON
	if json.Unmarshal(data, &v2) == nil && v2.Packages != nil {
		packages = v2.Packages
	} else {
		json.Unmarshal(data, &packages)
	}

	for _, pkg := range packages {
		// install-path is relative to vendor/composer/
		pkgDir := filepath.Join(composerDir, pkg.InstallPath)
		pkgDir = filepath.Clean(pkgDir)
		if len(pkg.Autoload.PSR4) > 0 {
			entries = append(entries, parsePSR4(pkg.Autoload.PSR4, pkgDir, true)...)
		}
		if len(pkg.Autoload.Files) > 0 {
			entries = append(entries, parseFiles(pkg.Autoload.Files, pkgDir, true)...)
		}
	}

	return entries
}

func parseFiles(files []string, basePath string, isVendor bool) []AutoloadEntry {
	var entries []AutoloadEntry
	for _, f := range files {
		entries = append(entries, AutoloadEntry{
			Path:     filepath.Join(basePath, f),
			IsVendor: isVendor,
			IsFile:   true,
		})
	}
	return entries
}

// FQNToPath returns the expected file path for a given FQN based on PSR-4 mappings.
// Only considers non-vendor entries. Returns empty string if no mapping matches.
func FQNToPath(fqn string, entries []AutoloadEntry) string {
	var bestNs string
	var bestPath string
	for _, entry := range entries {
		if entry.IsVendor || entry.IsFile || entry.Namespace == "" {
			continue
		}
		prefix := entry.Namespace
		if fqn == prefix || strings.HasPrefix(fqn, prefix+"\\") {
			// Longest prefix match wins
			if len(prefix) > len(bestNs) {
				bestNs = prefix
				bestPath = entry.Path
			}
		}
	}
	if bestNs == "" {
		return ""
	}
	// Strip the namespace prefix from the FQN to get the relative path
	relative := strings.TrimPrefix(fqn, bestNs)
	relative = strings.TrimPrefix(relative, "\\")
	// Convert namespace separators to directory separators
	relative = strings.ReplaceAll(relative, "\\", string(filepath.Separator))
	return filepath.Join(bestPath, relative+".php")
}

// PathToNamespace returns the expected namespace for a file path based on PSR-4 mappings.
// Only considers non-vendor entries. Returns empty string if no mapping matches.
func PathToNamespace(filePath string, entries []AutoloadEntry) string {
	absPath, _ := filepath.Abs(filePath)
	var bestNs string
	var bestLen int
	for _, entry := range entries {
		if entry.IsVendor || entry.IsFile || entry.Namespace == "" {
			continue
		}
		entryAbs, _ := filepath.Abs(entry.Path)
		if strings.HasPrefix(absPath, entryAbs+string(filepath.Separator)) {
			if len(entryAbs) > bestLen {
				bestLen = len(entryAbs)
				bestNs = entry.Namespace
				// Compute relative path and convert to namespace
				rel, _ := filepath.Rel(entryAbs, absPath)
				rel = strings.TrimSuffix(rel, ".php")
				// Get the directory portion (without the class name file)
				dir := filepath.Dir(rel)
				if dir == "." {
					bestNs = entry.Namespace
				} else {
					bestNs = entry.Namespace + "\\" + strings.ReplaceAll(dir, string(filepath.Separator), "\\")
				}
			}
		}
	}
	return bestNs
}

func parsePSR4(psr4 map[string]interface{}, basePath string, isVendor bool) []AutoloadEntry {
	var entries []AutoloadEntry
	for ns, paths := range psr4 {
		ns = strings.TrimRight(ns, "\\")
		var dirs []string
		switch v := paths.(type) {
		case string:
			dirs = []string{v}
		case []interface{}:
			for _, p := range v {
				if s, ok := p.(string); ok {
					dirs = append(dirs, s)
				}
			}
		}
		for _, dir := range dirs {
			absDir := filepath.Join(basePath, dir)
			entries = append(entries, AutoloadEntry{
				Namespace: ns,
				Path:      absDir,
				IsVendor:  isVendor,
			})
		}
	}
	return entries
}
