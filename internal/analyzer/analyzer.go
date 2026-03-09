package analyzer

import (
	"strings"

	"github.com/open-southeners/php-lsp/internal/container"
	"github.com/open-southeners/php-lsp/internal/parser"
	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/symbols"
)

type Analyzer struct {
	index     *symbols.Index
	container *container.ContainerAnalyzer
}

func NewAnalyzer(index *symbols.Index, ca *container.ContainerAnalyzer) *Analyzer {
	return &Analyzer{index: index, container: ca}
}

func (a *Analyzer) FindDefinition(uri, source string, pos protocol.Position) *protocol.Location {
	word := getWordAt(source, pos)
	if word == "" { return nil }
	file := parser.ParseFile(source)
	if file != nil {
		for _, u := range file.Uses {
			if u.Alias == word {
				for _, sym := range a.index.LookupByName(word) {
					if sym.FQN == u.FullName && sym.URI != "builtin" {
						return &protocol.Location{URI: sym.URI, Range: sym.Range}
					}
				}
			}
		}
	}
	for _, sym := range a.index.LookupByName(word) {
		if sym.URI != "builtin" { return &protocol.Location{URI: sym.URI, Range: sym.Range} }
	}
	return nil
}

func (a *Analyzer) FindReferences(uri, source string, pos protocol.Position) []protocol.Location {
	word := getWordAt(source, pos)
	if word == "" { return nil }
	var locs []protocol.Location
	for _, sym := range a.index.LookupByName(word) {
		if sym.URI != "builtin" { locs = append(locs, protocol.Location{URI: sym.URI, Range: sym.Range}) }
		if sym.Kind == symbols.KindInterface {
			for _, impl := range a.index.GetImplementors(sym.FQN) {
				if impl.URI != "builtin" { locs = append(locs, protocol.Location{URI: impl.URI, Range: impl.Range}) }
			}
		}
	}
	return locs
}

func (a *Analyzer) GetDocumentSymbols(uri, source string) []protocol.DocumentSymbol {
	file := parser.ParseFile(source)
	if file == nil { return nil }
	var ds []protocol.DocumentSymbol
	for _, cls := range file.Classes {
		s := protocol.DocumentSymbol{Name: cls.Name, Kind: protocol.SymbolKindClass,
			Range: mkRange(cls.StartLine), SelectionRange: mkRange(cls.StartLine)}
		for _, m := range cls.Methods {
			s.Children = append(s.Children, protocol.DocumentSymbol{Name: m.Name, Detail: m.Visibility, Kind: protocol.SymbolKindMethod, Range: mkRange(m.StartLine), SelectionRange: mkRange(m.StartLine)})
		}
		for _, p := range cls.Properties {
			s.Children = append(s.Children, protocol.DocumentSymbol{Name: p.Name, Detail: p.Type.Name, Kind: protocol.SymbolKindProperty, Range: mkRange(p.StartLine), SelectionRange: mkRange(p.StartLine)})
		}
		ds = append(ds, s)
	}
	for _, iface := range file.Interfaces {
		s := protocol.DocumentSymbol{Name: iface.Name, Kind: protocol.SymbolKindInterface, Range: mkRange(iface.StartLine), SelectionRange: mkRange(iface.StartLine)}
		ds = append(ds, s)
	}
	for _, en := range file.Enums {
		ds = append(ds, protocol.DocumentSymbol{Name: en.Name, Kind: protocol.SymbolKindEnum, Range: mkRange(en.StartLine), SelectionRange: mkRange(en.StartLine)})
	}
	for _, fn := range file.Functions {
		ds = append(ds, protocol.DocumentSymbol{Name: fn.Name, Kind: protocol.SymbolKindFunction, Range: mkRange(fn.StartLine), SelectionRange: mkRange(fn.StartLine)})
	}
	return ds
}

func (a *Analyzer) GetSignatureHelp(uri, source string, pos protocol.Position) *protocol.SignatureHelp {
	line := getLineAt(source, pos.Line)
	if line == "" { return nil }
	prefix := line[:min(pos.Character, len(line))]
	funcName, activeParam := extractCallInfo(prefix)
	if funcName == "" { return nil }
	syms := a.index.LookupByName(funcName)
	if len(syms) == 0 { return nil }
	sym := syms[0]
	sig := protocol.SignatureInformation{Label: sym.Name + formatParamLabel(sym)}
	for _, p := range sym.Params {
		l := ""; if p.Type != "" { l = p.Type + " " }; l += p.Name
		sig.Parameters = append(sig.Parameters, protocol.ParameterInformation{Label: l})
	}
	return &protocol.SignatureHelp{Signatures: []protocol.SignatureInformation{sig}, ActiveParameter: activeParam}
}

func extractCallInfo(prefix string) (string, int) {
	activeParam := 0; depth := 0; parenPos := -1
	for i := len(prefix) - 1; i >= 0; i-- {
		switch prefix[i] {
		case ')': depth++
		case '(': if depth == 0 { parenPos = i; goto found }; depth--
		case ',': if depth == 0 { activeParam++ }
		}
	}
	return "", 0
found:
	if parenPos <= 0 { return "", 0 }
	end := parenPos; start := end - 1
	for start >= 0 && isWordChar(prefix[start]) { start-- }
	start++
	if start >= end { return "", 0 }
	return prefix[start:end], activeParam
}

func formatParamLabel(sym *symbols.Symbol) string {
	var ps []string
	for _, p := range sym.Params {
		s := ""; if p.Type != "" { s = p.Type + " " }; s += p.Name; ps = append(ps, s)
	}
	ret := sym.ReturnType; if ret == "" { ret = "mixed" }
	return "(" + strings.Join(ps, ", ") + "): " + ret
}

func mkRange(line int) protocol.Range {
	return protocol.Range{Start: protocol.Position{Line: line}, End: protocol.Position{Line: line}}
}
func getLineAt(source string, line int) string {
	lines := strings.Split(source, "\n")
	if line >= 0 && line < len(lines) { return lines[line] }; return ""
}
func getWordAt(source string, pos protocol.Position) string {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) { return "" }
	line := lines[pos.Line]
	if pos.Character > len(line) { return "" }
	start := pos.Character
	for start > 0 && isWordChar(line[start-1]) { start-- }
	if start > 0 && line[start-1] == '$' { start-- }
	end := pos.Character
	for end < len(line) && isWordChar(line[end]) { end++ }
	if start >= end { return "" }; return line[start:end]
}
func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '\\'
}
func min(a, b int) int { if a < b { return a }; return b }
