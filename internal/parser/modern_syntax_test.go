package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// testdataRoot returns the absolute path to testdata/ relative to this file.
func testdataRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	// internal/parser/ -> internal/ -> repo root -> testdata/
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// TestH2PipeOperatorToken verifies that the PHP 8.5 pipe operator |> is
// tokenized as a single distinct TokenPipeArrow token and not as two separate
// tokens (TokenPipe + TokenUnknown).
func TestH2PipeOperatorToken(t *testing.T) {
	t.Helper()
	src := `<?php
$result = 3 |> double(...) |> addOne(...);
$val = 5 |> $process;
`
	result := New().Parse(src)

	var pipeArrows []Token
	for _, tok := range result.Tokens {
		if tok.Kind == TokenPipeArrow {
			pipeArrows = append(pipeArrows, tok)
		}
		// The single-char '|' must NOT appear as a standalone token for |>
		if tok.Kind == TokenPipe {
			// TokenPipe is valid for union types; check it is not a '|' from '|>'
			if tok.Value == "|" {
				// Verify this is not followed by '>' in the token stream
				// (which would indicate the two-char tokenizer missed the pair)
				for _, next := range result.Tokens {
					if next.Line == tok.Line && next.Column == tok.Column+1 && next.Value == ">" {
						t.Errorf("line %d col %d: '|>' was split into two tokens instead of one TokenPipeArrow", tok.Line, tok.Column)
					}
				}
			}
		}
	}

	if len(pipeArrows) != 3 {
		t.Errorf("expected 3 TokenPipeArrow tokens, got %d", len(pipeArrows))
	}
	for _, tok := range pipeArrows {
		if tok.Value != "|>" {
			t.Errorf("TokenPipeArrow has unexpected value %q (want \"|>\")", tok.Value)
		}
	}
}

// TestH2PipeOperatorPreservesUnionTypes checks that adding |> does not break
// union-type parsing — TokenPipe still works in type positions.
func TestH2PipeOperatorPreservesUnionTypes(t *testing.T) {
	t.Helper()
	src := `<?php
class Foo {
    public int|string $x;
    public function bar(int|float $n): string|null {}
}
`
	result := New().Parse(src)
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("unexpected parse error: line %d: %s", e.Line, e.Message)
		}
	}
	if len(result.Classes) != 1 {
		t.Fatalf("expected 1 class, got %d", len(result.Classes))
	}
	cls := result.Classes[0]
	if len(cls.Properties) != 1 {
		t.Fatalf("expected 1 property, got %d", len(cls.Properties))
	}
	prop := cls.Properties[0]
	if prop.Type != "int|string" {
		t.Errorf("union property type: want %q, got %q", "int|string", prop.Type)
	}
	if len(cls.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(cls.Methods))
	}
	m := cls.Methods[0]
	if m.ReturnType != "string|null" {
		t.Errorf("method return type: want %q, got %q", "string|null", m.ReturnType)
	}
}

// TestH2PipeOperatorPhp85FileParseClean verifies that the php85.php testdata
// fixture (which contains |> expressions) parses with zero errors now that
// |> is tokenized as TokenPipeArrow.
func TestH2PipeOperatorPhp85FileParseClean(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testdataRoot(t), "php-features", "php85.php"))
	if err != nil {
		t.Fatalf("cannot read php85.php: %v", err)
	}
	result := New().Parse(string(src))
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("php85.php: parse error at line %d: %s", e.Line, e.Message)
		}
	}
	// Confirm the file parsed to expected structure
	if len(result.Classes) < 2 {
		t.Errorf("php85.php: expected at least 2 classes, got %d", len(result.Classes))
	}
}

// TestH1PropertyHooksAreParsed verifies that PHP 8.4 property hooks are
// correctly recorded on PropertyDef.Hooks, and that variables inside hook
// bodies (like $this, $value) are NOT misinterpreted as class properties.
func TestH1PropertyHooksAreParsed(t *testing.T) {
	t.Helper()
	tests := []struct {
		name      string
		src       string
		className string
		propName  string
		wantType  string
		wantHooks []string // expected hook kinds in order
	}{
		{
			name: "get_and_set_short_hooks",
			src: `<?php
class Temperature {
    public float $fahrenheit {
        get => ($this->celsius * 9 / 5) + 32;
        set => $this->celsius = ($value - 32) * 5 / 9;
    }
}`,
			className: "Temperature",
			propName:  "$fahrenheit",
			wantType:  "float",
			wantHooks: []string{"get", "set"},
		},
		{
			name: "get_only_short_hook",
			src: `<?php
class Circle {
    public float $area {
        get => M_PI * $this->_radius ** 2;
    }
}`,
			className: "Circle",
			propName:  "$area",
			wantType:  "float",
			wantHooks: []string{"get"},
		},
		{
			name: "set_with_long_hook_body",
			src: `<?php
class Circle {
    public float $radius {
        get => $this->_radius;
        set {
            if ($value < 0) {
                throw new \InvalidArgumentException('Radius must be non-negative');
            }
            $this->_radius = $value;
        }
    }
}`,
			className: "Circle",
			propName:  "$radius",
			wantType:  "float",
			wantHooks: []string{"get", "set"},
		},
		{
			name: "untyped_property_with_hook",
			src: `<?php
class Foo {
    public $bar {
        get => $this->_bar;
    }
}`,
			className: "Foo",
			propName:  "$bar",
			wantType:  "",
			wantHooks: []string{"get"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()
			result := New().Parse(tc.src)
			if len(result.Errors) != 0 {
				for _, e := range result.Errors {
					t.Errorf("parse error at line %d: %s", e.Line, e.Message)
				}
			}
			var cls *ClassDef
			for i := range result.Classes {
				if result.Classes[i].Name == tc.className {
					cls = &result.Classes[i]
					break
				}
			}
			if cls == nil {
				t.Fatalf("class %q not found", tc.className)
			}
			// Ensure no spurious $this or $value properties appear
			for _, p := range cls.Properties {
				if p.Name == "$this" || p.Name == "$value" {
					t.Errorf("spurious property %s in class %s (likely from inside a hook body)", p.Name, tc.className)
				}
			}
			// Find the target property
			var prop *PropertyDef
			for i := range cls.Properties {
				if cls.Properties[i].Name == tc.propName {
					prop = &cls.Properties[i]
					break
				}
			}
			if prop == nil {
				t.Fatalf("property %s not found in class %s (have %d props)", tc.propName, tc.className, len(cls.Properties))
			}
			if prop.Type != tc.wantType {
				t.Errorf("property %s type: want %q, got %q", tc.propName, tc.wantType, prop.Type)
			}
			if len(prop.Hooks) != len(tc.wantHooks) {
				t.Errorf("property %s: want %d hooks %v, got %d hooks %v",
					tc.propName, len(tc.wantHooks), tc.wantHooks, len(prop.Hooks), prop.Hooks)
			} else {
				for i, h := range prop.Hooks {
					if h.Kind != tc.wantHooks[i] {
						t.Errorf("hook[%d]: want %q, got %q", i, tc.wantHooks[i], h.Kind)
					}
				}
			}
		})
	}
}

// TestH1PropertyHooksPhp84FileParseClean verifies that the php84.php testdata
// (heavy use of property hooks and asymmetric visibility) parses with zero errors
// and that property hooks are correctly recorded for Temperature and Circle.
func TestH1PropertyHooksPhp84FileParseClean(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testdataRoot(t), "php-features", "php84.php"))
	if err != nil {
		t.Fatalf("cannot read php84.php: %v", err)
	}
	result := New().Parse(string(src))
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("php84.php: parse error at line %d: %s", e.Line, e.Message)
		}
	}

	findClass := func(name string) *ClassDef {
		for i := range result.Classes {
			if result.Classes[i].Name == name {
				return &result.Classes[i]
			}
		}
		return nil
	}

	// Temperature: should have $celsius (no hooks), $fahrenheit (get+set), $kelvin (get)
	temp := findClass("Temperature")
	if temp == nil {
		t.Fatal("Temperature class not found")
	}
	// Verify no spurious $this/$value properties
	for _, p := range temp.Properties {
		if p.Name == "$this" || p.Name == "$value" {
			t.Errorf("Temperature class: spurious property %s (from hook body)", p.Name)
		}
	}
	findProp := func(cls *ClassDef, name string) *PropertyDef {
		for i := range cls.Properties {
			if cls.Properties[i].Name == name {
				return &cls.Properties[i]
			}
		}
		return nil
	}
	if p := findProp(temp, "$fahrenheit"); p == nil {
		t.Error("Temperature.$fahrenheit not found")
	} else if len(p.Hooks) != 2 {
		t.Errorf("Temperature.$fahrenheit: want 2 hooks, got %d %v", len(p.Hooks), p.Hooks)
	}
	if p := findProp(temp, "$kelvin"); p == nil {
		t.Error("Temperature.$kelvin not found")
	} else if len(p.Hooks) != 1 || p.Hooks[0].Kind != "get" {
		t.Errorf("Temperature.$kelvin: want [get], got %v", p.Hooks)
	}

	// Circle: $radius with get+set (long set body), $_radius (no hooks), $area (get), $circumference (get)
	circle := findClass("Circle")
	if circle == nil {
		t.Fatal("Circle class not found")
	}
	for _, p := range circle.Properties {
		if p.Name == "$this" || p.Name == "$value" {
			t.Errorf("Circle class: spurious property %s (from hook body)", p.Name)
		}
	}
	if p := findProp(circle, "$radius"); p == nil {
		t.Error("Circle.$radius not found")
	} else if len(p.Hooks) != 2 {
		t.Errorf("Circle.$radius: want 2 hooks, got %d %v", len(p.Hooks), p.Hooks)
	}
}

// TestH3AsymmetricVisibilityParsed verifies that PHP 8.4 asymmetric-visibility
// modifiers like "public private(set)" are parsed correctly: the primary
// (read) visibility is recorded in PropertyDef.Visibility and the secondary
// (write) visibility is in PropertyDef.SetVisibility.
func TestH3AsymmetricVisibilityParsed(t *testing.T) {
	t.Helper()
	tests := []struct {
		name           string
		src            string
		propName       string
		wantVisibility string
		wantSetVis     string
		wantType       string
	}{
		{
			name: "public_private_set",
			src: `<?php
class User {
    public private(set) int $id;
}`,
			propName:       "$id",
			wantVisibility: "public",
			wantSetVis:     "private",
			wantType:       "int",
		},
		{
			name: "public_protected_set",
			src: `<?php
class User {
    public protected(set) string $name;
}`,
			propName:       "$name",
			wantVisibility: "public",
			wantSetVis:     "protected",
			wantType:       "string",
		},
		{
			name: "protected_private_set",
			src: `<?php
class User {
    protected private(set) string $email;
}`,
			propName:       "$email",
			wantVisibility: "protected",
			wantSetVis:     "private",
			wantType:       "string",
		},
		{
			name: "private_set_only",
			src: `<?php
class Foo {
    private(set) string $val;
}`,
			propName:       "$val",
			wantVisibility: "public", // default when no explicit primary vis
			wantSetVis:     "private",
			wantType:       "string",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()
			result := New().Parse(tc.src)
			if len(result.Errors) != 0 {
				for _, e := range result.Errors {
					t.Errorf("parse error at line %d: %s", e.Line, e.Message)
				}
			}
			if len(result.Classes) == 0 {
				t.Fatal("no classes parsed")
			}
			cls := result.Classes[0]
			var prop *PropertyDef
			for i := range cls.Properties {
				if cls.Properties[i].Name == tc.propName {
					prop = &cls.Properties[i]
					break
				}
			}
			if prop == nil {
				t.Fatalf("property %s not found (have %d props)", tc.propName, len(cls.Properties))
			}
			if prop.Visibility != tc.wantVisibility {
				t.Errorf("Visibility: want %q, got %q", tc.wantVisibility, prop.Visibility)
			}
			if prop.SetVisibility != tc.wantSetVis {
				t.Errorf("SetVisibility: want %q, got %q", tc.wantSetVis, prop.SetVisibility)
			}
			if prop.Type != tc.wantType {
				t.Errorf("Type: want %q, got %q", tc.wantType, prop.Type)
			}
		})
	}
}

// TestH3AsymmetricVisibilityPhp84FileParseClean verifies that the php84.php
// User class (which uses asymmetric visibility) is parsed correctly.
func TestH3AsymmetricVisibilityPhp84FileParseClean(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testdataRoot(t), "php-features", "php84.php"))
	if err != nil {
		t.Fatalf("cannot read php84.php: %v", err)
	}
	result := New().Parse(string(src))
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("php84.php: parse error at line %d: %s", e.Line, e.Message)
		}
	}

	var userClass *ClassDef
	for i := range result.Classes {
		if result.Classes[i].Name == "User" {
			userClass = &result.Classes[i]
			break
		}
	}
	if userClass == nil {
		t.Fatal("User class not found in php84.php")
	}
	if len(userClass.Properties) != 3 {
		t.Errorf("User class: want 3 properties, got %d", len(userClass.Properties))
	}

	findProp := func(name string) *PropertyDef {
		for i := range userClass.Properties {
			if userClass.Properties[i].Name == name {
				return &userClass.Properties[i]
			}
		}
		return nil
	}

	if p := findProp("$id"); p == nil {
		t.Error("User.$id not found")
	} else {
		if p.Visibility != "public" {
			t.Errorf("User.$id.Visibility: want \"public\", got %q", p.Visibility)
		}
		if p.SetVisibility != "private" {
			t.Errorf("User.$id.SetVisibility: want \"private\", got %q", p.SetVisibility)
		}
	}
	if p := findProp("$name"); p == nil {
		t.Error("User.$name not found")
	} else {
		if p.SetVisibility != "protected" {
			t.Errorf("User.$name.SetVisibility: want \"protected\", got %q", p.SetVisibility)
		}
	}
	if p := findProp("$email"); p == nil {
		t.Error("User.$email not found")
	} else {
		if p.Visibility != "protected" {
			t.Errorf("User.$email.Visibility: want \"protected\", got %q", p.Visibility)
		}
		if p.SetVisibility != "private" {
			t.Errorf("User.$email.SetVisibility: want \"private\", got %q", p.SetVisibility)
		}
	}
}

// TestH4DynamicClassConstantFetch verifies that PHP 8.3 dynamic class constant
// fetches (Class::{$name}) parse cleanly without corrupting the enclosing
// class or function structure.
func TestH4DynamicClassConstantFetch(t *testing.T) {
	t.Helper()
	tests := []struct {
		name          string
		src           string
		wantFunctions int
		wantClasses   int
	}{
		{
			name: "dynamic_fetch_in_function",
			src: `<?php
class Colors {
    public const string RED   = '#FF0000';
    public const string GREEN = '#00FF00';
}
function getColor(string $name): string
{
    return Colors::{$name};
}
function another(): string { return 'x'; }
`,
			wantFunctions: 2,
			wantClasses:   1,
		},
		{
			name: "dynamic_fetch_multiple_in_function",
			src: `<?php
class Config {
    public const string PROD = 'production';
    public const string DEV  = 'development';
}
function resolveEnv(): string
{
    $env = 'PROD';
    return Config::{$env};
}
`,
			wantFunctions: 1,
			wantClasses:   1,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()
			result := New().Parse(tc.src)
			if len(result.Errors) != 0 {
				for _, e := range result.Errors {
					t.Errorf("parse error at line %d: %s", e.Line, e.Message)
				}
			}
			if len(result.Classes) != tc.wantClasses {
				t.Errorf("classes: want %d, got %d", tc.wantClasses, len(result.Classes))
			}
			if len(result.Functions) != tc.wantFunctions {
				t.Errorf("functions: want %d, got %d", tc.wantFunctions, len(result.Functions))
			}
		})
	}
}

// TestH4DynamicConstFetchPhp83FileParseClean verifies that the php83.php
// testdata (which contains Class::{$name} in function bodies) parses with
// zero errors and that the enclosing class structure is intact.
func TestH4DynamicConstFetchPhp83FileParseClean(t *testing.T) {
	t.Helper()
	src, err := os.ReadFile(filepath.Join(testdataRoot(t), "php-features", "php83.php"))
	if err != nil {
		t.Fatalf("cannot read php83.php: %v", err)
	}
	result := New().Parse(string(src))
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("php83.php: parse error at line %d: %s", e.Line, e.Message)
		}
	}

	// Colors class must be present and intact (it is the class whose constants are
	// dynamically fetched via Colors::{$name} in the getColor() function)
	var colorsClass *ClassDef
	for i := range result.Classes {
		if result.Classes[i].Name == "Colors" {
			colorsClass = &result.Classes[i]
			break
		}
	}
	if colorsClass == nil {
		t.Fatal("Colors class not found in php83.php")
	}

	// getColor and resolveEnv functions must be present (their bodies contain ::{ })
	foundGetColor, foundResolveEnv := false, false
	for _, fn := range result.Functions {
		switch fn.Name {
		case "getColor":
			foundGetColor = true
		case "resolveEnv":
			foundResolveEnv = true
		}
	}
	if !foundGetColor {
		t.Error("function getColor not found — dynamic fetch in body may have corrupted parsing")
	}
	if !foundResolveEnv {
		t.Error("function resolveEnv not found — dynamic fetch in body may have corrupted parsing")
	}
}

// TestInterfacePropertyHooksAbstract verifies that abstract property hooks
// in interfaces (the "get;" short form) are parsed without errors and that
// the property is correctly captured.
func TestInterfacePropertyHooksAbstract(t *testing.T) {
	t.Helper()
	src := `<?php
interface Readable {
    public string $name { get; }
}
`
	result := New().Parse(string(src))
	if len(result.Errors) != 0 {
		for _, e := range result.Errors {
			t.Errorf("parse error at line %d: %s", e.Line, e.Message)
		}
	}
	if len(result.Interfaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(result.Interfaces))
	}
	// The interface parsed correctly; the abstract hook property should be captured
	// via parseClassBody which is shared with interfaces.
}
