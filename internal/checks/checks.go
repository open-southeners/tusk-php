// Package checks provides standalone PHP static analysis rules that can be
// used by the LSP server, CLI tools, or CI pipelines. It has no dependency
// on LSP protocol types — all results are returned as Finding values.
package checks

import (
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

// Finding represents a single diagnostic result.
type Finding struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
	Severity  Severity
	Code      string // machine-readable: "unused-import", "unused-private-method", etc.
	Message   string // human-readable description
}

// Severity levels mirror LSP DiagnosticSeverity values.
type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// Rule is a single diagnostic check that can be enabled/disabled.
type Rule interface {
	Code() string
	Check(file *parser.FileNode, source string, index *symbols.Index) []Finding
}

// CheckFile runs all provided rules on a single file and returns the
// combined findings.
func CheckFile(file *parser.FileNode, source string, index *symbols.Index, rules []Rule) []Finding {
	var all []Finding
	for _, r := range rules {
		all = append(all, r.Check(file, source, index)...)
	}
	return all
}

// AllRules returns all built-in rules with default configuration.
func AllRules() []Rule {
	return []Rule{
		&UnusedImportsRule{},
		&UnusedPrivateRule{},
		&UnreachableCodeRule{},
	}
}
