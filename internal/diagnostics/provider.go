package diagnostics

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/open-southeners/php-lsp/internal/checks"
	"github.com/open-southeners/php-lsp/internal/config"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

type Provider struct {
	index     *symbols.Index
	framework string
	rootPath  string
	logger    *log.Logger
	cfg       *config.Config
	phpstan   *phpstanRunner
	pint      *pintRunner

	mu          sync.RWMutex
	toolResults map[string][]protocol.Diagnostic
}

func NewProvider(index *symbols.Index, framework, rootPath string, logger *log.Logger, cfg *config.Config) *Provider {
	p := &Provider{
		index:       index,
		framework:   framework,
		rootPath:    rootPath,
		logger:      logger,
		cfg:         cfg,
		toolResults: make(map[string][]protocol.Diagnostic),
	}
	p.phpstan = newPHPStanRunner(rootPath, cfg, logger)
	p.pint = newPintRunner(rootPath, cfg, logger)
	return p
}

// Analyze runs fast static checks and merges cached external tool results.
func (p *Provider) Analyze(uri, source string) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	file := parser.ParseFile(source)
	if file != nil {
		diags = append(diags, p.checkDeprecations(source)...)
		diags = append(diags, p.checkClassStructure(file)...)
		diags = append(diags, p.checkUnusedImports(file, source)...)
		diags = append(diags, findingsToDiagnostics((&checks.UnusedPrivateRule{}).Check(file, source, p.index))...)
		diags = append(diags, findingsToDiagnostics((&checks.UnreachableCodeRule{}).Check(file, source, p.index))...)
	}
	p.mu.RLock()
	if cached, ok := p.toolResults[uri]; ok {
		diags = append(diags, cached...)
	}
	p.mu.RUnlock()
	return diags
}

// RunTools executes external analysis tools (PHPStan, Pint) on a file.
// Results are cached and included in subsequent Analyze calls.
func (p *Provider) RunTools(uri, filePath string) {
	var toolDiags []protocol.Diagnostic
	if p.phpstan.available() {
		toolDiags = append(toolDiags, p.phpstan.analyze(filePath)...)
	}
	if p.pint.available() {
		toolDiags = append(toolDiags, p.pint.analyze(filePath)...)
	}
	p.mu.Lock()
	p.toolResults[uri] = toolDiags
	p.mu.Unlock()
}

// ClearCache removes cached tool results for a URI.
func (p *Provider) ClearCache(uri string) {
	p.mu.Lock()
	delete(p.toolResults, uri)
	p.mu.Unlock()
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
					Range:    protocol.Range{Start: protocol.Position{Line: i, Character: col}, End: protocol.Position{Line: i, Character: col + len(dep[0]) - 1}},
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
	rule := &checks.UnusedImportsRule{}
	return findingsToDiagnostics(rule.Check(file, source, p.index))
}

// findingsToDiagnostics converts standalone check findings to LSP diagnostics.
func findingsToDiagnostics(findings []checks.Finding) []protocol.Diagnostic {
	diags := make([]protocol.Diagnostic, 0, len(findings))
	for _, f := range findings {
		sev := protocol.DiagnosticSeverityHint
		switch f.Severity {
		case checks.SeverityError:
			sev = protocol.DiagnosticSeverityError
		case checks.SeverityWarning:
			sev = protocol.DiagnosticSeverityWarning
		case checks.SeverityInfo:
			sev = protocol.DiagnosticSeverityInformation
		case checks.SeverityHint:
			sev = protocol.DiagnosticSeverityHint
		}
		diags = append(diags, protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: f.StartLine, Character: f.StartCol},
				End:   protocol.Position{Line: f.EndLine, Character: f.EndCol},
			},
			Severity: sev,
			Source:   "php-lsp",
			Message:  f.Message,
			Code:     f.Code,
		})
	}
	return diags
}
