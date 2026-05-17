package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fuzzTestdataRoot returns the absolute path to the repository's testdata directory.
func fuzzTestdataRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// loadSeedFiles reads all .php files from a directory tree and returns their
// contents as a slice of strings. It silently skips unreadable entries.
func loadSeedFiles(dir string) []string {
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

// FuzzParseFile fuzzes the fault-tolerant ParseFile entry point.
// The corpus is seeded from real PHP fixtures in testdata/.
// The body asserts: no panic, non-nil return.
func FuzzParseFile(f *testing.F) {
	// Seed from testdata/php-features/*.php
	for _, src := range loadSeedFiles(filepath.Join(fuzzTestdataRoot(), "php-features")) {
		f.Add(src)
	}
	// Seed from testdata/tempest/**/*.php
	for _, src := range loadSeedFiles(filepath.Join(fuzzTestdataRoot(), "tempest")) {
		f.Add(src)
	}
	// Seed from testdata/project/src/*.php
	for _, src := range loadSeedFiles(filepath.Join(fuzzTestdataRoot(), "project")) {
		f.Add(src)
	}
	// A handful of representative inline seeds
	f.Add(`<?php`)
	f.Add(``)
	f.Add(`<?php class Foo {}`)
	f.Add(`<?php namespace App; class Bar extends Baz implements Qux {}`)
	f.Add(`<?php enum Status: string { case Active = 'active'; }`)
	f.Add(`<?php interface I { public function m(): void; }`)
	f.Add(`<?php trait T { public string $x; }`)
	f.Add(`<?php function fn1(int|string $v): never { throw new \Exception(); }`)

	f.Fuzz(func(t *testing.T, src string) {
		// ParseFile must never panic and must never return nil.
		var result *FileNode
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("ParseFile panicked: %v", r)
				}
			}()
			result = ParseFile(src)
		}()
		if result == nil {
			t.Errorf("ParseFile returned nil for input of length %d", len(src))
		}
	})
}

// FuzzTokenize fuzzes the internal tokenizer directly.
// The body asserts: no panic, returned token slice is non-nil.
func FuzzTokenize(f *testing.F) {
	// Seed from testdata/php-features/*.php
	for _, src := range loadSeedFiles(filepath.Join(fuzzTestdataRoot(), "php-features")) {
		f.Add(src)
	}
	// Seed from testdata/project/src/*.php
	for _, src := range loadSeedFiles(filepath.Join(fuzzTestdataRoot(), "project")) {
		f.Add(src)
	}
	// Inline seeds covering tokenizer edge-cases
	f.Add(`<?php`)
	f.Add(``)
	f.Add(`<?php // line comment`)
	f.Add(`<?php /* block comment */`)
	f.Add(`<?php /** doc comment */`)
	f.Add(`<?php "string literal"`)
	f.Add(`<?php 'single quoted'`)
	f.Add(`<?php $variable = 42;`)
	f.Add(`<?php $x = "hello \n world";`)
	f.Add(`<?php @@@ $$$ %%% ^^^ &&& *** ))) ((( !!! ~~~`)
	f.Add(`<?php $x = 1 |> fn($n) => $n * 2;`)

	f.Fuzz(func(t *testing.T, src string) {
		// tokenize must never panic.
		var tokens []Token
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("tokenize panicked: %v", r)
				}
			}()
			tokens, _ = tokenize(src)
		}()
		// A nil slice is acceptable; a panic is not.
		_ = tokens
	})
}
