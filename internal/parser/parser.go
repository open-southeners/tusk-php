package parser

import (
	"strings"
	"unicode"
)

// PHPParser parses PHP source into a lightweight AST for LSP features.
type PHPParser struct{}

func New() *PHPParser {
	return &PHPParser{}
}

// ParseResult holds the structured output of parsing a PHP file.
type ParseResult struct {
	Namespace  string
	Uses       []UseStatement
	Classes    []ClassDef
	Interfaces []InterfaceDef
	Traits     []TraitDef
	Enums      []EnumDef
	Functions  []FunctionDef
	Constants  []ConstantDef
	Variables  []VariableDef
	Tokens     []Token
	Errors     []ParseError
	Lines      []string
}

type UseStatement struct {
	FullName string
	Alias    string
	Kind     string // "class", "function", "const"
	Line     int
}

type ClassDef struct {
	Name       string
	FullName   string
	Extends    string
	Implements []string
	Traits     []string
	Properties []PropertyDef
	Methods    []MethodDef
	Constants  []ConstantDef
	Attributes []AttributeDef
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	Line       int
	EndLine    int
	DocComment string
}

type InterfaceDef struct {
	Name     string
	FullName string
	Extends  []string
	Methods  []MethodDef
	Line     int
	EndLine  int
}

type TraitDef struct {
	Name       string
	FullName   string
	Properties []PropertyDef
	Methods    []MethodDef
	Line       int
	EndLine    int
}

type EnumDef struct {
	Name       string
	FullName   string
	BackedType string
	Cases      []EnumCase
	Methods    []MethodDef
	Implements []string
	Line       int
	EndLine    int
}

type EnumCase struct {
	Name  string
	Value string
	Line  int
}

type FunctionDef struct {
	Name       string
	FullName   string
	Params     []ParamDef
	ReturnType string
	IsVariadic bool
	Line       int
	EndLine    int
	DocComment string
}

type MethodDef struct {
	Name       string
	Params     []ParamDef
	ReturnType string
	Visibility string
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	Line       int
	EndLine    int
	DocComment string
}

type PropertyDef struct {
	Name       string
	Type       string
	Visibility string
	IsStatic   bool
	IsReadonly bool
	HasDefault bool
	Line       int
	DocComment string
}

type ParamDef struct {
	Name       string
	Type       string
	HasDefault bool
	IsVariadic bool
	IsPromoted bool
	Visibility string
	IsReadonly bool
}

type ConstantDef struct {
	Name  string
	Value string
	Type  string
	Line  int
}

type VariableDef struct {
	Name string
	Type string
	Line int
}

type AttributeDef struct {
	Name string
	Args []string
	Line int
}

type Token struct {
	Kind   TokenKind
	Value  string
	Line   int
	Column int
	Offset int
}

type TokenKind int

const (
	TokenPHP TokenKind = iota
	TokenNamespace
	TokenUse
	TokenClass
	TokenInterface
	TokenTrait
	TokenEnum
	TokenFunction
	TokenConst
	TokenVar
	TokenPublic
	TokenProtected
	TokenPrivate
	TokenStatic
	TokenAbstract
	TokenFinal
	TokenReadonly
	TokenExtends
	TokenImplements
	TokenReturn
	TokenNew
	TokenArrow
	TokenDoubleArrow
	TokenDoubleColon
	TokenBackslash
	TokenString
	TokenNumber
	TokenVariable
	TokenIdentifier
	TokenOpenBrace
	TokenCloseBrace
	TokenOpenParen
	TokenCloseParen
	TokenOpenBracket
	TokenCloseBracket
	TokenSemicolon
	TokenComma
	TokenEquals
	TokenColon
	TokenQuestion
	TokenPipe
	TokenAmpersand
	TokenAt
	TokenHash
	TokenDocComment
	TokenComment
	TokenWhitespace
	TokenStringLiteral
	TokenEOF
	TokenUnknown
)

type ParseError struct {
	Message string
	Line    int
	Column  int
}

// Parse tokenizes and performs a structural parse of PHP source code.
func (p *PHPParser) Parse(source string) *ParseResult {
	result := &ParseResult{
		Lines: strings.Split(source, "\n"),
	}
	tokens := tokenize(source)
	result.Tokens = tokens
	parseStructure(tokens, result)
	return result
}

// GetTokenAtPosition returns the token at the given line and character.
func (pr *ParseResult) GetTokenAtPosition(line, character int) *Token {
	for i := range pr.Tokens {
		t := &pr.Tokens[i]
		if t.Line == line {
			end := t.Column + len(t.Value)
			if character >= t.Column && character <= end {
				return t
			}
		}
	}
	return nil
}

// GetWordAtPosition extracts the word/identifier at a given position.
func (pr *ParseResult) GetWordAtPosition(line, character int) string {
	if line >= len(pr.Lines) {
		return ""
	}
	ln := pr.Lines[line]
	if character >= len(ln) {
		return ""
	}
	start := character
	for start > 0 && isIdentChar(rune(ln[start-1])) {
		start--
	}
	if start > 0 && ln[start-1] == '$' {
		start--
	}
	end := character
	for end < len(ln) && isIdentChar(rune(ln[end])) {
		end++
	}
	return ln[start:end]
}

// GetContextAtPosition returns contextual info about -> or :: access.
func (pr *ParseResult) GetContextAtPosition(line, character int) (contextType string, receiver string) {
	if line >= len(pr.Lines) {
		return "", ""
	}
	ln := pr.Lines[line]
	if character < 2 {
		return "", ""
	}

	pos := character - 1
	for pos >= 0 && pos < len(ln) && ln[pos] == ' ' {
		pos--
	}

	if pos >= 1 && pos < len(ln) && ln[pos-1] == '-' && ln[pos] == '>' {
		rEnd := pos - 1
		rStart := rEnd - 1
		for rStart >= 0 && (isIdentChar(rune(ln[rStart])) || ln[rStart] == '$') {
			rStart--
		}
		rStart++
		if rStart <= rEnd {
			return "member", ln[rStart : rEnd+1]
		}
	}

	if pos >= 1 && pos < len(ln) && ln[pos-1] == ':' && ln[pos] == ':' {
		rEnd := pos - 1
		rStart := rEnd - 1
		for rStart >= 0 && (isIdentChar(rune(ln[rStart])) || ln[rStart] == '\\') {
			rStart--
		}
		rStart++
		if rStart <= rEnd {
			return "static", ln[rStart : rEnd+1]
		}
	}

	if pos >= 0 && pos < len(ln) && ln[pos] == '\\' {
		rStart := pos - 1
		for rStart >= 0 && (isIdentChar(rune(ln[rStart])) || ln[rStart] == '\\') {
			rStart--
		}
		rStart++
		return "namespace", ln[rStart : pos+1]
	}

	return "", ""
}

func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// --- Tokenizer ---

func tokenize(source string) []Token {
	tokens := make([]Token, 0, len(source)/4)
	line, col, offset := 0, 0, 0

	for offset < len(source) {
		ch := source[offset]

		// Whitespace
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			for offset < len(source) && (source[offset] == ' ' || source[offset] == '\t' || source[offset] == '\r' || source[offset] == '\n') {
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			continue
		}

		// Doc comment /** ... */
		if offset+2 < len(source) && source[offset:offset+3] == "/**" {
			start := offset
			startLine, startCol := line, col
			offset += 3
			col += 3
			for offset+1 < len(source) {
				if source[offset] == '*' && source[offset+1] == '/' {
					offset += 2
					col += 2
					break
				}
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			tokens = append(tokens, Token{TokenDocComment, source[start:offset], startLine, startCol, start})
			continue
		}

		// Block comment
		if offset+1 < len(source) && source[offset:offset+2] == "/*" {
			start := offset
			offset += 2
			col += 2
			for offset+1 < len(source) {
				if source[offset] == '*' && source[offset+1] == '/' {
					offset += 2
					col += 2
					break
				}
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			tokens = append(tokens, Token{TokenComment, source[start:offset], line, col, start})
			continue
		}

		// Line comment
		if (offset+1 < len(source) && source[offset:offset+2] == "//") || (ch == '#' && !(offset+1 < len(source) && source[offset+1] == '[')) {
			start := offset
			startCol := col
			for offset < len(source) && source[offset] != '\n' {
				offset++
				col++
			}
			tokens = append(tokens, Token{TokenComment, source[start:offset], line, startCol, start})
			continue
		}

		// Attribute #[
		if ch == '#' && offset+1 < len(source) && source[offset+1] == '[' {
			tokens = append(tokens, Token{TokenHash, "#", line, col, offset})
			offset++
			col++
			continue
		}

		// String literals
		if ch == '"' || ch == '\'' {
			start := offset
			startLine, startCol := line, col
			quote := ch
			offset++
			col++
			for offset < len(source) {
				if source[offset] == '\\' && offset+1 < len(source) {
					offset += 2
					col += 2
					continue
				}
				if source[offset] == byte(quote) {
					offset++
					col++
					break
				}
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			tokens = append(tokens, Token{TokenStringLiteral, source[start:offset], startLine, startCol, start})
			continue
		}

		// Variable $name
		if ch == '$' && offset+1 < len(source) && isIdentChar(rune(source[offset+1])) {
			start := offset
			startCol := col
			offset++
			col++
			for offset < len(source) && isIdentChar(rune(source[offset])) {
				offset++
				col++
			}
			tokens = append(tokens, Token{TokenVariable, source[start:offset], line, startCol, start})
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			start := offset
			startCol := col
			for offset < len(source) && (source[offset] >= '0' && source[offset] <= '9' || source[offset] == '.' || source[offset] == '_') {
				offset++
				col++
			}
			tokens = append(tokens, Token{TokenNumber, source[start:offset], line, startCol, start})
			continue
		}

		// Identifiers and keywords
		if isIdentChar(rune(ch)) {
			start := offset
			startCol := col
			for offset < len(source) && isIdentChar(rune(source[offset])) {
				offset++
				col++
			}
			word := source[start:offset]
			kind := identifierToKind(word)
			tokens = append(tokens, Token{kind, word, line, startCol, start})
			continue
		}

		// Multi-char operators
		if offset+1 < len(source) {
			two := source[offset : offset+2]
			switch two {
			case "->":
				tokens = append(tokens, Token{TokenArrow, "->", line, col, offset})
				offset += 2
				col += 2
				continue
			case "=>":
				tokens = append(tokens, Token{TokenDoubleArrow, "=>", line, col, offset})
				offset += 2
				col += 2
				continue
			case "::":
				tokens = append(tokens, Token{TokenDoubleColon, "::", line, col, offset})
				offset += 2
				col += 2
				continue
			}
		}

		// Single-char tokens
		kind := TokenUnknown
		switch ch {
		case '{':
			kind = TokenOpenBrace
		case '}':
			kind = TokenCloseBrace
		case '(':
			kind = TokenOpenParen
		case ')':
			kind = TokenCloseParen
		case '[':
			kind = TokenOpenBracket
		case ']':
			kind = TokenCloseBracket
		case ';':
			kind = TokenSemicolon
		case ',':
			kind = TokenComma
		case '=':
			kind = TokenEquals
		case ':':
			kind = TokenColon
		case '?':
			kind = TokenQuestion
		case '|':
			kind = TokenPipe
		case '&':
			kind = TokenAmpersand
		case '@':
			kind = TokenAt
		case '\\':
			kind = TokenBackslash
		}
		tokens = append(tokens, Token{kind, string(ch), line, col, offset})
		offset++
		col++
	}

	tokens = append(tokens, Token{TokenEOF, "", line, col, offset})
	return tokens
}

func identifierToKind(word string) TokenKind {
	switch word {
	case "namespace":
		return TokenNamespace
	case "use":
		return TokenUse
	case "class":
		return TokenClass
	case "interface":
		return TokenInterface
	case "trait":
		return TokenTrait
	case "enum":
		return TokenEnum
	case "function", "fn":
		return TokenFunction
	case "const":
		return TokenConst
	case "var":
		return TokenVar
	case "public":
		return TokenPublic
	case "protected":
		return TokenProtected
	case "private":
		return TokenPrivate
	case "static":
		return TokenStatic
	case "abstract":
		return TokenAbstract
	case "final":
		return TokenFinal
	case "readonly":
		return TokenReadonly
	case "extends":
		return TokenExtends
	case "implements":
		return TokenImplements
	case "return":
		return TokenReturn
	case "new":
		return TokenNew
	default:
		return TokenIdentifier
	}
}

// parseStructure extracts top-level structural info from tokens.
func parseStructure(tokens []Token, result *ParseResult) {
	p := &structParser{tokens: tokens, pos: 0, result: result}
	p.parse()
}

type structParser struct {
	tokens []Token
	pos    int
	result *ParseResult
}

func (p *structParser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *structParser) advance() Token {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *structParser) expect(kind TokenKind) (Token, bool) {
	t := p.peek()
	if t.Kind == kind {
		p.advance()
		return t, true
	}
	return t, false
}

func (p *structParser) skipUntil(kinds ...TokenKind) {
	depth := 0
	for p.peek().Kind != TokenEOF {
		t := p.peek()
		if t.Kind == TokenOpenBrace || t.Kind == TokenOpenParen {
			depth++
		} else if t.Kind == TokenCloseBrace || t.Kind == TokenCloseParen {
			if depth == 0 {
				return
			}
			depth--
		}
		for _, k := range kinds {
			if t.Kind == k && depth == 0 {
				return
			}
		}
		p.advance()
	}
}

func (p *structParser) skipBlock() {
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	depth := 1
	for depth > 0 && p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenOpenBrace:
			depth++
		case TokenCloseBrace:
			depth--
		}
		p.advance()
	}
}

func (p *structParser) parse() {
	var lastDocComment string
	for p.peek().Kind != TokenEOF {
		t := p.peek()
		switch t.Kind {
		case TokenDocComment:
			lastDocComment = t.Value
			p.advance()
			continue
		case TokenNamespace:
			p.parseNamespace()
		case TokenUse:
			p.parseUse()
		case TokenClass:
			p.parseClass(lastDocComment)
		case TokenAbstract, TokenFinal, TokenReadonly:
			// Look ahead for class
			p.parseClass(lastDocComment)
		case TokenInterface:
			p.parseInterface()
		case TokenTrait:
			p.parseTrait()
		case TokenEnum:
			p.parseEnum()
		case TokenFunction:
			p.parseFunction(lastDocComment)
		case TokenConst:
			p.parseConst()
		default:
			p.advance()
		}
		lastDocComment = ""
	}
}

func (p *structParser) parseNamespace() {
	p.advance()
	var parts []string
	for {
		t := p.peek()
		if t.Kind == TokenIdentifier {
			parts = append(parts, t.Value)
			p.advance()
		} else if t.Kind == TokenBackslash {
			p.advance()
		} else {
			break
		}
	}
	p.result.Namespace = strings.Join(parts, "\\")
	p.expect(TokenSemicolon)
}

func (p *structParser) parseUse() {
	p.advance()
	kind := "class"
	if p.peek().Kind == TokenFunction {
		kind = "function"
		p.advance()
	} else if p.peek().Kind == TokenConst {
		kind = "const"
		p.advance()
	}

	var parts []string
	line := p.peek().Line
	for {
		t := p.peek()
		if t.Kind == TokenIdentifier {
			parts = append(parts, t.Value)
			p.advance()
		} else if t.Kind == TokenBackslash {
			parts = append(parts, "\\")
			p.advance()
		} else {
			break
		}
	}

	fullName := strings.Join(parts, "")
	alias := ""
	if p.peek().Value == "as" {
		p.advance()
		if t, ok := p.expect(TokenIdentifier); ok {
			alias = t.Value
		}
	}
	if alias == "" {
		segments := strings.Split(fullName, "\\")
		alias = segments[len(segments)-1]
	}
	p.result.Uses = append(p.result.Uses, UseStatement{
		FullName: fullName, Alias: alias, Kind: kind, Line: line,
	})
	p.expect(TokenSemicolon)
}

func (p *structParser) parseClass(docComment string) {
	cls := ClassDef{DocComment: docComment}
	for {
		switch p.peek().Kind {
		case TokenAbstract:
			cls.IsAbstract = true
			p.advance()
		case TokenFinal:
			cls.IsFinal = true
			p.advance()
		case TokenReadonly:
			cls.IsReadonly = true
			p.advance()
		default:
			goto afterMod
		}
	}
afterMod:
	if _, ok := p.expect(TokenClass); !ok {
		return
	}
	if t, ok := p.expect(TokenIdentifier); ok {
		cls.Name = t.Value
		cls.Line = t.Line
		if p.result.Namespace != "" {
			cls.FullName = p.result.Namespace + "\\" + cls.Name
		} else {
			cls.FullName = cls.Name
		}
	}
	if p.peek().Kind == TokenExtends {
		p.advance()
		cls.Extends = p.readTypeName()
	}
	if p.peek().Kind == TokenImplements {
		p.advance()
		for {
			name := p.readTypeName()
			if name == "" {
				break
			}
			cls.Implements = append(cls.Implements, name)
			if p.peek().Kind == TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	cls.Methods, cls.Properties, cls.Constants = p.parseClassBody()
	cls.EndLine = p.peek().Line
	p.result.Classes = append(p.result.Classes, cls)
}

func (p *structParser) parseInterface() {
	p.advance()
	iface := InterfaceDef{}
	if t, ok := p.expect(TokenIdentifier); ok {
		iface.Name = t.Value
		iface.Line = t.Line
		if p.result.Namespace != "" {
			iface.FullName = p.result.Namespace + "\\" + iface.Name
		} else {
			iface.FullName = iface.Name
		}
	}
	if p.peek().Kind == TokenExtends {
		p.advance()
		for {
			name := p.readTypeName()
			if name == "" {
				break
			}
			iface.Extends = append(iface.Extends, name)
			if p.peek().Kind == TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	iface.Methods, _, _ = p.parseClassBody()
	iface.EndLine = p.peek().Line
	p.result.Interfaces = append(p.result.Interfaces, iface)
}

func (p *structParser) parseTrait() {
	p.advance()
	trait := TraitDef{}
	if t, ok := p.expect(TokenIdentifier); ok {
		trait.Name = t.Value
		trait.Line = t.Line
		if p.result.Namespace != "" {
			trait.FullName = p.result.Namespace + "\\" + trait.Name
		} else {
			trait.FullName = trait.Name
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	trait.Methods, trait.Properties, _ = p.parseClassBody()
	trait.EndLine = p.peek().Line
	p.result.Traits = append(p.result.Traits, trait)
}

func (p *structParser) parseEnum() {
	p.advance()
	e := EnumDef{}
	if t, ok := p.expect(TokenIdentifier); ok {
		e.Name = t.Value
		e.Line = t.Line
		if p.result.Namespace != "" {
			e.FullName = p.result.Namespace + "\\" + e.Name
		} else {
			e.FullName = e.Name
		}
	}
	if p.peek().Kind == TokenColon {
		p.advance()
		e.BackedType = p.readTypeName()
	}
	if p.peek().Kind == TokenImplements {
		p.advance()
		for {
			name := p.readTypeName()
			if name == "" {
				break
			}
			e.Implements = append(e.Implements, name)
			if p.peek().Kind == TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	depth := 1
	for depth > 0 && p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenOpenBrace:
			depth++
			p.advance()
		case TokenCloseBrace:
			depth--
			p.advance()
		case TokenIdentifier:
			if p.peek().Value == "case" {
				p.advance()
				c := EnumCase{Line: p.peek().Line}
				if t, ok := p.expect(TokenIdentifier); ok {
					c.Name = t.Value
				}
				if p.peek().Kind == TokenEquals {
					p.advance()
					c.Value = p.peek().Value
					p.advance()
				}
				e.Cases = append(e.Cases, c)
				p.expect(TokenSemicolon)
			} else {
				p.advance()
			}
		default:
			p.advance()
		}
	}
	e.EndLine = p.peek().Line
	p.result.Enums = append(p.result.Enums, e)
}

func (p *structParser) parseFunction(docComment string) {
	p.advance()
	fn := FunctionDef{DocComment: docComment}
	if t, ok := p.expect(TokenIdentifier); ok {
		fn.Name = t.Value
		fn.Line = t.Line
		if p.result.Namespace != "" {
			fn.FullName = p.result.Namespace + "\\" + fn.Name
		} else {
			fn.FullName = fn.Name
		}
	}
	fn.Params = p.parseParams()
	if p.peek().Kind == TokenColon {
		p.advance()
		fn.ReturnType = p.readTypeName()
	}
	p.skipBlock()
	fn.EndLine = p.peek().Line
	p.result.Functions = append(p.result.Functions, fn)
}

func (p *structParser) parseConst() {
	p.advance()
	c := ConstantDef{Line: p.peek().Line}
	if t, ok := p.expect(TokenIdentifier); ok {
		c.Name = t.Value
	}
	if p.peek().Kind == TokenEquals {
		p.advance()
		c.Value = p.peek().Value
		p.advance()
	}
	p.expect(TokenSemicolon)
	p.result.Constants = append(p.result.Constants, c)
}

func (p *structParser) parseClassBody() (methods []MethodDef, props []PropertyDef, consts []ConstantDef) {
	depth := 1
	var docComment string
	for depth > 0 && p.peek().Kind != TokenEOF {
		t := p.peek()
		if t.Kind == TokenDocComment {
			docComment = t.Value
			p.advance()
			continue
		}
		if t.Kind == TokenCloseBrace {
			depth--
			p.advance()
			continue
		}
		if t.Kind == TokenOpenBrace {
			depth++
			p.advance()
			continue
		}

		visibility := "public"
		isStatic, isAbstract, isFinal, isReadonly := false, false, false, false
		for {
			switch p.peek().Kind {
			case TokenPublic:
				visibility = "public"
				p.advance()
			case TokenProtected:
				visibility = "protected"
				p.advance()
			case TokenPrivate:
				visibility = "private"
				p.advance()
			case TokenStatic:
				isStatic = true
				p.advance()
			case TokenAbstract:
				isAbstract = true
				p.advance()
			case TokenFinal:
				isFinal = true
				p.advance()
			case TokenReadonly:
				isReadonly = true
				p.advance()
			default:
				goto doneMod
			}
		}
	doneMod:
		switch p.peek().Kind {
		case TokenFunction:
			p.advance()
			m := MethodDef{
				Visibility: visibility, IsStatic: isStatic,
				IsAbstract: isAbstract, IsFinal: isFinal,
				DocComment: docComment, Line: p.peek().Line,
			}
			if t, ok := p.expect(TokenIdentifier); ok {
				m.Name = t.Value
			}
			m.Params = p.parseParams()
			if p.peek().Kind == TokenColon {
				p.advance()
				m.ReturnType = p.readTypeName()
			}
			if p.peek().Kind == TokenOpenBrace {
				innerDepth := 1
				p.advance()
				for innerDepth > 0 && p.peek().Kind != TokenEOF {
					if p.peek().Kind == TokenOpenBrace {
						innerDepth++
					} else if p.peek().Kind == TokenCloseBrace {
						innerDepth--
					}
					p.advance()
				}
			} else {
				p.expect(TokenSemicolon)
			}
			m.EndLine = p.peek().Line
			methods = append(methods, m)

		case TokenConst:
			p.advance()
			c := ConstantDef{Line: p.peek().Line}
			if t, ok := p.expect(TokenIdentifier); ok {
				c.Name = t.Value
			}
			if p.peek().Kind == TokenEquals {
				p.advance()
				c.Value = p.peek().Value
				p.advance()
			}
			p.expect(TokenSemicolon)
			consts = append(consts, c)

		case TokenVariable:
			prop := PropertyDef{
				Visibility: visibility, IsStatic: isStatic,
				IsReadonly: isReadonly, DocComment: docComment,
				Line: p.peek().Line, Name: p.peek().Value,
			}
			p.advance()
			if p.peek().Kind == TokenEquals {
				prop.HasDefault = true
				p.advance()
				p.skipUntil(TokenSemicolon)
			}
			p.expect(TokenSemicolon)
			props = append(props, prop)

		case TokenIdentifier, TokenQuestion:
			isNullable := false
			if p.peek().Kind == TokenQuestion {
				isNullable = true
				p.advance()
			}
			typeName := p.readTypeName()
			if isNullable {
				typeName = "?" + typeName
			}
			// Union/intersection
			for p.peek().Kind == TokenPipe || p.peek().Kind == TokenAmpersand {
				op := p.peek().Value
				p.advance()
				typeName += op + p.readTypeName()
			}
			if p.peek().Kind == TokenVariable {
				prop := PropertyDef{
					Visibility: visibility, IsStatic: isStatic,
					IsReadonly: isReadonly, Type: typeName,
					DocComment: docComment, Line: p.peek().Line,
					Name: p.peek().Value,
				}
				p.advance()
				if p.peek().Kind == TokenEquals {
					prop.HasDefault = true
					p.advance()
					p.skipUntil(TokenSemicolon)
				}
				p.expect(TokenSemicolon)
				props = append(props, prop)
			} else {
				p.advance()
			}

		default:
			p.advance()
		}
		docComment = ""
	}
	return
}

func (p *structParser) parseParams() []ParamDef {
	if _, ok := p.expect(TokenOpenParen); !ok {
		return nil
	}
	var params []ParamDef
	for p.peek().Kind != TokenCloseParen && p.peek().Kind != TokenEOF {
		param := ParamDef{}
		for {
			switch p.peek().Kind {
			case TokenPublic:
				param.IsPromoted = true
				param.Visibility = "public"
				p.advance()
			case TokenProtected:
				param.IsPromoted = true
				param.Visibility = "protected"
				p.advance()
			case TokenPrivate:
				param.IsPromoted = true
				param.Visibility = "private"
				p.advance()
			case TokenReadonly:
				param.IsReadonly = true
				p.advance()
			default:
				goto doneParamMod
			}
		}
	doneParamMod:
		if p.peek().Kind == TokenQuestion {
			p.advance()
			param.Type = "?" + p.readTypeName()
		} else if p.peek().Kind == TokenIdentifier || p.peek().Kind == TokenBackslash {
			param.Type = p.readTypeName()
			for p.peek().Kind == TokenPipe || p.peek().Kind == TokenAmpersand {
				op := p.peek().Value
				p.advance()
				param.Type += op + p.readTypeName()
			}
		}
		if p.peek().Value == "..." {
			param.IsVariadic = true
			p.advance()
		}
		if p.peek().Kind == TokenVariable {
			param.Name = p.peek().Value
			p.advance()
		}
		if p.peek().Kind == TokenEquals {
			param.HasDefault = true
			p.advance()
			depth := 0
			for p.peek().Kind != TokenEOF {
				if p.peek().Kind == TokenOpenParen {
					depth++
				} else if p.peek().Kind == TokenCloseParen {
					if depth == 0 {
						break
					}
					depth--
				} else if p.peek().Kind == TokenComma && depth == 0 {
					break
				}
				p.advance()
			}
		}
		params = append(params, param)
		if p.peek().Kind == TokenComma {
			p.advance()
		}
	}
	p.expect(TokenCloseParen)
	return params
}

func (p *structParser) readTypeName() string {
	var parts []string
	for {
		t := p.peek()
		if t.Kind == TokenIdentifier {
			parts = append(parts, t.Value)
			p.advance()
		} else if t.Kind == TokenBackslash {
			parts = append(parts, "\\")
			p.advance()
		} else {
			break
		}
	}
	return strings.Join(parts, "")
}
