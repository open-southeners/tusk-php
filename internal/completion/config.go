package completion

import (
	"strings"

	"github.com/open-southeners/php-lsp/internal/protocol"
	"github.com/open-southeners/php-lsp/internal/resolve"
	"github.com/open-southeners/php-lsp/internal/types"
)

type configResultArrayContext struct {
	ConfigArg  string
	AccessKeys []string
	Partial    string
	Quote      string
}

func parseConfigResultArrayContext(prefix string) *configResultArrayContext {
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
		closeQ := prefix[i]
		i--
		keyEnd := i + 1
		for i >= 0 && prefix[i] != closeQ {
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
	if i < 0 || prefix[i] != ')' {
		return nil
	}
	i--

	depth := 1
	for i >= 0 && depth > 0 {
		if prefix[i] == ')' {
			depth++
		} else if prefix[i] == '(' {
			depth--
		}
		if depth > 0 {
			i--
		}
	}
	if i < 0 || prefix[i] != '(' {
		return nil
	}
	argContent := prefix[i+1:]
	closeP := strings.Index(argContent, ")")
	if closeP < 0 {
		return nil
	}
	argContent = strings.TrimSpace(argContent[:closeP])
	configArg := ""
	if len(argContent) >= 2 && (argContent[0] == '\'' || argContent[0] == '"') && argContent[len(argContent)-1] == argContent[0] {
		configArg = argContent[1 : len(argContent)-1]
	}
	if configArg == "" {
		return nil
	}

	i--

	for i >= 0 && (prefix[i] == ' ' || prefix[i] == '\t') {
		i--
	}
	end := i + 1
	for i >= 0 && resolve.IsWordChar(prefix[i]) {
		i--
	}
	funcName := prefix[i+1 : end]
	if funcName != "config" {
		return nil
	}

	return &configResultArrayContext{
		ConfigArg:  configArg,
		AccessKeys: accessKeys,
		Partial:    partial,
		Quote:      quote,
	}
}

func (p *Provider) completeConfigResultKeys(ctx *configResultArrayContext) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	parts := strings.Split(ctx.ConfigArg, ".")
	configFile := parts[0]
	keys := p.arrayResolver.ParseConfigFile(configFile)
	if keys == nil {
		return nil
	}

	for _, segment := range parts[1:] {
		var nestedType string
		for _, f := range keys {
			if f.Key == segment {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		keys = types.ParseArrayShape(nestedType)
		if keys == nil {
			return nil
		}
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
		if keys == nil {
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
		insertText := k.Key
		if ctx.Quote == "" {
			insertText = q + k.Key + q
		}
		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       protocol.CompletionItemKindProperty,
			Detail:     k.Type,
			InsertText: insertText,
			SortText:   "0" + k.Key,
		})
	}
	return items
}

func extractConfigArgContext(trimmed string) (configPath, partial, quote string, ok bool) {
	idx := strings.LastIndex(trimmed, "config(")
	if idx < 0 {
		return
	}
	after := trimmed[idx+len("config("):]
	if strings.Contains(after, ")") {
		return
	}
	if len(after) > 0 && (after[0] == '\'' || after[0] == '"') {
		quote = string(after[0])
		after = after[1:]
	} else if len(after) == 0 {
		ok = true
		return
	}
	if quote != "" && len(after) > 0 && after[len(after)-1] == quote[0] {
		after = after[:len(after)-1]
	}

	lastDot := strings.LastIndex(after, ".")
	if lastDot >= 0 {
		configPath = after[:lastDot]
		partial = after[lastDot+1:]
	} else {
		partial = after
	}
	ok = true
	return
}

func (p *Provider) completeConfigKeys(configPath, partial, quote string) []protocol.CompletionItem {
	if p.arrayResolver == nil {
		return nil
	}

	var keys []types.ShapeField
	if configPath == "" {
		keys = p.arrayResolver.ListConfigFiles()
	} else {
		parts := strings.Split(configPath, ".")
		configFile := parts[0]
		keys = p.arrayResolver.ParseConfigFile(configFile)
		if keys == nil {
			return nil
		}
		for _, segment := range parts[1:] {
			var nestedType string
			for _, f := range keys {
				if f.Key == segment {
					nestedType = f.Type
					break
				}
			}
			if nestedType == "" {
				return nil
			}
			keys = types.ParseArrayShape(nestedType)
			if keys == nil {
				return nil
			}
		}
	}

	if len(keys) == 0 {
		return nil
	}

	var items []protocol.CompletionItem
	lpartial := strings.ToLower(partial)

	for _, k := range keys {
		if k.Key == "" {
			continue
		}
		if lpartial != "" && !strings.HasPrefix(strings.ToLower(k.Key), lpartial) {
			continue
		}

		isNested := strings.HasPrefix(k.Type, "array{") || k.Type == "array"
		detail := k.Type
		if isNested && strings.HasPrefix(k.Type, "array{") {
			inner := types.ParseArrayShape(k.Type)
			if len(inner) > 0 {
				var names []string
				for _, f := range inner {
					if f.Key != "" {
						names = append(names, f.Key)
					}
					if len(names) >= 4 {
						names = append(names, "...")
						break
					}
				}
				detail = "{" + strings.Join(names, ", ") + "}"
			}
		}

		insertText := k.Key
		if isNested {
			insertText = k.Key + "."
		}
		if quote == "" {
			insertText = "'" + insertText + "'"
		}

		kind := protocol.CompletionItemKindProperty
		if isNested {
			kind = protocol.CompletionItemKindModule
		}

		sortText := "0" + k.Key
		if isNested {
			sortText = "1" + k.Key
		}

		items = append(items, protocol.CompletionItem{
			Label:      k.Key,
			Kind:       kind,
			Detail:     detail,
			InsertText: insertText,
			SortText:   sortText,
		})
	}
	return items
}
