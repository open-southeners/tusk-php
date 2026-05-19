package inlayhint

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/config"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// isScalarType returns true for PHP built-in scalar type names that we do not
// annotate with variable-type hints (they are already obvious from the literal).
func isScalarType(name string) bool {
	switch name {
	case "int", "string", "bool", "float":
		return true
	}
	return false
}

// shortenFQNs rewrites every backslash-qualified name in s to its final
// segment, so "Illuminate\Support\Collection<int, App\Models\User>" renders as
// "Collection<int, User>". Tokens without a backslash — scalars, built-ins,
// generic placeholders — are left untouched, and a leading backslash on a
// fully-qualified name is dropped.
func shortenFQNs(s string) string {
	var sb strings.Builder
	runStart := -1
	flush := func(end int) {
		if runStart < 0 {
			return
		}
		run := s[runStart:end]
		if idx := strings.LastIndexByte(run, '\\'); idx >= 0 {
			run = run[idx+1:]
		}
		sb.WriteString(run)
		runStart = -1
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isRun := c == '\\' || c == '_' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if isRun {
			if runStart < 0 {
				runStart = i
			}
			continue
		}
		flush(i)
		sb.WriteByte(c)
	}
	flush(len(s))
	return sb.String()
}

// skipNonSig skips whitespace and comment tokens forward, returning the index
// of the next meaningful token.
func skipNonSig(tokens []parser.Token, i int) int {
	for i < len(tokens) {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i++
		default:
			return i
		}
	}
	return i
}

// collectVarTypeHints emits ": Type" hints after variable names in simple
// assignments of the form  $var = <expr>  that appear outside if/while/for
// condition parens where the LHS is a plain variable (not a destructure).
func (p *Provider) collectVarTypeHints(
	result *parser.ParseResult,
	lines []string,
	file *parser.FileNode,
	source string,
) []protocol.InlayHint {
	tokens := result.Tokens
	var hints []protocol.InlayHint

	// parenDepth tracks whether we are inside (...) that opened after the most
	// recent statement boundary (semicolon or open-brace).  When parenDepth > 0
	// the assignment is inside a condition / call argument and we skip it for
	// if/while/for.  However we still want `if ($x = getValue())` style (PHP
	// allows assignment-in-condition), so we allow it — the plan says to include
	// those.  We only skip plain `while ($i < $n)` style where no `=` appears.
	// In practice the simplest safe guard: only skip if we are inside a paren AND
	// the `=` is the first non-whitespace operator after `(`.  Since the plan's
	// updated edge-cases text says assignment-in-conditions should be included,
	// we emit hints even inside parens.

	for i := 0; i < len(tokens)-2; i++ {
		tok := tokens[i]
		if tok.Kind != parser.TokenVariable {
			continue
		}

		// Next non-whitespace must be `=` (not `==` or `=>`)
		j := skipNonSig(tokens, i+1)
		if j >= len(tokens) {
			continue
		}
		if tokens[j].Kind != parser.TokenEquals {
			continue
		}
		// Ensure the token after `=` is not another `=` or `>`
		if j+1 < len(tokens) {
			next := tokens[j+1]
			if next.Kind == parser.TokenEquals || next.Kind == parser.TokenDoubleArrow {
				continue
			}
		}

		// Skip list()/destructuring: check if the token before $var is `[` or
		// the `=` is actually part of a list assignment.  A simpler check: if the
		// token immediately before the variable (skipping whitespace backwards) is
		// `[` or `,` inside `[`, we are in a destructure — skip.
		if i > 0 {
			prev := skipBackNonSig(tokens, i-1)
			if prev >= 0 {
				kind := tokens[prev].Kind
				if kind == parser.TokenOpenBracket || kind == parser.TokenComma {
					continue
				}
			}
		}

		varName := tok.Value
		// Use the line after the assignment as the lookup position so the resolver
		// can see the post-assignment state.
		assignLine := tok.Line
		lookupPos := protocol.Position{Line: assignLine + 1, Character: 0}

		rt := p.resolver.ResolveVariableTypeTyped(varName, lines, lookupPos, file)
		if rt.IsEmpty() {
			continue
		}
		typStr := shortenFQNs(rt.String())
		if typStr == "" || typStr == "mixed" {
			continue
		}
		if isScalarType(typStr) {
			continue
		}

		// Hint goes at end of the variable token (after the last character).
		endChar := tok.Column + len(tok.Value)
		hints = append(hints, protocol.InlayHint{
			Position:    protocol.Position{Line: tok.Line, Character: endChar},
			Label:       ": " + typStr,
			Kind:        protocol.InlayHintKindType,
			PaddingLeft: true,
		})
	}
	return hints
}

// skipBackNonSig scans backwards from i skipping whitespace/comments and
// returns the index of the first non-skippable token, or -1 if none.
func skipBackNonSig(tokens []parser.Token, i int) int {
	for i >= 0 {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i--
		default:
			return i
		}
	}
	return -1
}

// collectForeachHints emits ": Type" hints after key/value variable tokens in
// foreach loops.
func (p *Provider) collectForeachHints(
	result *parser.ParseResult,
	lines []string,
	file *parser.FileNode,
	source string,
) []protocol.InlayHint {
	tokens := result.Tokens
	var hints []protocol.InlayHint

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		// foreach keyword is tokenized as TokenIdentifier with value "foreach"
		if tok.Kind != parser.TokenIdentifier || tok.Value != "foreach" {
			continue
		}

		// Scan forward to find the opening paren of the foreach expression.
		j := skipNonSig(tokens, i+1)
		if j >= len(tokens) || tokens[j].Kind != parser.TokenOpenParen {
			continue
		}
		j++ // consume `(`

		// Scan through the expression until we find the `as` keyword, tracking
		// paren depth so we skip nested parens inside the iterable expression.
		depth := 1
		asIdx := -1
		for k := j; k < len(tokens); k++ {
			switch tokens[k].Kind {
			case parser.TokenOpenParen:
				depth++
			case parser.TokenCloseParen:
				depth--
				if depth == 0 {
					goto doneForEachScan
				}
			case parser.TokenIdentifier:
				if depth == 1 && tokens[k].Value == "as" {
					asIdx = k
					goto foundAs
				}
			}
		}
	doneForEachScan:
		continue
	foundAs:
		// After `as`: optional key variable + `=>` + value variable, or just
		// value variable.
		k := asIdx + 1
		k = skipNonSig(tokens, k)
		if k >= len(tokens) {
			continue
		}

		// Find the closing `)` of the foreach header to determine the lookup line.
		closeParen := -1
		depth2 := 1
		for m := j; m < len(tokens); m++ {
			switch tokens[m].Kind {
			case parser.TokenOpenParen:
				depth2++
			case parser.TokenCloseParen:
				depth2--
				if depth2 == 0 {
					closeParen = m
				}
			}
			if closeParen >= 0 {
				break
			}
		}

		var bodyOpenLine int
		if closeParen >= 0 {
			// Look for the `{` after the closing paren.
			n := skipNonSig(tokens, closeParen+1)
			if n < len(tokens) && tokens[n].Kind == parser.TokenOpenBrace {
				bodyOpenLine = tokens[n].Line
			} else if closeParen >= 0 {
				bodyOpenLine = tokens[closeParen].Line
			}
		}
		lookupPos := protocol.Position{Line: bodyOpenLine + 1, Character: 0}

		var keyVarIdx, valVarIdx int = -1, -1

		firstVar := k
		if firstVar < len(tokens) && tokens[firstVar].Kind == parser.TokenVariable {
			// Check if next non-whitespace is `=>`
			n := skipNonSig(tokens, firstVar+1)
			if n < len(tokens) && tokens[n].Kind == parser.TokenDoubleArrow {
				keyVarIdx = firstVar
				n = skipNonSig(tokens, n+1)
				if n < len(tokens) && tokens[n].Kind == parser.TokenVariable {
					valVarIdx = n
				}
			} else {
				valVarIdx = firstVar
			}
		}

		if keyVarIdx >= 0 {
			kTok := tokens[keyVarIdx]
			rt := p.resolver.ResolveVariableTypeTyped(kTok.Value, lines, lookupPos, file)
			if !rt.IsEmpty() && rt.String() != "" && rt.String() != "mixed" {
				endChar := kTok.Column + len(kTok.Value)
				hints = append(hints, protocol.InlayHint{
					Position:    protocol.Position{Line: kTok.Line, Character: endChar},
					Label:       ": " + shortenFQNs(rt.String()),
					Kind:        protocol.InlayHintKindType,
					PaddingLeft: true,
				})
			}
		}
		if valVarIdx >= 0 {
			vTok := tokens[valVarIdx]
			rt := p.resolver.ResolveVariableTypeTyped(vTok.Value, lines, lookupPos, file)
			if !rt.IsEmpty() && rt.String() != "" && rt.String() != "mixed" {
				endChar := vTok.Column + len(vTok.Value)
				hints = append(hints, protocol.InlayHint{
					Position:    protocol.Position{Line: vTok.Line, Character: endChar},
					Label:       ": " + shortenFQNs(rt.String()),
					Kind:        protocol.InlayHintKindType,
					PaddingLeft: true,
				})
			}
		}
	}
	return hints
}

// collectClosureReturnHints emits ": Type" hints for arrow functions (fn) and
// anonymous functions that lack an explicit return type annotation.
func (p *Provider) collectClosureReturnHints(
	result *parser.ParseResult,
	lines []string,
	file *parser.FileNode,
	source string,
) []protocol.InlayHint {
	tokens := result.Tokens
	var hints []protocol.InlayHint

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind != parser.TokenFunction {
			continue
		}

		// Determine if this is an arrow function (fn) or an anonymous function
		// (function keyword followed immediately by `(`).
		isArrow := tok.Value == "fn"
		if !isArrow {
			// Regular `function`: anonymous if next non-whitespace is `(`
			j := skipNonSig(tokens, i+1)
			if j >= len(tokens) || tokens[j].Kind != parser.TokenOpenParen {
				// Named function — not a closure
				continue
			}
		}

		// Find the closing `)` of the parameter list.
		j := skipNonSig(tokens, i+1)
		if j >= len(tokens) || tokens[j].Kind != parser.TokenOpenParen {
			continue
		}
		closeParenIdx := findMatchingClose(tokens, j, parser.TokenOpenParen, parser.TokenCloseParen)
		if closeParenIdx < 0 {
			continue
		}

		// Check if there is already a `: Type` between `)` and the `=>` / `{`.
		k := skipNonSig(tokens, closeParenIdx+1)
		if k >= len(tokens) {
			continue
		}
		if tokens[k].Kind == parser.TokenColon {
			// Explicit return type present — no hint needed.
			continue
		}

		closeTok := tokens[closeParenIdx]
		hintPos := protocol.Position{Line: closeTok.Line, Character: closeTok.Column + len(closeTok.Value)}

		if isArrow {
			// Arrow function: `fn(...) => expr`
			// k should be `=>` at this point (already confirmed no `:`)
			if k >= len(tokens) || tokens[k].Kind != parser.TokenDoubleArrow {
				continue
			}
			// Resolve the type of the first expression token after `=>`
			exprIdx := skipNonSig(tokens, k+1)
			if exprIdx >= len(tokens) {
				continue
			}
			exprTok := tokens[exprIdx]
			lookupPos := protocol.Position{Line: exprTok.Line, Character: exprTok.Column}
			rt := resolveExprToken(p, tokens, exprIdx, lines, lookupPos, file, source)
			if rt.IsEmpty() || rt.String() == "" || rt.String() == "mixed" {
				continue
			}
			hints = append(hints, protocol.InlayHint{
				Position:    hintPos,
				Label:       ": " + shortenFQNs(rt.String()),
				Kind:        protocol.InlayHintKindType,
				PaddingLeft: true,
			})
		} else {
			// Anonymous function: scan body for return statements.
			if k >= len(tokens) || tokens[k].Kind != parser.TokenOpenBrace {
				continue
			}
			// Find `return <expr>` tokens inside the body at depth 1.
			bodyStart := k
			bodyEnd := findMatchingClose(tokens, bodyStart, parser.TokenOpenBrace, parser.TokenCloseBrace)
			if bodyEnd < 0 {
				continue
			}
			rt := p.resolveFirstReturn(tokens, bodyStart+1, bodyEnd, lines, file, source)
			if rt.IsEmpty() || rt.String() == "" || rt.String() == "mixed" {
				continue
			}
			hints = append(hints, protocol.InlayHint{
				Position:    hintPos,
				Label:       ": " + shortenFQNs(rt.String()),
				Kind:        protocol.InlayHintKindType,
				PaddingLeft: true,
			})
		}
	}
	return hints
}

// resolveExprToken resolves the return type represented by the token at index
// exprIdx.  Handles variable references and new-expressions.
func resolveExprToken(
	p *Provider,
	tokens []parser.Token,
	exprIdx int,
	lines []string,
	pos protocol.Position,
	file *parser.FileNode,
	source string,
) resolve.ResolvedType {
	if exprIdx >= len(tokens) {
		return resolve.ResolvedType{}
	}
	tok := tokens[exprIdx]
	switch tok.Kind {
	case parser.TokenVariable:
		return p.resolver.ResolveVariableTypeTyped(tok.Value, lines, pos, file)
	case parser.TokenNew:
		// new ClassName(...)
		j := skipNonSig(tokens, exprIdx+1)
		if j >= len(tokens) {
			return resolve.ResolvedType{}
		}
		className := collectTypeName(tokens, j)
		if className == "" {
			return resolve.ResolvedType{}
		}
		fqn := p.resolver.ResolveClassName(className, file)
		if fqn == "" {
			fqn = className
		}
		return resolve.ResolvedType{FQN: fqn}
	}
	// For chain expressions, use the TypedChainResolver if available.
	if p.resolver.TypedChainResolver != nil {
		// Build a small expression string from the token's line up to and
		// including the token.
		if tok.Line < len(lines) {
			lineStr := lines[tok.Line]
			end := tok.Column + len(tok.Value)
			if end > len(lineStr) {
				end = len(lineStr)
			}
			expr := strings.TrimSpace(lineStr[:end])
			if expr != "" {
				return p.resolver.TypedChainResolver(expr, source, pos, file)
			}
		}
	}
	return resolve.ResolvedType{}
}

// resolveFirstReturn scans tokens between start and end (exclusive) for the
// first return statement and resolves the returned expression type.
func (p *Provider) resolveFirstReturn(
	tokens []parser.Token,
	start, end int,
	lines []string,
	file *parser.FileNode,
	source string,
) resolve.ResolvedType {
	depth := 0
	for i := start; i < end && i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenBrace, parser.TokenOpenParen:
			depth++
		case parser.TokenCloseBrace, parser.TokenCloseParen:
			depth--
		case parser.TokenReturn:
			if depth != 0 {
				continue
			}
			// The expression immediately follows `return`
			j := skipNonSig(tokens, i+1)
			if j >= end || j >= len(tokens) {
				continue
			}
			retTok := tokens[j]
			lookupPos := protocol.Position{Line: retTok.Line, Character: retTok.Column}
			rt := resolveExprToken(p, tokens, j, lines, lookupPos, file, source)
			if !rt.IsEmpty() {
				return rt
			}
		}
	}
	return resolve.ResolvedType{}
}

// findMatchingClose finds the index of the closing token that matches the
// opening token at index openIdx.  Returns -1 if not found.
func findMatchingClose(tokens []parser.Token, openIdx int, openKind, closeKind parser.TokenKind) int {
	if openIdx >= len(tokens) || tokens[openIdx].Kind != openKind {
		return -1
	}
	depth := 1
	for i := openIdx + 1; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case openKind:
			depth++
		case closeKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// collectTypeName reads a potentially backslash-qualified identifier starting
// at index i and returns it as a string.
func collectTypeName(tokens []parser.Token, i int) string {
	var parts []string
	for i < len(tokens) {
		tok := tokens[i]
		switch tok.Kind {
		case parser.TokenIdentifier, parser.TokenBackslash:
			parts = append(parts, tok.Value)
			i++
		default:
			goto done
		}
	}
done:
	return strings.Join(parts, "")
}

// collectMethodReturnHints emits ": Type" hints at the closing `)` of methods
// that lack an explicit return type but carry a @return docblock annotation.
func (p *Provider) collectMethodReturnHints(result *parser.ParseResult, file *parser.FileNode) []protocol.InlayHint {
	if file == nil || result == nil {
		return nil
	}
	var hints []protocol.InlayHint
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			// Skip if an explicit return type is already declared.
			if m.ReturnType.Name != "" {
				continue
			}
			if m.DocComment == "" {
				continue
			}
			doc := parser.ParseDocBlock(m.DocComment)
			if doc == nil || doc.Return.Type == "" {
				continue
			}
			retType := shortenFQNs(doc.Return.Type)
			if retType == "" || retType == "void" || retType == "mixed" {
				continue
			}

			// Place the hint immediately after the closing `)` of the
			// parameter list — the natural slot for a `: Type` annotation.
			closeTok, ok := methodSignatureCloseParen(result.Tokens, m.StartLine)
			if !ok {
				continue
			}
			hints = append(hints, protocol.InlayHint{
				Position:    protocol.Position{Line: closeTok.Line, Character: closeTok.Column + 1},
				Label:       ": " + retType,
				Kind:        protocol.InlayHintKindType,
				PaddingLeft: true,
			})
		}
	}
	return hints
}

// methodSignatureCloseParen finds the `)` token that closes the parameter list
// of the method whose `function` keyword appears on startLine. MethodNode's
// StartLine is taken from the `function` keyword's line, so the two align.
func methodSignatureCloseParen(tokens []parser.Token, startLine int) (parser.Token, bool) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != parser.TokenFunction || tokens[i].Line != startLine {
			continue
		}
		for j := i + 1; j < len(tokens); j++ {
			if tokens[j].Kind != parser.TokenOpenParen {
				continue
			}
			if closeIdx := findMatchingClose(tokens, j, parser.TokenOpenParen, parser.TokenCloseParen); closeIdx >= 0 {
				return tokens[closeIdx], true
			}
			return parser.Token{}, false
		}
		return parser.Token{}, false
	}
	return parser.Token{}, false
}

// collectParamNameHints emits "name:" hints before positional call arguments.
func (p *Provider) collectParamNameHints(
	result *parser.ParseResult,
	lines []string,
	file *parser.FileNode,
	source string,
	cfg *config.InlayHintsConfig,
) []protocol.InlayHint {
	tokens := result.Tokens
	var hints []protocol.InlayHint

	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]

		// We are looking for patterns that end with `ident (` or `ident :: ident (`
		// or `$var -> ident (`.
		if tok.Kind != parser.TokenOpenParen {
			continue
		}

		// Scan backwards (skipping whitespace) to find the callee identifier.
		callee, calleeKind, receiverExpr := extractCallee(tokens, i)
		if callee == "" {
			continue
		}

		// Resolve the callee's parameter list from the symbol index.
		params := p.resolveCalleeParams(callee, calleeKind, receiverExpr, tokens, i, lines, file, source)
		if len(params) == 0 {
			continue
		}

		// SuppressSingleParam: no hints when there is exactly one parameter.
		if cfg.SuppressSingleParam && len(params) == 1 {
			continue
		}

		// Extract argument token spans.
		closeIdx := findMatchingClose(tokens, i, parser.TokenOpenParen, parser.TokenCloseParen)
		if closeIdx < 0 {
			continue
		}
		args := splitCallArgs(tokens, i+1, closeIdx)

		for argIdx, arg := range args {
			if argIdx >= len(params) {
				break
			}
			param := params[argIdx]
			if param.IsVariadic {
				break
			}
			if len(arg) == 0 {
				continue
			}

			// Skip named arguments: first two tokens are  ident  `:`  (and not `::`)
			if isNamedArg(arg) {
				continue
			}

			// SuppressNameMatch: skip if the argument is a $variable whose name
			// (without $) matches the parameter name (without $).
			if cfg.SuppressNameMatch {
				firstTok := firstMeaningfulToken(arg)
				if firstTok != nil && firstTok.Kind == parser.TokenVariable {
					argVarName := strings.TrimPrefix(firstTok.Value, "$")
					paramName := strings.TrimPrefix(param.Name, "$")
					if argVarName == paramName {
						continue
					}
				}
			}

			paramLabel := strings.TrimPrefix(param.Name, "$")
			if paramLabel == "" {
				continue
			}

			firstTok := firstMeaningfulToken(arg)
			if firstTok == nil {
				continue
			}
			hints = append(hints, protocol.InlayHint{
				Position:     protocol.Position{Line: firstTok.Line, Character: firstTok.Column},
				Label:        paramLabel + ":",
				Kind:         protocol.InlayHintKindParameter,
				PaddingRight: true,
			})
		}
	}
	return hints
}

// calleeKindT distinguishes how the callee was identified.
type calleeKindT int

const (
	calleeFunction calleeKindT = iota // plain function call: name(
	calleeMethod                      // instance method: $expr->name(
	calleeStatic                      // static method: Class::name(
)

// extractCallee walks backwards from the `(` at index openParen and extracts
// the callee name, kind, and (for method calls) the receiver expression string.
func extractCallee(tokens []parser.Token, openParen int) (callee string, kind calleeKindT, receiver string) {
	i := openParen - 1
	i = skipBackNonSig(tokens, i)
	if i < 0 {
		return "", calleeFunction, ""
	}

	// The token before `(` should be the method/function name.
	nameTok := tokens[i]
	if nameTok.Kind != parser.TokenIdentifier {
		return "", calleeFunction, ""
	}
	callee = nameTok.Value
	i--
	i = skipBackNonSig(tokens, i)
	if i < 0 {
		return callee, calleeFunction, ""
	}

	switch tokens[i].Kind {
	case parser.TokenArrow: // ->
		// Instance method call: collect the receiver expression to the left.
		receiver = collectReceiverExpr(tokens, i-1)
		return callee, calleeMethod, receiver
	case parser.TokenDoubleColon: // ::
		// Static method call: the token to the left is the class name.
		j := skipBackNonSig(tokens, i-1)
		if j < 0 {
			return callee, calleeStatic, ""
		}
		cls := collectTypeName(tokens, j)
		if cls == "" && tokens[j].Kind == parser.TokenIdentifier {
			cls = tokens[j].Value
		}
		return callee, calleeStatic, cls
	default:
		return callee, calleeFunction, ""
	}
}

// collectReceiverExpr builds a minimal expression string for the receiver of a
// method call by scanning backwards from the token before `->`.  Returns
// something like `$foo`, `$foo->bar()`, or a class name.
func collectReceiverExpr(tokens []parser.Token, fromIdx int) string {
	i := skipBackNonSig(tokens, fromIdx)
	if i < 0 {
		return ""
	}
	tok := tokens[i]

	// Simple variable: $foo
	if tok.Kind == parser.TokenVariable {
		return tok.Value
	}

	// Closing paren: a method chain like $foo->bar()
	if tok.Kind == parser.TokenCloseParen {
		// Find the matching open paren.
		openIdx := findMatchingCloseBack(tokens, i)
		if openIdx < 0 {
			return ""
		}
		// Grab the method name before `(`.
		j := skipBackNonSig(tokens, openIdx-1)
		if j < 0 {
			return ""
		}
		methodName := ""
		if tokens[j].Kind == parser.TokenIdentifier {
			methodName = tokens[j].Value
			j--
			j = skipBackNonSig(tokens, j)
		}
		if j < 0 {
			return ""
		}
		// Now j should be at `->` or `::`
		if tokens[j].Kind == parser.TokenArrow || tokens[j].Kind == parser.TokenDoubleColon {
			ownerExpr := collectReceiverExpr(tokens, j-1)
			op := "->"
			if tokens[j].Kind == parser.TokenDoubleColon {
				op = "::"
			}
			return ownerExpr + op + methodName + "()"
		}
		return methodName + "()"
	}

	// Identifier (class name for static calls, or $this)
	if tok.Kind == parser.TokenIdentifier {
		return tok.Value
	}

	return ""
}

// findMatchingCloseBack finds the open token that matches a closing paren/brace
// at closeIdx by scanning backwards.
func findMatchingCloseBack(tokens []parser.Token, closeIdx int) int {
	if closeIdx < 0 || closeIdx >= len(tokens) {
		return -1
	}
	closeTok := tokens[closeIdx]
	var openKind, closeKind parser.TokenKind
	switch closeTok.Kind {
	case parser.TokenCloseParen:
		openKind = parser.TokenOpenParen
		closeKind = parser.TokenCloseParen
	case parser.TokenCloseBrace:
		openKind = parser.TokenOpenBrace
		closeKind = parser.TokenCloseBrace
	default:
		return -1
	}
	depth := 1
	for i := closeIdx - 1; i >= 0; i-- {
		switch tokens[i].Kind {
		case closeKind:
			depth++
		case openKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// resolveCalleeParams looks up the parameter list for a call-site callee.
func (p *Provider) resolveCalleeParams(
	callee string,
	kind calleeKindT,
	receiverOrClass string,
	tokens []parser.Token,
	openParen int,
	lines []string,
	file *parser.FileNode,
	source string,
) []paramEntry {
	switch kind {
	case calleeMethod:
		if p.resolver.TypedChainResolver == nil {
			return nil
		}
		// Find the position of the call-site so the resolver has context.
		callPos := protocol.Position{Line: tokens[openParen].Line, Character: tokens[openParen].Column}
		rt := p.resolver.TypedChainResolver(receiverOrClass, source, callPos, file)
		if rt.IsEmpty() {
			return nil
		}
		fqn := rt.BaseFQN()
		return p.membersToParams(fqn, callee)
	case calleeStatic:
		cls := p.resolver.ResolveClassName(receiverOrClass, file)
		if cls == "" {
			cls = receiverOrClass
		}
		return p.membersToParams(cls, callee)
	default: // calleeFunction
		// Look up as a global function in the index.
		syms := p.index.LookupByName(callee)
		for _, s := range syms {
			if s.ParentFQN == "" { // global function, not a method
				return symbolParamsToEntries(s.Params)
			}
		}
		return nil
	}

}

// paramEntry is a lightweight local copy of a parameter definition.
type paramEntry struct {
	Name       string
	IsVariadic bool
}

// membersToParams looks up the named member on classFQN and returns its params.
func (p *Provider) membersToParams(classFQN, methodName string) []paramEntry {
	for _, sym := range p.index.GetClassMembers(classFQN) {
		if sym.Name == methodName {
			return symbolParamsToEntries(sym.Params)
		}
	}
	return nil
}

// symbolParamsToEntries converts symbols.ParamInfo slice to paramEntry slice.
func symbolParamsToEntries(params []symbols.ParamInfo) []paramEntry {
	entries := make([]paramEntry, len(params))
	for i, p := range params {
		entries[i] = paramEntry{Name: p.Name, IsVariadic: p.IsVariadic}
	}
	return entries
}

// splitCallArgs splits the token slice between start (inclusive) and end
// (exclusive) on top-level commas, returning one token slice per argument.
func splitCallArgs(tokens []parser.Token, start, end int) [][]parser.Token {
	var args [][]parser.Token
	depth := 0
	argStart := start
	for i := start; i < end && i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				args = append(args, tokens[argStart:i])
				argStart = i + 1
			}
		}
	}
	// Last (or only) argument
	if argStart < end {
		args = append(args, tokens[argStart:end])
	}
	return args
}

// isNamedArg returns true if the argument token sequence starts with  ident `:`
// (but not `::`), indicating a PHP 8.0+ named argument.
func isNamedArg(arg []parser.Token) bool {
	first := firstMeaningfulToken(arg)
	if first == nil || first.Kind != parser.TokenIdentifier {
		return false
	}
	// Find the second meaningful token.
	var second *parser.Token
	found := false
	for i := range arg {
		if &arg[i] == first {
			found = true
			continue
		}
		if !found {
			continue
		}
		switch arg[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			continue
		}
		second = &arg[i]
		break
	}
	if second == nil {
		return false
	}
	// Named arg: second token is `:` (not `::`)
	return second.Kind == parser.TokenColon
}

// firstMeaningfulToken returns a pointer to the first non-whitespace,
// non-comment token in the slice, or nil.
func firstMeaningfulToken(tokens []parser.Token) *parser.Token {
	for i := range tokens {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			continue
		}
		return &tokens[i]
	}
	return nil
}
