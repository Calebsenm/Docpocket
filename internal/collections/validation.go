package collections

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ValidationError describes a document that does not satisfy its collection
// schema. It maps to HTTP 422 in the API.
type ValidationError struct {
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("validation failed for %q: %s", e.Field, e.Message)
	}
	return "validation failed: " + e.Message
}

// ValidateDocument checks system fields and the optional collection schema.
func (c *CollectionDef) ValidateDocument(d Document) error {
	if d == nil {
		return &ValidationError{Message: "document must be a JSON object"}
	}
	id := d.ID()
	if id == "" {
		return &ValidationError{Field: FieldID, Message: "id is required"}
	}
	if !ValidID(id) {
		return &ValidationError{Field: FieldID, Message: "id must be a ULID"}
	}
	if _, ok := d[FieldCreated].(string); !ok {
		return &ValidationError{Field: FieldCreated, Message: "created timestamp is required"}
	}
	if _, ok := d[FieldUpdated].(string); !ok {
		return &ValidationError{Field: FieldUpdated, Message: "updated timestamp is required"}
	}
	if len(c.Fields) == 0 {
		return nil
	}
	for _, f := range c.Fields {
		v := GetPath(d, f.Name)
		if v == nil {
			if f.Required {
				return &ValidationError{Field: f.Name, Message: "field is required"}
			}
			continue
		}
		if err := validateFieldType(f, v); err != nil {
			return err
		}
	}
	return nil
}

func validateFieldType(f Field, v any) error {
	switch f.Type {
	case "", "any":
		return nil
	case "string":
		s, ok := v.(string)
		if !ok {
			return &ValidationError{Field: f.Name, Message: "expected string"}
		}
		if f.MaxLen > 0 && utf8.RuneCountInString(s) > f.MaxLen {
			return &ValidationError{Field: f.Name, Message: fmt.Sprintf("maximum length is %d characters", f.MaxLen)}
		}
	case "number":
		if _, ok := numericValue(v); !ok {
			return &ValidationError{Field: f.Name, Message: "expected number"}
		}
	case "bool":
		if _, ok := v.(bool); !ok {
			return &ValidationError{Field: f.Name, Message: "expected boolean"}
		}
	case "array":
		if _, ok := v.([]any); !ok {
			return &ValidationError{Field: f.Name, Message: "expected array"}
		}
	case "object":
		if _, ok := v.(map[string]any); !ok {
			if _, ok := v.(Document); !ok {
				return &ValidationError{Field: f.Name, Message: "expected object"}
			}
		}
	case "file":
		if s, ok := v.(string); !ok || strings.TrimSpace(s) == "" {
			return &ValidationError{Field: f.Name, Message: "expected file name string"}
		}
	default:
		return &ValidationError{Field: f.Name, Message: "unknown field type " + f.Type}
	}
	return nil
}

func numericValue(v any) (float64, bool) {
	switch x := v.(type) {
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}

// MergePatch applies a shallow RFC 7396-style merge patch: an explicit null
// removes a field, other values replace the previous value. Nested objects are
// merged recursively, which is convenient for PATCH operations.
func MergePatch(dst Document, patch Document) {
	for k, v := range patch {
		if v == nil {
			delete(dst, k)
			continue
		}
		pm, pok := v.(map[string]any)
		dm, dok := dst[k].(map[string]any)
		if pok && dok {
			MergePatch(Document(dm), Document(pm))
			continue
		}
		dst[k] = v
	}
}
