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

	// TypeResolver resolves the type of a PHP expression for redundant nullsafe checks.
	// Set by the LSP server after initialization.
	TypeResolver func(expr string, source string, line int, file *parser.FileNode) string

	// BuilderModelResolver resolves the Eloquent model FQN from a builder chain.
	// Set by the LSP server after initialization.
	BuilderModelResolver checks.ModelResolverFunc

	// BuilderMemberChecker validates columns/relations on models.
	// Set by the LSP server after initialization.
	BuilderMemberChecker checks.MemberChecker

	mu          sync.RWMutex
	toolResults map[string][]protocol.Diagnostic
	saveResults map[string][]protocol.Diagnostic
}

func NewProvider(index *symbols.Index, framework, rootPath string, logger *log.Logger, cfg *config.Config) *Provider {
	p := &Provider{
		index:       index,
		framework:   framework,
		rootPath:    rootPath,
		logger:      logger,
		cfg:         cfg,
		toolResults: make(map[string][]protocol.Diagnostic),
		saveResults: make(map[string][]protocol.Diagnostic),
	}
	p.phpstan = newPHPStanRunner(rootPath, cfg, logger)
	p.pint = newPintRunner(rootPath, cfg, logger)
	return p
}

// Analyze runs fast static checks and merges cached external tool results.
// Called on every file change — only includes lightweight checks.
func (p *Provider) Analyze(uri, source string) []protocol.Diagnostic {
	var diags []protocol.Diagnostic
	file := parser.ParseFile(source)
	if file != nil {
		diags = append(diags, p.checkDeprecations(source)...)
		diags = append(diags, p.checkClassStructure(file)...)
		if p.cfg.IsRuleEnabled("unused-import") {
			diags = append(diags, p.checkUnusedImports(file, source)...)
		}
		if p.cfg.IsRuleEnabled("unused-private-method") || p.cfg.IsRuleEnabled("unused-private-property") {
			diags = append(diags, p.filterByConfig(findingsToDiagnostics((&checks.UnusedPrivateRule{}).Check(file, source, p.index)))...)
		}
		if p.cfg.IsRuleEnabled("unreachable-code") {
			diags = append(diags, findingsToDiagnostics((&checks.UnreachableCodeRule{}).Check(file, source, p.index))...)
		}
		if p.cfg.IsRuleEnabled("redundant-union-member") {
			diags = append(diags, findingsToDiagnostics((&checks.RedundantUnionRule{}).Check(file, source, p.index))...)
		}
	}
	p.mu.RLock()
	if cached, ok := p.toolResults[uri]; ok {
		diags = append(diags, cached...)
	}
	if cached, ok := p.saveResults[uri]; ok {
		diags = append(diags, cached...)
	}
	p.mu.RUnlock()
	return diags
}

// AnalyzeOnSave runs heavier checks that need type resolution or model lookups.
// Called on file save only. Results are cached and merged into Analyze() output.
func (p *Provider) AnalyzeOnSave(uri, source string) {
	file := parser.ParseFile(source)
	if file == nil {
		return
	}
	var diags []protocol.Diagnostic

	// Redundant nullsafe — needs type resolution
	if p.TypeResolver != nil && p.cfg.IsRuleEnabled("redundant-nullsafe") {
		rule := &checks.RedundantNullsafeRule{TypeResolver: p.TypeResolver}
		diags = append(diags, findingsToDiagnostics(rule.Check(file, source, p.index))...)
	}

	// Invalid builder args — needs model resolution + member checking
	if p.BuilderModelResolver != nil && p.BuilderMemberChecker != nil {
		if p.cfg.IsRuleEnabled("unknown-column") || p.cfg.IsRuleEnabled("unknown-relation") {
			rule := &checks.InvalidBuilderArgRule{
				ModelResolver: p.BuilderModelResolver,
				Members:       p.BuilderMemberChecker,
			}
			diags = append(diags, p.filterByConfig(findingsToDiagnostics(rule.Check(file, source, p.index)))...)
		}
	}

	p.mu.Lock()
	p.saveResults[uri] = diags
	p.mu.Unlock()
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
	delete(p.saveResults, uri)
	p.mu.Unlock()
}

// filterByConfig removes diagnostics whose Code is disabled in config.
func (p *Provider) filterByConfig(diags []protocol.Diagnostic) []protocol.Diagnostic {
	var filtered []protocol.Diagnostic
	for _, d := range diags {
		if d.Code == "" || p.cfg.IsRuleEnabled(d.Code) {
			filtered = append(filtered, d)
		}
	}
	return filtered
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
					Tags: []protocol.DiagnosticTag{protocol.DiagnosticTagDeprecated},
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
		d := protocol.Diagnostic{
			Range: protocol.Range{
				Start: protocol.Position{Line: f.StartLine, Character: f.StartCol},
				End:   protocol.Position{Line: f.EndLine, Character: f.EndCol},
			},
			Severity: sev,
			Source:   "php-lsp",
			Message:  f.Message,
			Code:     f.Code,
		}
		for _, tag := range f.Tags {
			d.Tags = append(d.Tags, protocol.DiagnosticTag(tag))
		}
		diags = append(diags, d)
	}
	return diags
}
