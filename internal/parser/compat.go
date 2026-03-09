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
}

type ConstantNode struct {
	Name      string
	Value     string
	Type      TypeNode
	StartLine int
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
	DocComment string
}

type TraitNode struct {
	Name       string
	FullName   string
	Properties []PropertyNode
	Methods    []MethodNode
	StartLine  int
	DocComment string
}

type EnumCaseNode struct {
	Name      string
	Value     string
	StartLine int
}

type EnumNode struct {
	Name       string
	FullName   string
	BackedType string
	Cases      []EnumCaseNode
	Methods    []MethodNode
	Implements []string
	StartLine  int
	DocComment string
}

type FunctionNode struct {
	Name       string
	FullName   string
	Params     []ParamNode
	ReturnType TypeNode
	StartLine  int
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

type DocBlock struct {
	Summary string
	Tags    map[string][]string
}

func ParseFile(source string) *FileNode {
	result := New().Parse(source)
	if result == nil {
		return nil
	}
	return toFileNode(result)
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
			classNode.Properties = append(classNode.Properties, toPropertyNode(propertyDef))
		}
		for _, methodDef := range classDef.Methods {
			classNode.Methods = append(classNode.Methods, toMethodNode(methodDef))
		}
		for _, constantDef := range classDef.Constants {
			classNode.Constants = append(classNode.Constants, toConstantNode(constantDef))
		}
		file.Classes = append(file.Classes, classNode)
	}

	for _, interfaceDef := range result.Interfaces {
		ifaceNode := InterfaceNode{
			Name:      interfaceDef.Name,
			FullName:  interfaceDef.FullName,
			Extends:   append([]string(nil), interfaceDef.Extends...),
			StartLine: interfaceDef.Line,
		}
		for _, methodDef := range interfaceDef.Methods {
			ifaceNode.Methods = append(ifaceNode.Methods, toMethodNode(methodDef))
		}
		file.Interfaces = append(file.Interfaces, ifaceNode)
	}

	for _, traitDef := range result.Traits {
		traitNode := TraitNode{
			Name:      traitDef.Name,
			FullName:  traitDef.FullName,
			StartLine: traitDef.Line,
		}
		for _, propertyDef := range traitDef.Properties {
			traitNode.Properties = append(traitNode.Properties, toPropertyNode(propertyDef))
		}
		for _, methodDef := range traitDef.Methods {
			traitNode.Methods = append(traitNode.Methods, toMethodNode(methodDef))
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
		}
		for _, enumCase := range enumDef.Cases {
			enumNode.Cases = append(enumNode.Cases, EnumCaseNode{
				Name:      enumCase.Name,
				Value:     enumCase.Value,
				StartLine: enumCase.Line,
			})
		}
		for _, methodDef := range enumDef.Methods {
			enumNode.Methods = append(enumNode.Methods, toMethodNode(methodDef))
		}
		file.Enums = append(file.Enums, enumNode)
	}

	for _, functionDef := range result.Functions {
		fnNode := FunctionNode{
			Name:       functionDef.Name,
			FullName:   functionDef.FullName,
			ReturnType: TypeNode{Name: functionDef.ReturnType},
			StartLine:  functionDef.Line,
			DocComment: functionDef.DocComment,
		}
		for _, paramDef := range functionDef.Params {
			fnNode.Params = append(fnNode.Params, toParamNode(paramDef))
		}
		file.Functions = append(file.Functions, fnNode)
	}

	for _, constantDef := range result.Constants {
		file.Constants = append(file.Constants, toConstantNode(constantDef))
	}

	return file
}

func toPropertyNode(propertyDef PropertyDef) PropertyNode {
	return PropertyNode{
		Name:       propertyDef.Name,
		Type:       TypeNode{Name: propertyDef.Type},
		Visibility: propertyDef.Visibility,
		IsStatic:   propertyDef.IsStatic,
		DocComment: propertyDef.DocComment,
		StartLine:  propertyDef.Line,
	}
}

func toMethodNode(methodDef MethodDef) MethodNode {
	methodNode := MethodNode{
		Name:       methodDef.Name,
		ReturnType: TypeNode{Name: methodDef.ReturnType},
		Visibility: methodDef.Visibility,
		IsStatic:   methodDef.IsStatic,
		IsAbstract: methodDef.IsAbstract,
		IsFinal:    methodDef.IsFinal,
		DocComment: methodDef.DocComment,
		StartLine:  methodDef.Line,
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

func toConstantNode(constantDef ConstantDef) ConstantNode {
	return ConstantNode{
		Name:      constantDef.Name,
		Value:     constantDef.Value,
		Type:      TypeNode{Name: constantDef.Type},
		StartLine: constantDef.Line,
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
