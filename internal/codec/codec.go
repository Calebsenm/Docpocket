// Package codec implements the order-preserving key encoding used by the
// storage engine for secondary indexes.
//
// The encoding guarantees that for two scalar values a and b of the same type:
//
//	a < b  =>  EncodeIndexValue(a) < EncodeIndexValue(b)
//
// byte-by-byte. Values of different types are ordered by their type tag:
// null < false < true < number < string.
package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"

	"encoding/json"
)

// ErrNotIndexable is returned when a value cannot be used as an index key
// (objects and nested arrays are not indexable).
var ErrNotIndexable = errors.New("codec: value is not indexable")

// ErrValueTooLarge is returned when a single encoded key would exceed the
// safe bbolt key limit. Bbolt rejects keys larger than approximately 32 KiB.
var ErrValueTooLarge = errors.New("codec: encoded key is too large")

// MaxKeySize is a conservative upper bound for a complete index key
// (encoded value + document id). Bbolt's hard limit is about 32 KiB.
const MaxKeySize = 30 * 1024

// Type tags. Keep the numeric order in sync with the documented ordering.
const (
	tagNull   byte = 0x00
	tagFalse  byte = 0x01
	tagTrue   byte = 0x02
	tagNumber byte = 0x03
	tagString byte = 0x04
)

// TypeTag returns the ordering tag for an indexable scalar value.
func TypeTag(v any) (byte, error) {
	switch v.(type) {
	case nil:
		return tagNull, nil
	case bool:
		return tagFalse, nil // refined below
	case json.Number:
		return tagNumber, nil
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return tagNumber, nil
	case string:
		return tagString, nil
	default:
		return 0, fmt.Errorf("%w: %T", ErrNotIndexable, v)
	}
}

// EncodeIndexValue encodes a single scalar value as [tag][payload].
func EncodeIndexValue(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return []byte{tagNull}, nil
	case bool:
		if x {
			return []byte{tagTrue}, nil
		}
		return []byte{tagFalse}, nil
	case json.Number:
		f, err := x.Float64()
		if err != nil {
			return nil, fmt.Errorf("codec: invalid number %q: %w", x.String(), err)
		}
		return encodeNumber(f), nil
	case float64:
		return encodeNumber(x), nil
	case float32:
		return encodeNumber(float64(x)), nil
	case int:
		return encodeNumber(float64(x)), nil
	case int8:
		return encodeNumber(float64(x)), nil
	case int16:
		return encodeNumber(float64(x)), nil
	case int32:
		return encodeNumber(float64(x)), nil
	case int64:
		return encodeNumber(float64(x)), nil
	case uint:
		return encodeNumber(float64(x)), nil
	case uint8:
		return encodeNumber(float64(x)), nil
	case uint16:
		return encodeNumber(float64(x)), nil
	case uint32:
		return encodeNumber(float64(x)), nil
	case uint64:
		return encodeNumber(float64(x)), nil
	case string:
		return EncodeString(x), nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrNotIndexable, v)
	}
}

func encodeNumber(f float64) []byte {
	out := make([]byte, 9)
	out[0] = tagNumber
	bits := math.Float64bits(f)
	if bits&(1<<63) != 0 {
		// Negative numbers: flip every bit so that more negative values sort first.
		bits = ^bits
	} else {
		// Positive numbers: flip the sign bit so they sort after negatives.
		bits ^= 1 << 63
	}
	binary.BigEndian.PutUint64(out[1:], bits)
	return out
}

// EncodeString encodes a string as [tag][escaped bytes][terminator] so that
// byte-wise comparison matches lexical comparison and no encoded value is a
// prefix of another.
//
// NUL bytes are escaped as 0x00 0xFF and the string is terminated with
// 0x00 0x01. This makes "ab" sort before "abc" and keeps a value safely
// separated from the document id appended to non-unique index keys.
func EncodeString(s string) []byte {
	out := make([]byte, 0, len(s)+3)
	out = append(out, tagString)
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b == 0x00 {
			out = append(out, 0x00, 0xFF)
			continue
		}
		out = append(out, b)
	}
	out = append(out, 0x00, 0x01)
	return out
}

// EncodeIndexValues expands a document value into one or more encoded index
// values. Arrays are multi-key: one entry per scalar element. An empty array
// produces no entries. A missing value is encoded exactly like null.
func EncodeIndexValues(v any) ([][]byte, error) {
	switch x := v.(type) {
	case []any:
		if len(x) == 0 {
			return nil, nil
		}
		out := make([][]byte, 0, len(x))
		for _, el := range x {
			enc, err := EncodeIndexValue(el)
			if err != nil {
				return nil, fmt.Errorf("array element: %w", err)
			}
			out = append(out, enc)
		}
		// Dedupe exact duplicates to avoid redundant index writes.
		sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
		uniq := out[:0]
		for _, e := range out {
			if len(uniq) == 0 || string(uniq[len(uniq)-1]) != string(e) {
				uniq = append(uniq, e)
			}
		}
		return uniq, nil
	case []string:
		out := make([][]byte, 0, len(x))
		for _, el := range x {
			out = append(out, EncodeString(el))
		}
		sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
		return out, nil
	default:
		enc, err := EncodeIndexValue(v)
		if err != nil {
			return nil, err
		}
		return [][]byte{enc}, nil
	}
}

// EncodeKey builds a non-unique index key: [value][id].
func EncodeKey(v any, id []byte) ([]byte, error) {
	enc, err := EncodeIndexValue(v)
	if err != nil {
		return nil, err
	}
	if len(id) != 16 {
		return nil, fmt.Errorf("codec: document id must be 16 bytes, got %d", len(id))
	}
	key := make([]byte, 0, len(enc)+len(id))
	key = append(key, enc...)
	key = append(key, id...)
	if len(key) > MaxKeySize {
		return nil, ErrValueTooLarge
	}
	return key, nil
}

// EncodeUniqueKey builds a unique index key: [value].
func EncodeUniqueKey(v any) ([]byte, error) {
	enc, err := EncodeIndexValue(v)
	if err != nil {
		return nil, err
	}
	if len(enc) > MaxKeySize {
		return nil, ErrValueTooLarge
	}
	return enc, nil
}

// PrefixRange returns the half-open range [prefix, end) covering every key
// that starts with prefix. A nil end means "to the end of the bucket".
func PrefixRange(prefix []byte) (start, end []byte) {
	start = append([]byte(nil), prefix...)
	end = PrefixSuccessor(prefix)
	return start, end
}

// PrefixSuccessor returns the smallest key greater than every key having the
// given prefix. It returns nil when no finite upper bound exists (all 0xFF).
func PrefixSuccessor(prefix []byte) []byte {
	out := append([]byte(nil), prefix...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] != 0xFF {
			out[i]++
			return out[:i+1]
		}
	}
	return nil
}

// Number converts a JSON-style numeric value to float64.
func Number(v any) (float64, bool) {
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
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

// Compare orders two scalar values consistently with EncodeIndexValue.
// It returns -1, 0 or 1. Values of incomparable types are ordered by type tag.
func Compare(a, b any) int {
	ta, ea := TypeTag(a)
	tb, eb := TypeTag(b)
	if ea != nil || eb != nil {
		// Fall back to a deterministic string representation for non-indexable
		// values; callers normally validate before reaching this point.
		return compareStrings(fmt.Sprintf("%T:%v", a, a), fmt.Sprintf("%T:%v", b, b))
	}
	if a == nil || b == nil {
		if a == nil && b == nil {
			return 0
		}
		if a == nil {
			return -1
		}
		return 1
	}
	if ta == tagTrue || ta == tagFalse {
		ta = boolTag(a.(bool))
	}
	if tb == tagTrue || tb == tagFalse {
		tb = boolTag(b.(bool))
	}
	if ta != tb {
		if ta < tb {
			return -1
		}
		return 1
	}
	switch ta {
	case tagNull:
		return 0
	case tagFalse, tagTrue:
		bo, _ := a.(bool)
		co, _ := b.(bool)
		if bo == co {
			return 0
		}
		if !bo {
			return -1
		}
		return 1
	case tagNumber:
		af, _ := Number(a)
		bf, _ := Number(b)
		if af < bf {
			return -1
		}
		if af > bf {
			return 1
		}
		return 0
	case tagString:
		as, _ := a.(string)
		bs, _ := b.(string)
		return compareStrings(as, bs)
	default:
		return 0
	}
}

func boolTag(v bool) byte {
	if v {
		return tagTrue
	}
	return tagFalse
}

func compareStrings(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// KeyString renders an encoded key for debugging and test failure messages.
func KeyString(key []byte) string {
	if len(key) == 0 {
		return ""
	}
	b := make([]byte, 0, len(key)*4)
	for _, c := range key {
		if c >= 0x20 && c < 0x7F {
			b = append(b, c)
			continue
		}
		b = append(b, '\\', 'x')
		b = append(b, "0123456789abcdef"[c>>4], "0123456789abcdef"[c&0xF])
	}
	return string(b)
}

// SprintfKey is a small helper used in tests and error messages.
func SprintfKey(v any) string {
	enc, err := EncodeIndexValue(v)
	if err != nil {
		return "<err:" + err.Error() + ">"
	}
	return KeyString(enc)
}
