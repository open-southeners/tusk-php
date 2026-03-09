package diagnostics

import (
	"fmt"
	"strings"

	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

type Provider struct {
	index     *symbols.Index
	framework string
}

func NewProvider(index *symbols.Index, framework string) *Provider {
	return &Provider{index: index, framework: framework}
}

func (p *Provider) Analyze(uri, source string) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	file := parser.ParseFile(source)
	if file == nil {
		return diags
	}
	diags = append(diags, p.checkDeprecations(source)...)
	diags = append(diags, p.checkClassStructure(file)...)
	diags = append(diags, p.checkUnusedImports(file, source)...)
	return diags
}

func (p *Provider) checkDeprecations(source string) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	lines := strings.Split(source, "\n")
	patterns := [][2]string{
		{"each(", "each() is deprecated since PHP 7.2"},
		{"create_function(", "create_function() is deprecated since PHP 7.2"},
		{"utf8_encode(", "utf8_encode() is deprecated since PHP 8.2, use mb_convert_encoding()"},
		{"utf8_decode(", "utf8_decode() is deprecated since PHP 8.2, use mb_convert_encoding()"},
	}
	for i, line := range lines {
		for _, dep := range patterns {
			if col := strings.Index(line, dep[0]); col >= 0 {
				diags = append(diags, protocol.Diagnostic{
					Range:    protocol.Range{Start: protocol.Position{Line: i, Character: col}, End: protocol.Position{Line: i, Character: col + len(dep[0])}},
					Severity: protocol.DiagnosticSeverityWarning, Source: "php-lsp", Message: dep[1], Code: "deprecated",
				})
			}
		}
	}
	return diags
}

func (p *Provider) checkClassStructure(file *parser.FileNode) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	for _, cls := range file.Classes {
		if !cls.IsAbstract {
			for _, m := range cls.Methods {
				if m.IsAbstract {
					diags = append(diags, protocol.Diagnostic{
						Range:    protocol.Range{Start: protocol.Position{Line: m.StartLine}},
						Severity: protocol.DiagnosticSeverityError, Source: "php-lsp",
						Message:  fmt.Sprintf("Class '%s' contains abstract method '%s' but is not declared abstract", cls.Name, m.Name),
						Code:     "abstract-in-concrete",
					})
				}
			}
		}
	}
	return diags
}

func (p *Provider) checkUnusedImports(file *parser.FileNode, source string) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	for _, u := range file.Uses {
		useLine := fmt.Sprintf("use %s", u.FullName)
		remaining := strings.Replace(source, useLine, "", 1)
		if !strings.Contains(remaining, u.Alias) {
			diags = append(diags, protocol.Diagnostic{
				Range:    protocol.Range{Start: protocol.Position{Line: u.StartLine}},
				Severity: protocol.DiagnosticSeverityHint, Source: "php-lsp",
				Message:  fmt.Sprintf("Unused import '%s'", u.FullName), Code: "unused-import",
			})
		}
	}
	return diags
}
