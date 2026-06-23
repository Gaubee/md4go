package parser

import (
	"testing"

	"md4go/ast"
)

func TestBuildAttribute_Trivial(t *testing.T) {
	// No backslash, ampersand, or NULL → trivial attribute
	attr := BuildAttribute([]byte("hello world"), 0)
	if !attr.IsTrivial() {
		t.Errorf("expected trivial attribute")
	}
	if string(attr.Text) != "hello world" {
		t.Errorf("text = %q, want %q", attr.Text, "hello world")
	}
	if len(attr.SubTypes) != 1 || attr.SubTypes[0] != ast.SubstrNormal {
		t.Errorf("expected single NORMAL substring")
	}
	if len(attr.SubOffsets) != 2 || attr.SubOffsets[0] != 0 || attr.SubOffsets[1] != 11 {
		t.Errorf("offsets = %v, want [0, 11]", attr.SubOffsets)
	}
}

func TestBuildAttribute_Empty(t *testing.T) {
	attr := BuildAttribute(nil, 0)
	if len(attr.Text) != 0 {
		t.Errorf("expected empty text")
	}
}

func TestBuildAttribute_Entity(t *testing.T) {
	// Named entity
	attr := BuildAttribute([]byte("foo &amp; bar"), 0)
	if string(attr.Text) != "foo &amp; bar" {
		t.Errorf("text = %q, want %q", attr.Text, "foo &amp; bar")
	}
	// Should have 3 substrings: NORMAL("foo "), ENTITY("&amp;"), NORMAL(" bar")
	if len(attr.SubTypes) != 3 {
		t.Fatalf("expected 3 substrings, got %d", len(attr.SubTypes))
	}
	if attr.SubTypes[0] != ast.SubstrNormal {
		t.Errorf("sub[0] type = %v, want Normal", attr.SubTypes[0])
	}
	if attr.SubTypes[1] != ast.SubstrEntity {
		t.Errorf("sub[1] type = %v, want Entity", attr.SubTypes[1])
	}
	if attr.SubTypes[2] != ast.SubstrNormal {
		t.Errorf("sub[2] type = %v, want Normal", attr.SubTypes[2])
	}
	// Verify substring text
	if string(attr.Text[attr.SubOffsets[0]:attr.SubOffsets[1]]) != "foo " {
		t.Errorf("sub[0] text = %q", attr.Text[attr.SubOffsets[0]:attr.SubOffsets[1]])
	}
	if string(attr.Text[attr.SubOffsets[1]:attr.SubOffsets[2]]) != "&amp;" {
		t.Errorf("sub[1] text = %q", attr.Text[attr.SubOffsets[1]:attr.SubOffsets[2]])
	}
	if string(attr.Text[attr.SubOffsets[2]:attr.SubOffsets[3]]) != " bar" {
		t.Errorf("sub[2] text = %q", attr.Text[attr.SubOffsets[2]:attr.SubOffsets[3]])
	}
}

func TestBuildAttribute_NumericEntity(t *testing.T) {
	// Decimal numeric entity
	attr := BuildAttribute([]byte("&#65;"), 0)
	if len(attr.SubTypes) != 1 || attr.SubTypes[0] != ast.SubstrEntity {
		t.Errorf("expected single ENTITY substring, got %v", attr.SubTypes)
	}
	if string(attr.Text) != "&#65;" {
		t.Errorf("text = %q, want %q", attr.Text, "&#65;")
	}
}

func TestBuildAttribute_HexEntity(t *testing.T) {
	// Hex numeric entity
	attr := BuildAttribute([]byte("&#x41;"), 0)
	if len(attr.SubTypes) != 1 || attr.SubTypes[0] != ast.SubstrEntity {
		t.Errorf("expected single ENTITY substring, got %v", attr.SubTypes)
	}
}

func TestBuildAttribute_InvalidEntityNotEntity(t *testing.T) {
	// & without valid entity → treated as normal text
	attr := BuildAttribute([]byte("foo & bar"), 0)
	if !attr.IsTrivial() {
		t.Errorf("expected trivial attribute for non-entity ampersand")
	}
	// Actually, "foo & bar" has no backslash, but has '&', so is_trivial=false
	// But since "& bar" is not a valid entity, it should be all NORMAL
	for i, st := range attr.SubTypes {
		if st != ast.SubstrNormal {
			t.Errorf("sub[%d] type = %v, want Normal", i, st)
		}
	}
	if string(attr.Text) != "foo & bar" {
		t.Errorf("text = %q, want %q", attr.Text, "foo & bar")
	}
}

func TestBuildAttribute_BackslashEscape(t *testing.T) {
	// \! → ! (backslash escape resolved)
	attr := BuildAttribute([]byte(`foo \! bar`), 0)
	if string(attr.Text) != "foo ! bar" {
		t.Errorf("text = %q, want %q", attr.Text, "foo ! bar")
	}
}

func TestBuildAttribute_NoEscapes(t *testing.T) {
	// With noEscapes flag, backslash is NOT resolved
	attr := BuildAttribute([]byte(`foo \! bar`), buildAttrNoEscapes)
	if string(attr.Text) != `foo \! bar` {
		t.Errorf("text = %q, want %q", attr.Text, `foo \! bar`)
	}
}

func TestBuildAttribute_NullChar(t *testing.T) {
	// NULL character → NULLCHAR substring
	input := []byte{'f', 'o', 'o', 0, 'b', 'a', 'r'}
	attr := BuildAttribute(input, 0)
	if len(attr.SubTypes) != 3 {
		t.Fatalf("expected 3 substrings, got %d", len(attr.SubTypes))
	}
	if attr.SubTypes[0] != ast.SubstrNormal {
		t.Errorf("sub[0] type = %v, want Normal", attr.SubTypes[0])
	}
	if attr.SubTypes[1] != ast.SubstrNullChar {
		t.Errorf("sub[1] type = %v, want NullChar", attr.SubTypes[1])
	}
	if attr.SubTypes[2] != ast.SubstrNormal {
		t.Errorf("sub[2] type = %v, want Normal", attr.SubTypes[2])
	}
	if string(attr.Text) != "foo\x00bar" {
		t.Errorf("text = %q", attr.Text)
	}
}

func TestBuildAttribute_Mixed(t *testing.T) {
	// Mix of entity, NULL char, and normal text
	input := []byte("a &amp; b\x00c")
	attr := BuildAttribute(input, 0)
	if len(attr.SubTypes) < 4 {
		t.Fatalf("expected at least 4 substrings, got %d", len(attr.SubTypes))
	}
	// Verify substring types
	types := []ast.SubstrType{ast.SubstrNormal, ast.SubstrEntity, ast.SubstrNormal, ast.SubstrNullChar, ast.SubstrNormal}
	if len(attr.SubTypes) != len(types) {
		t.Fatalf("expected %d substrings, got %d", len(types), len(attr.SubTypes))
	}
	for i, want := range types {
		if attr.SubTypes[i] != want {
			t.Errorf("sub[%d] type = %v, want %v", i, attr.SubTypes[i], want)
		}
	}
}

func TestBuildAttribute_MultipleEntities(t *testing.T) {
	attr := BuildAttribute([]byte("&amp;&amp;"), 0)
	if len(attr.SubTypes) != 2 {
		t.Fatalf("expected 2 ENTITY substrings, got %d", len(attr.SubTypes))
	}
	for i, st := range attr.SubTypes {
		if st != ast.SubstrEntity {
			t.Errorf("sub[%d] type = %v, want Entity", i, st)
		}
	}
}

func TestFindEntityEnd_NamedEntity(t *testing.T) {
	text := []byte("&amp; rest")
	end := findEntityEnd(text, 0, len(text))
	if end != 5 {
		t.Errorf("end = %d, want 5", end)
	}
}

func TestFindEntityEnd_InvalidEntity(t *testing.T) {
	// & without valid entity content
	text := []byte("& bar")
	end := findEntityEnd(text, 0, len(text))
	if end != 0 {
		t.Errorf("end = %d, want 0 (not a valid entity)", end)
	}
}

func TestFindEntityEnd_HexEntity(t *testing.T) {
	text := []byte("&#x41;")
	end := findEntityEnd(text, 0, len(text))
	if end != 6 {
		t.Errorf("end = %d, want 6", end)
	}
}

func TestFindEntityEnd_DecEntity(t *testing.T) {
	text := []byte("&#65;")
	end := findEntityEnd(text, 0, len(text))
	if end != 5 {
		t.Errorf("end = %d, want 5", end)
	}
}

func TestFindEntityEnd_TooLongHex(t *testing.T) {
	// Hex entity with > 6 digits → invalid
	text := []byte("&#x1234567;")
	end := findEntityEnd(text, 0, len(text))
	if end != 0 {
		t.Errorf("end = %d, want 0 (too long)", end)
	}
}

func TestFindEntityEnd_NamedTooShort(t *testing.T) {
	// Named entity with < 2 chars → invalid
	text := []byte("&a;")
	end := findEntityEnd(text, 0, len(text))
	if end != 0 {
		t.Errorf("end = %d, want 0 (too short)", end)
	}
}

func TestFindEntityEnd_NamedStartsWithDigit(t *testing.T) {
	// Named entity must start with a letter
	text := []byte("&1abc;")
	end := findEntityEnd(text, 0, len(text))
	if end != 0 {
		t.Errorf("end = %d, want 0 (starts with digit)", end)
	}
}
