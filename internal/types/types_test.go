package types

import (
	"testing"
)

func TestParseArrayShapeBasic(t *testing.T) {
	fields := ParseArrayShape("array{name: string, age: int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Key != "name" || fields[0].Type != "string" {
		t.Errorf("field 0: got %+v", fields[0])
	}
	if fields[1].Key != "age" || fields[1].Type != "int" {
		t.Errorf("field 1: got %+v", fields[1])
	}
}

func TestParseArrayShapeOptional(t *testing.T) {
	fields := ParseArrayShape("array{name: string, address?: string}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Optional {
		t.Error("name should not be optional")
	}
	if !fields[1].Optional || fields[1].Key != "address" {
		t.Errorf("address should be optional, got %+v", fields[1])
	}
}

func TestParseArrayShapeNested(t *testing.T) {
	fields := ParseArrayShape("array{user: array{name: string, age: int}, count: int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Key != "user" || fields[0].Type != "array{name: string, age: int}" {
		t.Errorf("field 0: got %+v", fields[0])
	}
	if fields[1].Key != "count" || fields[1].Type != "int" {
		t.Errorf("field 1: got %+v", fields[1])
	}
}

func TestParseArrayShapeNullable(t *testing.T) {
	fields := ParseArrayShape("?array{key: string}")
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].Key != "key" {
		t.Errorf("expected key 'key', got %q", fields[0].Key)
	}
}

func TestParseArrayShapeInUnion(t *testing.T) {
	fields := ParseArrayShape("string|array{key: int}|null")
	if len(fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(fields))
	}
	if fields[0].Key != "key" || fields[0].Type != "int" {
		t.Errorf("got %+v", fields[0])
	}
}

func TestParseArrayShapeNotAShape(t *testing.T) {
	tests := []string{"string", "array", "int|string", "array<string, int>", ""}
	for _, s := range tests {
		if fields := ParseArrayShape(s); fields != nil {
			t.Errorf("ParseArrayShape(%q) should return nil, got %v", s, fields)
		}
	}
}

func TestParseArrayShapeGenericTypes(t *testing.T) {
	fields := ParseArrayShape("array{items: Collection<User>, total: int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Type != "Collection<User>" {
		t.Errorf("expected Collection<User>, got %q", fields[0].Type)
	}
}

func TestParseArrayShapeQuotedKeys(t *testing.T) {
	fields := ParseArrayShape("array{'content-type': string, 'x-api-key': string}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	if fields[0].Key != "content-type" {
		t.Errorf("expected 'content-type', got %q", fields[0].Key)
	}
}

func TestParseArrayShapePositional(t *testing.T) {
	fields := ParseArrayShape("array{string, int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
	// Positional fields have no key
	if fields[0].Key != "" || fields[0].Type != "string" {
		t.Errorf("field 0: got %+v", fields[0])
	}
}

func TestParseArrayShapeList(t *testing.T) {
	fields := ParseArrayShape("list{string, int}")
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(fields))
	}
}

func TestExtractDocTypeString(t *testing.T) {
	tests := []struct {
		input    string
		wantType string
		wantRest string
	}{
		{"string $name", "string", "$name"},
		{"array{name: string, age: int} $config desc", "array{name: string, age: int}", "$config desc"},
		{"?array{key: string} $var", "?array{key: string}", "$var"},
		{"Collection<User> $users", "Collection<User>", "$users"},
		{"int", "int", ""},
		{"", "", ""},
		{"array<string, array{id: int}> $data", "array<string, array{id: int}>", "$data"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			typ, rest := ExtractDocTypeString(tt.input)
			if typ != tt.wantType {
				t.Errorf("type = %q, want %q", typ, tt.wantType)
			}
			if rest != tt.wantRest {
				t.Errorf("rest = %q, want %q", rest, tt.wantRest)
			}
		})
	}
}
