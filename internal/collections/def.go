package collections

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Collection types.
const (
	TypeBase = "base"
	TypeAuth = "auth"
)

// Rule operations.
var RuleOps = []string{"list", "view", "create", "update", "delete"}

// Rules is a per-operation map of CEL expressions. An empty string or "true"
// means public; a missing/null rule is restricted to superusers.
type Rules map[string]string

// Field describes an optional schema field. Collections without fields accept
// arbitrary JSON documents.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // string | number | bool | array | object | file | any
	Required bool   `json:"required,omitempty"`
	MaxLen   int    `json:"maxLen,omitempty"`
}

// Index describes a single-field secondary index. Arrays are indexed using
// multi-key entries (one entry per element). Full-text indexes are indexed by
// token instead of by whole value.
type Index struct {
	Name     string `json:"name"`
	Field    string `json:"field"`
	Unique   bool   `json:"unique,omitempty"`
	Fulltext bool   `json:"fulltext,omitempty"`
	State    string `json:"state"` // building | ready
}

// SavedView is a named, reusable query definition (filter + sort) that users
// can save and share against a collection.
type SavedView struct {
	Name    string `json:"name"`
	Filter  string `json:"filter"`
	Sort    string `json:"sort"`
	OwnerID string `json:"ownerId,omitempty"`
	Shared  bool   `json:"shared,omitempty"`
}

// CascadeChild describes a child collection that references the parent
// document. When the parent is deleted, matching child documents are removed
// in the same Bbolt transaction (atomic cascade).
type CascadeChild struct {
	Collection string `json:"collection"`
	Field      string `json:"field"`
}

// CollectionDef is the persisted definition of a collection.
type CollectionDef struct {
	Name    string         `json:"name"`
	Type    string         `json:"type,omitempty"`
	Fields  []Field        `json:"fields,omitempty"`
	Indexes []Index        `json:"indexes,omitempty"`
	Rules   Rules          `json:"rules,omitempty"`
	Views   []SavedView    `json:"views,omitempty"`
	Cascade []CascadeChild `json:"cascade,omitempty"`
}

var namePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,63}$`)

// ValidName reports whether a collection or index name is acceptable.
func ValidName(s string) bool { return namePattern.MatchString(s) }

// Validate checks structural invariants of a collection definition.
func (c *CollectionDef) Validate() error {
	if c == nil {
		return fmt.Errorf("collection definition is nil")
	}
	if !ValidName(c.Name) {
		return fmt.Errorf("invalid collection name %q: use letters, digits and underscore (max 64)", c.Name)
	}
	if c.Type == "" {
		c.Type = TypeBase
	}
	if c.Type != TypeBase && c.Type != TypeAuth {
		return fmt.Errorf("invalid collection type %q", c.Type)
	}
	seenFields := map[string]bool{}
	for i := range c.Fields {
		f := &c.Fields[i]
		if !ValidPath(f.Name) {
			return fmt.Errorf("invalid field name %q", f.Name)
		}
		if seenFields[f.Name] {
			return fmt.Errorf("duplicate field %q", f.Name)
		}
		seenFields[f.Name] = true
		switch f.Type {
		case "string", "number", "bool", "array", "object", "file", "any":
		default:
			return fmt.Errorf("invalid type %q for field %q", f.Type, f.Name)
		}
		if f.MaxLen < 0 {
			return fmt.Errorf("field %q: maxLen cannot be negative", f.Name)
		}
	}
	seenIdx := map[string]bool{}
	for i := range c.Indexes {
		idx := &c.Indexes[i]
		if !ValidName(idx.Name) {
			return fmt.Errorf("invalid index name %q", idx.Name)
		}
		if seenIdx[idx.Name] {
			return fmt.Errorf("duplicate index %q", idx.Name)
		}
		seenIdx[idx.Name] = true
		if !ValidPath(idx.Field) {
			return fmt.Errorf("index %q: invalid field %q", idx.Name, idx.Field)
		}
		if idx.State == "" {
			idx.State = "building"
		}
		if idx.State != "building" && idx.State != "ready" {
			return fmt.Errorf("index %q: invalid state %q", idx.Name, idx.State)
		}
		if idx.Unique && idx.Field == FieldID {
			// The document id is already unique by construction.
			idx.Unique = false
		}
		if idx.Fulltext {
			// Full-text indexes are multi-key by nature.
			idx.Unique = false
		}
	}
	for op := range c.Rules {
		if !validRuleOp(op) {
			return fmt.Errorf("invalid rule operation %q", op)
		}
	}
	for i := range c.Cascade {
		cc := &c.Cascade[i]
		if !ValidName(cc.Collection) {
			return fmt.Errorf("cascade %q: invalid collection name", cc.Collection)
		}
		if !ValidPath(cc.Field) {
			return fmt.Errorf("cascade %q: invalid field %q", cc.Collection, cc.Field)
		}
	}
	return nil
}

func validRuleOp(op string) bool {
	for _, o := range RuleOps {
		if o == op {
			return true
		}
	}
	return false
}

// Normalize fills in sensible defaults and system fields.
func (c *CollectionDef) Normalize() {
	if c.Type == "" {
		c.Type = TypeBase
	}
	if c.Rules == nil {
		c.Rules = Rules{}
	}
	if c.Views == nil {
		c.Views = []SavedView{}
	}
	if c.Type == TypeAuth {
		ensureField(c, FieldEmail, "string", true)
		ensureField(c, FieldPasswordHash, "string", false)
		ensureField(c, FieldVerified, "bool", false)
		ensureField(c, FieldTokenKey, "string", false)
		if len(c.Rules) == 0 {
			c.Rules["list"] = "request.auth != null && resource.id == request.auth.id"
			c.Rules["view"] = "request.auth != null && resource.id == request.auth.id"
			c.Rules["create"] = ""
			c.Rules["update"] = "request.auth != null && resource.id == request.auth.id"
			c.Rules["delete"] = "request.auth != null && resource.id == request.auth.id"
		}
	}
	// Every collection can sort/filter by created and updated efficiently when
	// these indexes exist; we do not add them automatically to keep writes lean.
	for i := range c.Indexes {
		if c.Indexes[i].Name == "" {
			c.Indexes[i].Name = indexNameFor(c.Indexes[i].Field, c.Indexes)
		}
	}
}

func indexNameFor(field string, existing []Index) string {
	base := strings.NewReplacer(".", "_").Replace(field) + "_idx"
	name := base
	for n := 2; ; n++ {
		taken := false
		for _, idx := range existing {
			if idx.Name == name {
				taken = true
				break
			}
		}
		if !taken {
			return name
		}
		name = fmt.Sprintf("%s_%d", base, n)
	}
}

func ensureField(c *CollectionDef, name, typ string, required bool) {
	for i := range c.Fields {
		if c.Fields[i].Name == name {
			return
		}
	}
	c.Fields = append(c.Fields, Field{Name: name, Type: typ, Required: required})
}

// Rule returns the rule for an operation. Missing rules return "" only if the
// map holds an explicit empty string; callers should use RuleSet to distinguish
// "locked" from "public".
func (c *CollectionDef) Rule(op string) string {
	if c == nil || c.Rules == nil {
		return ""
	}
	return c.Rules[op]
}

// RuleSet reports whether a rule was configured for the operation.
func (c *CollectionDef) RuleSet(op string) (string, bool) {
	if c == nil || c.Rules == nil {
		return "", false
	}
	r, ok := c.Rules[op]
	return r, ok
}

// Index looks up an index by name.
func (c *CollectionDef) Index(name string) (*Index, bool) {
	for i := range c.Indexes {
		if c.Indexes[i].Name == name {
			return &c.Indexes[i], true
		}
	}
	return nil, false
}

// IndexForField returns a ready index for a field, preferring unique indexes.
func (c *CollectionDef) IndexForField(field string, readyOnly bool) (*Index, bool) {
	var best *Index
	for i := range c.Indexes {
		idx := &c.Indexes[i]
		if idx.Field != field {
			continue
		}
		if readyOnly && idx.State != "ready" {
			continue
		}
		if best == nil || (!best.Unique && idx.Unique) {
			best = idx
		}
	}
	return best, best != nil
}

// Clone returns a deep copy of the definition.
func (c *CollectionDef) Clone() *CollectionDef {
	if c == nil {
		return nil
	}
	raw, _ := json.Marshal(c)
	var out CollectionDef
	_ = json.Unmarshal(raw, &out)
	return &out
}

// SortedIndexes returns indexes sorted by name (deterministic for tests).
func (c *CollectionDef) SortedIndexes() []Index {
	out := append([]Index(nil), c.Indexes...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// FieldNames returns declared field names.
func (c *CollectionDef) FieldNames() []string {
	out := make([]string, 0, len(c.Fields))
	for _, f := range c.Fields {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}
