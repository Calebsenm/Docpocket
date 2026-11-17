package collections

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDocumentEncodeDecodePreservesNumbers(t *testing.T) {
	raw := []byte(`{"id":"01JC8Z3X7Q0N5K9M2R4T6V8WYB","n":9007199254740993,"nested":{"x":1.5},"tags":["a"]}`)
	d, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d["n"].(json.Number); !ok {
		t.Fatalf("expected json.Number, got %T", d["n"])
	}
	out, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "9007199254740993") {
		t.Fatalf("number precision lost: %s", out)
	}
}

func TestClockFormatSorts(t *testing.T) {
	base := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	a := FormatTime(base)
	b := FormatTime(base.Add(time.Millisecond))
	c := FormatTime(base.Add(time.Hour))
	if !(a < b && b < c) {
		t.Fatalf("timestamps do not sort: %q %q %q", a, b, c)
	}
	if _, err := ParseTime(a); err != nil {
		t.Fatal(err)
	}
}

func TestGetPath(t *testing.T) {
	d := Document{
		"author": map[string]any{
			"name": "Ana",
			"meta": map[string]any{"age": json.Number("30")},
		},
		"tags": []any{"go"},
	}
	if got := GetPath(d, "author.name"); got != "Ana" {
		t.Fatalf("got %v", got)
	}
	if got := GetPath(d, "author.meta.age"); got != json.Number("30") {
		t.Fatalf("got %v", got)
	}
	if got := GetPath(d, "author.missing.deep"); got != nil {
		t.Fatalf("got %v", got)
	}
	if got := GetPath(d, "tags.0"); got != nil {
		t.Fatalf("array indexing is not supported, got %v", got)
	}
}

func TestSetDeletePath(t *testing.T) {
	d := Document{}
	SetPath(d, "a.b.c", 1)
	if got := GetPath(d, "a.b.c"); got != 1 {
		t.Fatalf("got %v", got)
	}
	DeletePath(d, "a.b.c")
	if got := GetPath(d, "a.b.c"); got != nil {
		t.Fatalf("expected deleted, got %v", got)
	}
}

func TestSanitize(t *testing.T) {
	d := Document{"id": "x", "passwordHash": "secret", "tokenKey": "k", "email": "a@b.c"}
	s := d.Sanitize()
	for _, key := range []string{"passwordHash", "tokenKey"} {
		if _, ok := s[key]; ok {
			t.Fatalf("%s leaked", key)
		}
	}
	if _, ok := d["passwordHash"]; !ok {
		t.Fatal("Sanitize mutated the original document")
	}
}

func TestCollectionValidation(t *testing.T) {
	good := &CollectionDef{
		Name: "posts",
		Fields: []Field{
			{Name: "title", Type: "string", Required: true},
			{Name: "author.name", Type: "string"},
		},
		Indexes: []Index{{Name: "title_idx", Field: "title"}, {Name: "views_idx", Field: "views", Unique: false}},
		Rules:   Rules{"list": "", "view": "true"},
	}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}

	bad := []*CollectionDef{
		{Name: ""},
		{Name: "1bad"},
		{Name: "posts", Type: "nope"},
		{Name: "posts", Fields: []Field{{Name: "a", Type: "nope"}}},
		{Name: "posts", Fields: []Field{{Name: "../a", Type: "string"}}},
		{Name: "posts", Indexes: []Index{{Name: "i", Field: "a", State: "weird"}}},
		{Name: "posts", Indexes: []Index{{Name: "i", Field: "a"}, {Name: "i", Field: "b"}}},
		{Name: "posts", Rules: Rules{"nope": ""}},
	}
	for i, def := range bad {
		if err := def.Validate(); err == nil {
			t.Fatalf("case %d should have failed: %+v", i, def)
		}
	}
}

func TestValidateDocument(t *testing.T) {
	def := &CollectionDef{
		Name: "posts",
		Fields: []Field{
			{Name: "title", Type: "string", Required: true, MaxLen: 5},
			{Name: "views", Type: "number"},
			{Name: "tags", Type: "array"},
			{Name: "ok", Type: "bool"},
		},
	}
	base := Document{FieldID: NewID(), FieldCreated: Now(), FieldUpdated: Now()}

	if err := def.ValidateDocument(base); err == nil {
		t.Fatal("missing required field should fail")
	}

	d := base.Clone()
	d["title"] = "hello"
	d["views"] = json.Number("3")
	d["tags"] = []any{"a"}
	d["ok"] = true
	if err := def.ValidateDocument(d); err != nil {
		t.Fatal(err)
	}

	d["title"] = "toolong"
	if err := def.ValidateDocument(d); err == nil {
		t.Fatal("max length should fail")
	}
	d["title"] = "ok"
	d["views"] = "three"
	if err := def.ValidateDocument(d); err == nil {
		t.Fatal("wrong type should fail")
	}
}

func TestMergePatch(t *testing.T) {
	dst := Document{"a": 1, "keep": true, "obj": map[string]any{"x": 1, "y": 2}}
	patch := Document{"a": nil, "new": "v", "obj": map[string]any{"y": nil, "z": 3}}
	MergePatch(dst, patch)
	if _, ok := dst["a"]; ok {
		t.Fatal("null should delete")
	}
	if dst["keep"] != true || dst["new"] != "v" {
		t.Fatalf("unexpected %v", dst)
	}
	obj := dst["obj"].(map[string]any)
	if obj["x"] != 1 || obj["z"] != 3 {
		t.Fatalf("nested merge failed: %v", obj)
	}
	if _, ok := obj["y"]; ok {
		t.Fatal("nested null should delete")
	}
}

func TestAuthCollectionDefaults(t *testing.T) {
	def := &CollectionDef{Name: "users", Type: TypeAuth}
	def.Normalize()
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
	if def.Rule("list") == "" || def.Rule("view") == "" {
		t.Fatal("auth collection should get owner-based defaults")
	}
	for _, f := range []string{FieldEmail, FieldPasswordHash, FieldTokenKey, FieldVerified} {
		found := false
		for _, field := range def.Fields {
			if field.Name == f {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing system field %q", f)
		}
	}
	// Explicit public rules must be respected.
	def2 := &CollectionDef{Name: "open", Type: TypeAuth, Rules: Rules{"list": ""}}
	def2.Normalize()
	if rule, ok := def2.RuleSet("list"); !ok || rule != "" {
		t.Fatal("explicit public list rule was overwritten")
	}
}
