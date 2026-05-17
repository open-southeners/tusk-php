package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ecosystemTestdataRoot returns the absolute path to the repository's testdata directory.
func ecosystemTestdataRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// internal/parser/ -> internal/ -> repo root -> testdata/
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// TestPhpFeaturesParseClean walks testdata/php-features/*.php and asserts that
// each file parses without panicking, returns a non-nil result, and produces
// zero parse errors (clean modern PHP must parse without errors).
func TestPhpFeaturesParseClean(t *testing.T) {
	featuresDir := filepath.Join(ecosystemTestdataRoot(), "php-features")

	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		t.Fatalf("cannot read testdata/php-features: %v", err)
	}

	found := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".php") {
			continue
		}
		found++
		name := entry.Name()
		path := filepath.Join(featuresDir, name)

		t.Run(name, func(t *testing.T) {
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("cannot read %s: %v", path, readErr)
			}
			sourceStr := string(src)

			// ParseFile (fault-tolerant wrapper) must not panic and must not return nil.
			var fileNode *FileNode
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("ParseFile panicked on %s: %v", name, r)
					}
				}()
				fileNode = ParseFile(sourceStr)
			}()
			if fileNode == nil {
				t.Errorf("ParseFile returned nil for %s", name)
			}

			// Use the parser directly to check for parse errors; clean modern PHP must
			// produce zero errors.
			var result *ParseResult
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Parse() panicked on %s: %v", name, r)
					}
				}()
				result = New().Parse(sourceStr)
			}()
			if result == nil {
				t.Errorf("Parse() returned nil for %s", name)
				return
			}
			if len(result.Errors) != 0 {
				for _, e := range result.Errors {
					t.Errorf("%s: unexpected parse error at line %d col %d: %s",
						name, e.Line, e.Column, e.Message)
				}
			}
		})
	}

	if found == 0 {
		t.Fatalf("no .php files found in testdata/php-features — check test setup")
	}
}

// TestTempestFixtureParseNoPanic walks testdata/tempest/**/*.php and asserts that
// each file parses without panicking and returns a non-nil result. Parse errors
// are logged but do not fail the test (Tempest stubs reference framework types
// that are not in the fixture's vendor directory).
func TestTempestFixtureParseNoPanic(t *testing.T) {
	tempestDir := filepath.Join(ecosystemTestdataRoot(), "tempest")

	found := 0
	walkErr := filepath.WalkDir(tempestDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".php") {
			return nil
		}
		found++

		rel, _ := filepath.Rel(tempestDir, path)
		t.Run(rel, func(t *testing.T) {
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatalf("cannot read %s: %v", path, readErr)
			}
			sourceStr := string(src)

			// ParseFile (fault-tolerant wrapper) must not panic and must not return nil.
			var fileNode *FileNode
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("ParseFile panicked on %s: %v", rel, r)
					}
				}()
				fileNode = ParseFile(sourceStr)
			}()
			if fileNode == nil {
				t.Errorf("ParseFile returned nil for %s", rel)
				return
			}

			// Also exercise the direct parser; log (but do not fail on) parse errors.
			var result *ParseResult
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Parse() panicked on %s: %v", rel, r)
					}
				}()
				result = New().Parse(sourceStr)
			}()
			if result == nil {
				t.Errorf("Parse() returned nil for %s", rel)
				return
			}
			for _, e := range result.Errors {
				t.Logf("%s: parse error at line %d col %d: %s",
					rel, e.Line, e.Column, e.Message)
			}
		})

		return nil
	})

	if walkErr != nil {
		t.Fatalf("error walking testdata/tempest: %v", walkErr)
	}
	if found == 0 {
		t.Fatalf("no .php files found in testdata/tempest — check test setup")
	}
}
