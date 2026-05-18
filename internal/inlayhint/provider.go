// Package inlayhint implements the textDocument/inlayHint LSP request handler.
// It provides five categories of inline type and parameter-name annotations for
// PHP source files: variable type hints, foreach variable types, closure return
// types, method return types, and call-site parameter name labels.
package inlayhint

import (
	"sort"

	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// Provider produces inlay hints for a PHP source file.
// It is designed to be constructed once and reused across requests.
type Provider struct {
	index     *symbols.Index
	container *container.ContainerAnalyzer
	resolver  *resolve.Resolver
}

// NewProvider creates a Provider backed by the shared symbol index and optional
// container analyzer.  Call SetTypedChainResolver after construction to wire
// chain-expression resolution (required for variable-type and param-name hints
// that involve method chains).
func NewProvider(index *symbols.Index, ca *container.ContainerAnalyzer) *Provider {
	return &Provider{
		index:     index,
		container: ca,
		resolver:  resolve.NewResolver(index),
	}
}

// SetTypedChainResolver wires chain resolution to an external implementation —
// typically the completion provider's ResolveExpressionTypeTyped.  It also
// derives the plain-string ChainResolver from it so that both callbacks are
// always in sync.
func (p *Provider) SetTypedChainResolver(
	fn func(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType,
) {
	p.resolver.TypedChainResolver = fn
	p.resolver.ChainResolver = func(expr, source string, pos protocol.Position, file *parser.FileNode) string {
		return fn(expr, source, pos, file).String()
	}
}

// GetInlayHints returns all inlay hints for the given source file, filtered by
// the configuration flags.  The returned slice is sorted by line then character
// for deterministic output.  Range filtering (to params.Range) is performed by
// the LSP handler, not here.
func (p *Provider) GetInlayHints(uri, source string, cfg *config.InlayHintsConfig) []protocol.InlayHint {
	if !cfg.Enabled {
		return nil
	}

	file := parser.ParseFile(source)
	result := parser.New().Parse(source)
	lines := resolve.SplitLines(source)

	var hints []protocol.InlayHint

	if cfg.VariableTypes {
		hints = append(hints, p.collectVarTypeHints(result, lines, file, source)...)
	}
	if cfg.ForeachTypes {
		hints = append(hints, p.collectForeachHints(result, lines, file, source)...)
	}
	if cfg.ClosureReturnTypes {
		hints = append(hints, p.collectClosureReturnHints(result, lines, file, source)...)
	}
	if cfg.ReturnTypes {
		hints = append(hints, p.collectMethodReturnHints(result, file)...)
	}
	if cfg.ParameterNames {
		hints = append(hints, p.collectParamNameHints(result, lines, file, source, cfg)...)
	}

	sort.SliceStable(hints, func(i, j int) bool {
		a, b := hints[i].Position, hints[j].Position
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Character < b.Character
	})
	return hints
}
