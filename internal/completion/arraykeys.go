package completion

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/phparray"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/symbols"
	"github.com/open-southeners/tusk-php/internal/types"
)

// arrayKeyContext holds the parsed context for array key completion.
type arrayKeyContext struct {
	VarName    string
	AccessKeys []string
	Partial    string
	Quote      string
}

func extractArrayKeyContext(prefix string) (varName, partial, quote string, ok bool) {
	ctx := parseArrayKeyContext(prefix)
	if ctx == nil {
		return
	}
	return ctx.VarName, ctx.Partial, ctx.Quote, true
}

func parseArrayKeyContext(prefix string) *arrayKeyContext {
	i := len(prefix) - 1

	partialStart := i + 1
	for i >= 0 && prefix[i] != '\'' && prefix[i] != '"' && prefix[i] != '[' {
		i--
	}
	if i < 0 {
		return nil
	}

	var partial, quote string
	if prefix[i] == '\'' || prefix[i] == '"' {
		quote = string(prefix[i])
		partial = prefix[i+1 : partialStart]
		i--
	} else if prefix[i] == '[' {
		partial = ""
	}

	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil
	}
	if prefix[i] == '[' {
		i--
	} else if quote != "" {
		return nil
	}

	var accessKeys []string
	for i >= 0 {
		for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
			i--
		}
		if i < 0 || prefix[i] != ']' {
			break
		}
		i--
		if i < 0 || (prefix[i] != '\'' && prefix[i] != '"') {
			break
		}
		closeQuote := prefix[i]
		i--

		keyEnd := i + 1
		for i >= 0 && prefix[i] != closeQuote {
			i--
		}
		if i < 0 {
			break
		}
		key := prefix[i+1 : keyEnd]
		i--

		for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
			i--
		}
		if i < 0 || prefix[i] != '[' {
			break
		}
		i--

		accessKeys = append([]string{key}, accessKeys...)
	}

	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil
	}

	end := i + 1
	for i >= 0 && resolve.IsWordChar(prefix[i]) {
		i--
	}
	if i >= 0 && prefix[i] == '$' {
		return &arrayKeyContext{
			VarName:    prefix[i:end],
			AccessKeys: accessKeys,
			Partial:    partial,
			Quote:      quote,
		}
	}
	return nil
}

func (p *Provider) completeArrayKeys(source string, pos protocol.Position, ctx *arrayKeyContext, file *parser.FileNode) []protocol.CompletionItem {
	keys := p.resolveArrayKeysFromType(source, pos, ctx.VarName, file)
	if len(keys) == 0 {
		keys = scanLiteralArrayKeys(source, pos, ctx.VarName)
	}

	for _, accessKey := range ctx.AccessKeys {
		var nestedType string
		for _, f := range keys {
			if f.Key == accessKey {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		keys = types.ParseArrayShape(nestedType)
		if len(keys) == 0 {
			return nil
		}
	}

	if len(keys) == 0 {
		return nil
	}

	q := "'"
	if ctx.Quote == "\"" {
		q = "\""
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(ctx.Partial)
	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}
		detail := k.Type
		if k.Optional {
			detail += " (optional)"
		}

		insertText := k.Key
		if ctx.Quote == "" {
			insertText = q + k.Key + q
		}

		sortText := "0" + k.Key
		if k.Optional {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}

func (p *Provider) resolveArrayKeysFromType(source string, pos protocol.Position, varName string, file *parser.FileNode) []types.ShapeField {
	if file == nil {
		return nil
	}

	bare := strings.TrimPrefix(varName, "$")

	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if pos.Line >= m.StartLine {
				if m.DocComment != "" {
					if fields := extractShapeFromDocParams(m.DocComment, bare); len(fields) > 0 {
						return fields
					}
				}
				for _, param := range m.Params {
					if param.Name == varName {
						if fields := types.ParseArrayShape(param.Type.Name); len(fields) > 0 {
							return fields
						}
					}
				}
			}
		}
	}
	for _, fn := range file.Functions {
		if pos.Line >= fn.StartLine {
			if fn.DocComment != "" {
				if fields := extractShapeFromDocParams(fn.DocComment, bare); len(fields) > 0 {
					return fields
				}
			}
			for _, param := range fn.Params {
				if param.Name == varName {
					if fields := types.ParseArrayShape(param.Type.Name); len(fields) > 0 {
						return fields
					}
				}
			}
		}
	}

	lines := strings.Split(source, "\n")
	for i := pos.Line; i >= 0 && i >= pos.Line-10; i-- {
		if i >= len(lines) {
			continue
		}
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "@var") && strings.Contains(line, varName) {
			varIdx := strings.Index(line, "@var ")
			if varIdx >= 0 {
				rest := strings.TrimSpace(line[varIdx+5:])
				typeStr, _ := types.ExtractDocTypeString(rest)
				if fields := types.ParseArrayShape(typeStr); len(fields) > 0 {
					return fields
				}
			}
		}
	}

	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, varName) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(varName):])
		if !strings.HasPrefix(rest, "=") {
			continue
		}
		rhs := strings.TrimSpace(rest[1:])

		if p.arrayResolver != nil {
			if fields := p.arrayResolver.ResolveCallReturnKeys(rhs, source); len(fields) > 0 {
				return fields
			}
		}

		if retType := p.resolveCallReturnType(rhs, source, file); retType != "" {
			if fields := types.ParseArrayShape(retType); len(fields) > 0 {
				return fields
			}
		}

		if p.arrayResolver != nil {
			if classFQN, methodName := parseMethodCall(rhs, file); classFQN != "" && methodName != "" {
				if fields := p.arrayResolver.ResolveMethodReturnKeys(classFQN, methodName); len(fields) > 0 {
					return fields
				}
			}
		}
		break
	}

	return nil
}

func parseMethodCall(expr string, file *parser.FileNode) (classFQN, methodName string) {
	expr = strings.TrimSuffix(strings.TrimSpace(expr), ";")
	if parenIdx := strings.Index(expr, "("); parenIdx > 0 {
		expr = expr[:parenIdx]
	}
	if !strings.Contains(expr, "->") {
		return
	}
	parts := strings.Split(expr, "->")
	methodName = parts[len(parts)-1]

	target := strings.TrimSpace(parts[0])
	if target == "$this" && file != nil && len(file.Classes) > 0 {
		cls := file.Classes[0]
		if cls.FullName != "" {
			classFQN = cls.FullName
		} else if file.Namespace != "" {
			classFQN = file.Namespace + "\\" + cls.Name
		} else {
			classFQN = cls.Name
		}
	}
	return
}

func extractShapeFromDocParams(docComment, paramBare string) []types.ShapeField {
	doc := parser.ParseDocBlock(docComment)
	if doc == nil {
		return nil
	}
	target := "$" + paramBare
	for _, param := range doc.Params {
		if param.Name == target {
			return types.ParseArrayShape(param.Type)
		}
	}
	if vars, ok := doc.Tags["var"]; ok {
		for _, v := range vars {
			typeStr, rest := types.ExtractDocTypeString(v)
			if strings.Contains(rest, target) {
				return types.ParseArrayShape(typeStr)
			}
		}
	}
	return nil
}

func (p *Provider) resolveCallReturnType(expr, source string, file *parser.FileNode) string {
	expr = strings.TrimSuffix(strings.TrimSpace(expr), ";")
	if parenIdx := strings.Index(expr, "("); parenIdx > 0 {
		expr = expr[:parenIdx]
	}
	expr = strings.TrimSpace(expr)

	if strings.Contains(expr, "->") {
		parts := strings.Split(expr, "->")
		methodName := parts[len(parts)-1]
		if file != nil && len(file.Classes) > 0 {
			cls := file.Classes[0]
			classFQN := cls.FullName
			if classFQN == "" && file.Namespace != "" {
				classFQN = file.Namespace + "\\" + cls.Name
			} else if classFQN == "" {
				classFQN = cls.Name
			}
			for _, member := range p.index.GetClassMembers(classFQN) {
				if member.Name == methodName && member.Kind == symbols.KindMethod {
					if member.DocComment != "" {
						doc := parser.ParseDocBlock(member.DocComment)
						if doc != nil && doc.Return.Type != "" {
							return doc.Return.Type
						}
					}
					return member.ReturnType
				}
			}
		}
	}

	funcName := expr
	syms := p.index.LookupByName(funcName)
	for _, sym := range syms {
		if sym.Kind == symbols.KindFunction || sym.Kind == symbols.KindMethod {
			if sym.DocComment != "" {
				doc := parser.ParseDocBlock(sym.DocComment)
				if doc != nil && doc.Return.Type != "" {
					return doc.Return.Type
				}
			}
			return sym.ReturnType
		}
	}
	return ""
}

func scanLiteralArrayKeys(source string, pos protocol.Position, varName string) []types.ShapeField {
	lines := strings.Split(source, "\n")
	var keys []types.ShapeField
	seen := make(map[string]bool)

	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])

		if strings.HasPrefix(trimmed, varName) {
			rest := strings.TrimSpace(trimmed[len(varName):])
			if strings.HasPrefix(rest, "=") {
				rhs := strings.TrimSpace(rest[1:])
				if strings.HasPrefix(rhs, "[") || strings.HasPrefix(strings.ToLower(rhs), "array(") {
					arrayText := phparray.CollectArrayLiteral(lines, i)
					parsed := phparray.ParseLiteralToShape(arrayText)
					if len(parsed) > 0 {
						keys = parsed
						break
					}
				}
			}
		}

		if k := extractIncrementalKey(trimmed, varName); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, types.ShapeField{Key: k})
		}
	}

	for i := pos.Line + 1; i < len(lines) && i <= pos.Line+50; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if k := extractIncrementalKey(trimmed, varName); k != "" && !seen[k] {
			seen[k] = true
			keys = append(keys, types.ShapeField{Key: k})
		}
	}

	return keys
}

func extractIncrementalKey(trimmed, varName string) string {
	if !strings.HasPrefix(trimmed, varName+"[") {
		return ""
	}
	after := trimmed[len(varName)+1:]
	if len(after) < 3 || (after[0] != '\'' && after[0] != '"') {
		return ""
	}
	q := after[0]
	endQ := strings.IndexByte(after[1:], q)
	if endQ <= 0 {
		return ""
	}
	rest := strings.TrimSpace(after[endQ+2:])
	if !strings.HasPrefix(rest, "]") {
		return ""
	}
	afterBracket := strings.TrimSpace(rest[1:])
	if !strings.HasPrefix(afterBracket, "=") {
		return ""
	}
	return after[1 : endQ+1]
}
