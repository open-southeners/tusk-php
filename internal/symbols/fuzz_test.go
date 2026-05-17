package symbols

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fuzzSymbolsTestdataRoot returns the absolute path to the repository's testdata directory.
func fuzzSymbolsTestdataRoot() string {
	_, file, _, _ := runtime.Caller(0)
	// internal/symbols/ -> internal/ -> repo root -> testdata/
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// loadSymbolSeedFiles reads all .php files from a directory tree and returns their
// contents as a slice of strings. Unreadable entries are silently skipped.
func loadSymbolSeedFiles(dir string) []string {
	var seeds []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".php") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		seeds = append(seeds, string(data))
		return nil
	})
	return seeds
}

// FuzzIndexFile fuzzes the Index.IndexFile entry point with arbitrary PHP source.
// A fresh Index is created for each input so that no state bleeds between runs.
// The body asserts: no panic.
func FuzzIndexFile(f *testing.F) {
	root := fuzzSymbolsTestdataRoot()

	// Seed from testdata/php-features/*.php
	for _, src := range loadSymbolSeedFiles(filepath.Join(root, "php-features")) {
		f.Add(src)
	}
	// Seed from testdata/tempest/**/*.php
	for _, src := range loadSymbolSeedFiles(filepath.Join(root, "tempest")) {
		f.Add(src)
	}
	// Seed from testdata/project/
	for _, src := range loadSymbolSeedFiles(filepath.Join(root, "project")) {
		f.Add(src)
	}
	// Inline seeds
	f.Add(`<?php`)
	f.Add(``)
	f.Add(`<?php class Foo {}`)
	f.Add(`<?php namespace App; class Bar extends Baz implements Qux { public function m(): void {} }`)
	f.Add(`<?php enum Status: string { case Active = 'active'; case Inactive = 'inactive'; }`)
	f.Add(`<?php interface I { public function m(): void; }`)
	f.Add(`<?php trait T { public string $x; public function getX(): string { return $this->x; } }`)
	f.Add(`<?php namespace App; use Monolog\Logger; class Service { private Logger $log; }`)
	f.Add(`<?php function foo(int|string $v): never { throw new \Exception('e'); }`)
	f.Add(`<?php readonly class Point { public function __construct(public float $x, public float $y) {} }`)
	f.Add(`<?php @@@ $$$ %%% ^^^`)

	f.Fuzz(func(t *testing.T, src string) {
		// IndexFile must never panic regardless of input.
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("IndexFile panicked: %v", r)
				}
			}()
			idx := NewIndex()
			idx.IndexFile("file:///fuzz/test.php", src)
		}()
	})
}
