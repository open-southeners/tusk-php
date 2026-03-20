package parser

import "strings"

// Compatibility types kept for older callers that still expect the previous AST shape.
type TypeNode struct {
	Name string
}

type UseNode struct {
	FullName  string
	Alias     string
	Kind      string
	StartLine int
}

type ParamNode struct {
	Name         string
	Type         TypeNode
	DefaultValue string
	IsVariadic   bool
	IsReference  bool
}

type PropertyNode struct {
	Name       string
	Type       TypeNode
	Visibility string
	IsStatic   bool
	DocComment string
	StartLine  int
	StartCol   int
}

type MethodNode struct {
	Name       string
	Params     []ParamNode
	ReturnType TypeNode
	Visibility string
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	DocComment string
	StartLine  int
	StartCol   int
}

type ConstantNode struct {
	Name      string
	Value     string
	Type      TypeNode
	StartLine int
	StartCol  int
}

type ClassNode struct {
	Name       string
	FullName   string
	Extends    string
	Implements []string
	Traits     []string
	Properties []PropertyNode
	Methods    []MethodNode
	Constants  []ConstantNode
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	StartLine  int
	StartCol   int
	DocComment string
}

type InterfaceNode struct {
	Name       string
	FullName   string
	Extends    []string
	Methods    []MethodNode
	StartLine  int
	StartCol   int
	DocComment string
}

type TraitNode struct {
	Name       string
	FullName   string
	Properties []PropertyNode
	Methods    []MethodNode
	StartLine  int
	StartCol   int
	DocComment string
}

type EnumCaseNode struct {
	Name      string
	Value     string
	StartLine int
	StartCol  int
}

type EnumNode struct {
	Name       string
	FullName   string
	BackedType string
	Cases      []EnumCaseNode
	Methods    []MethodNode
	Implements []string
	StartLine  int
	StartCol   int
	DocComment string
}

type FunctionNode struct {
	Name       string
	FullName   string
	Params     []ParamNode
	ReturnType TypeNode
	StartLine  int
	StartCol   int
	DocComment string
}

type FileNode struct {
	Namespace  string
	Uses       []UseNode
	Classes    []ClassNode
	Interfaces []InterfaceNode
	Traits     []TraitNode
	Enums      []EnumNode
	Functions  []FunctionNode
	Constants  []ConstantNode
}

type DocParam struct {
	Type        string
	Name        string
	Description string
}

type DocReturn struct {
	Type        string
	Description string
}

type DocThrow struct {
	Type        string
	Description string
}

type DocBlock struct {
	Summary       string
	Tags          map[string][]string
	Params        []DocParam
	Return        DocReturn
	Throws        []DocThrow
	Deprecated    bool
	DeprecatedMsg string
}

func ParseFile(source string) (file *FileNode) {
	var result *ParseResult
	defer func() {
		if r := recover(); r != nil {
			// Return partial results if available rather than nil.
			if result != nil {
				func() {
					defer func() { recover() }()
					file = toFileNode(result)
				}()
			}
		}
	}()
	result = New().Parse(source)
	if result == nil {
		return nil
	}
	file = toFileNode(result)
	return
}

func ParseDocBlock(raw string) *DocBlock {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	trimmed = strings.TrimPrefix(trimmed, "/**")
	trimmed = strings.TrimSuffix(trimmed, "*/")
	lines := strings.Split(trimmed, "\n")
	doc := &DocBlock{Tags: make(map[string][]string)}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@") {
			parts := strings.SplitN(line[1:], " ", 2)
			name := parts[0]
			value := ""
			if len(parts) == 2 {
				value = strings.TrimSpace(parts[1])
			}
			doc.Tags[name] = append(doc.Tags[name], value)

			switch name {
			case "param":
				doc.Params = append(doc.Params, parseDocParam(value))
			case "return":
				doc.Return = parseDocReturn(value)
			case "throws":
				doc.Throws = append(doc.Throws, parseDocThrow(value))
			case "deprecated":
				doc.Deprecated = true
				doc.DeprecatedMsg = value
			}
			continue
		}
		if doc.Summary == "" {
			doc.Summary = line
		}
	}

	if doc.Summary == "" && len(doc.Tags) == 0 {
		return nil
	}
	return doc
}

// parseDocParam parses "@param Type $name Description" into structured DocParam.
func parseDocParam(value string) DocParam {
	p := DocParam{}
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return p
	}
	idx := 0
	// First token that starts with $ is the name; everything before is the type
	if len(parts) > 0 && !strings.HasPrefix(parts[0], "$") {
		p.Type = parts[0]
		idx = 1
	}
	if idx < len(parts) && strings.HasPrefix(parts[idx], "$") {
		p.Name = parts[idx]
		idx++
	}
	if idx < len(parts) {
		p.Description = strings.Join(parts[idx:], " ")
	}
	return p
}

// parseDocReturn parses "@return Type Description" into structured DocReturn.
func parseDocReturn(value string) DocReturn {
	r := DocReturn{}
	parts := strings.SplitN(value, " ", 2)
	if len(parts) >= 1 {
		r.Type = parts[0]
	}
	if len(parts) >= 2 {
		r.Description = strings.TrimSpace(parts[1])
	}
	return r
}

// parseDocThrow parses "@throws Type Description" into structured DocThrow.
func parseDocThrow(value string) DocThrow {
	th := DocThrow{}
	parts := strings.SplitN(value, " ", 2)
	if len(parts) >= 1 {
		th.Type = parts[0]
	}
	if len(parts) >= 2 {
		th.Description = strings.TrimSpace(parts[1])
	}
	return th
}

func toFileNode(result *ParseResult) *FileNode {
	file := &FileNode{Namespace: result.Namespace}

	for _, useStmt := range result.Uses {
		file.Uses = append(file.Uses, UseNode{
			FullName:  useStmt.FullName,
			Alias:     useStmt.Alias,
			Kind:      useStmt.Kind,
			StartLine: useStmt.Line,
		})
	}

	for _, classDef := range result.Classes {
		classNode := ClassNode{
			Name:       classDef.Name,
			FullName:   classDef.FullName,
			Extends:    classDef.Extends,
			Implements: append([]string(nil), classDef.Implements...),
			Traits:     append([]string(nil), classDef.Traits...),
			IsAbstract: classDef.IsAbstract,
			IsFinal:    classDef.IsFinal,
			IsReadonly: classDef.IsReadonly,
			StartLine:  classDef.Line,
			StartCol:   startColumnForDeclaration(result, classDef.Name, classDef.Line, TokenClass),
			DocComment: classDef.DocComment,
		}
		for _, propertyDef := range classDef.Properties {
			classNode.Properties = append(classNode.Properties, toPropertyNode(result, propertyDef))
		}
		for _, methodDef := range classDef.Methods {
			classNode.Methods = append(classNode.Methods, toMethodNode(result, methodDef))
		}
		for _, constantDef := range classDef.Constants {
			classNode.Constants = append(classNode.Constants, toConstantNode(result, constantDef))
		}
		file.Classes = append(file.Classes, classNode)
	}

	for _, interfaceDef := range result.Interfaces {
		ifaceNode := InterfaceNode{
			Name:      interfaceDef.Name,
			FullName:  interfaceDef.FullName,
			Extends:   append([]string(nil), interfaceDef.Extends...),
			StartLine: interfaceDef.Line,
			StartCol:  startColumnForDeclaration(result, interfaceDef.Name, interfaceDef.Line, TokenInterface),
		}
		for _, methodDef := range interfaceDef.Methods {
			ifaceNode.Methods = append(ifaceNode.Methods, toMethodNode(result, methodDef))
		}
		file.Interfaces = append(file.Interfaces, ifaceNode)
	}

	for _, traitDef := range result.Traits {
		traitNode := TraitNode{
			Name:      traitDef.Name,
			FullName:  traitDef.FullName,
			StartLine: traitDef.Line,
			StartCol:  startColumnForDeclaration(result, traitDef.Name, traitDef.Line, TokenTrait),
		}
		for _, propertyDef := range traitDef.Properties {
			traitNode.Properties = append(traitNode.Properties, toPropertyNode(result, propertyDef))
		}
		for _, methodDef := range traitDef.Methods {
			traitNode.Methods = append(traitNode.Methods, toMethodNode(result, methodDef))
		}
		file.Traits = append(file.Traits, traitNode)
	}

	for _, enumDef := range result.Enums {
		enumNode := EnumNode{
			Name:       enumDef.Name,
			FullName:   enumDef.FullName,
			BackedType: enumDef.BackedType,
			Implements: append([]string(nil), enumDef.Implements...),
			StartLine:  enumDef.Line,
			StartCol:   startColumnForDeclaration(result, enumDef.Name, enumDef.Line, TokenEnum),
		}
		for _, enumCase := range enumDef.Cases {
			enumNode.Cases = append(enumNode.Cases, EnumCaseNode{
				Name:      enumCase.Name,
				Value:     enumCase.Value,
				StartLine: enumCase.Line,
				StartCol:  nameColumnOnLine(result, enumCase.Name, enumCase.Line),
			})
		}
		for _, methodDef := range enumDef.Methods {
			enumNode.Methods = append(enumNode.Methods, toMethodNode(result, methodDef))
		}
		file.Enums = append(file.Enums, enumNode)
	}

	for _, functionDef := range result.Functions {
		fnNode := FunctionNode{
			Name:       functionDef.Name,
			FullName:   functionDef.FullName,
			ReturnType: TypeNode{Name: functionDef.ReturnType},
			StartLine:  functionDef.Line,
			StartCol:   startColumnForDeclaration(result, functionDef.Name, functionDef.Line, TokenFunction),
			DocComment: functionDef.DocComment,
		}
		for _, paramDef := range functionDef.Params {
			fnNode.Params = append(fnNode.Params, toParamNode(paramDef))
		}
		file.Functions = append(file.Functions, fnNode)
	}

	for _, constantDef := range result.Constants {
		file.Constants = append(file.Constants, toConstantNode(result, constantDef))
	}

	return file
}

func toPropertyNode(result *ParseResult, propertyDef PropertyDef) PropertyNode {
	return PropertyNode{
		Name:       propertyDef.Name,
		Type:       TypeNode{Name: propertyDef.Type},
		Visibility: propertyDef.Visibility,
		IsStatic:   propertyDef.IsStatic,
		DocComment: propertyDef.DocComment,
		StartLine:  propertyDef.Line,
		StartCol:   nameColumnOnLine(result, propertyDef.Name, propertyDef.Line),
	}
}

func toMethodNode(result *ParseResult, methodDef MethodDef) MethodNode {
	methodNode := MethodNode{
		Name:       methodDef.Name,
		ReturnType: TypeNode{Name: methodDef.ReturnType},
		Visibility: methodDef.Visibility,
		IsStatic:   methodDef.IsStatic,
		IsAbstract: methodDef.IsAbstract,
		IsFinal:    methodDef.IsFinal,
		DocComment: methodDef.DocComment,
		StartLine:  methodDef.Line,
		StartCol:   startColumnForDeclaration(result, methodDef.Name, methodDef.Line, TokenFunction),
	}
	for _, paramDef := range methodDef.Params {
		methodNode.Params = append(methodNode.Params, toParamNode(paramDef))
	}
	return methodNode
}

func toParamNode(paramDef ParamDef) ParamNode {
	return ParamNode{
		Name:       paramDef.Name,
		Type:       TypeNode{Name: paramDef.Type},
		IsVariadic: paramDef.IsVariadic,
	}
}

func toConstantNode(result *ParseResult, constantDef ConstantDef) ConstantNode {
	return ConstantNode{
		Name:      constantDef.Name,
		Value:     constantDef.Value,
		Type:      TypeNode{Name: constantDef.Type},
		StartLine: constantDef.Line,
		StartCol:  nameColumnOnLine(result, constantDef.Name, constantDef.Line),
	}
}

func startColumnForDeclaration(result *ParseResult, name string, line int, kind TokenKind) int {
	for i := 0; i < len(result.Tokens)-1; i++ {
		token := result.Tokens[i]
		if token.Kind != kind || token.Line != line {
			continue
		}
		for j := i + 1; j < len(result.Tokens); j++ {
			next := result.Tokens[j]
			if next.Line != line {
				break
			}
			if next.Kind == TokenIdentifier && next.Value == name {
				return next.Column
			}
		}
		return token.Column
	}
	return 0
}

// nameColumnOnLine finds the column of an identifier token with the given name on the given line.
// Falls back to searching for a variable token ($name) for property declarations.
func nameColumnOnLine(result *ParseResult, name string, line int) int {
	for _, token := range result.Tokens {
		if token.Line != line {
			if token.Line > line {
				break
			}
			continue
		}
		if token.Kind == TokenIdentifier && token.Value == name {
			return token.Column
		}
		// For properties, the name includes $ in the token
		if token.Kind == TokenVariable && (token.Value == name || token.Value == "$"+name) {
			return token.Column
		}
	}
	return 0
}
