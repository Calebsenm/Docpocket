package query

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const maxCursorBytes = 8 * 1024

type cursorData struct {
	Type   string `json:"t"` // "idx", "id" or "sort"
	Key    []byte `json:"k,omitempty"`
	ID     string `json:"id,omitempty"`
	Values []any  `json:"v,omitempty"`
}

func encodeCursor(c cursorData) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (*cursorData, error) {
	if s == "" {
		return nil, nil
	}
	if len(s) > maxCursorBytes {
		return nil, fmt.Errorf("cursor is too large")
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding")
	}
	var c cursorData
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor payload")
	}
	switch c.Type {
	case "idx", "id", "sort":
	default:
		return nil, fmt.Errorf("invalid cursor type")
	}
	return &c, nil
}
