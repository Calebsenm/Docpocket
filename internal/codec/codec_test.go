package codec

import (
	"bytes"
	"encoding/json"
	"math"
	"math/rand"
	"sort"
	"testing"
)

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := EncodeIndexValue(v)
	if err != nil {
		t.Fatalf("EncodeIndexValue(%v): %v", v, err)
	}
	return b
}

func TestTypeOrdering(t *testing.T) {
	values := []any{
		nil,
		false,
		true,
		math.Inf(-1),
		-1e300,
		-1,
		-0.5,
		0.0,
		0.5,
		1.0,
		1e300,
		math.Inf(1),
		"",
		"\x00",
		"\x00a",
		"a",
		"aa",
		"ab",
		"abc",
		"b",
		"é",
	}
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			a := mustEncode(t, values[i])
			b := mustEncode(t, values[j])
			if bytes.Compare(a, b) >= 0 {
				t.Fatalf("expected %v < %v, got encoded %q >= %q", values[i], values[j], KeyString(a), KeyString(b))
			}
		}
	}
}

func TestNumberOrdering(t *testing.T) {
	nums := []float64{
		math.Inf(-1), -math.MaxFloat64, -1e100, -1024, -1, -0.0001,
		math.Copysign(0, -1), 0, 0.0001, 1, 1024, 1e100, math.MaxFloat64, math.Inf(1),
	}
	for i := 0; i < len(nums); i++ {
		for j := i + 1; j < len(nums); j++ {
			a := mustEncode(t, nums[i])
			b := mustEncode(t, nums[j])
			if bytes.Compare(a, b) >= 0 {
				t.Fatalf("expected %v < %v", nums[i], nums[j])
			}
		}
	}
}

func TestStringEncodingHasNoPrefixCollisions(t *testing.T) {
	// A string must never be encoded as a strict prefix of another string's
	// encoding, otherwise appending a document id would create ambiguous keys.
	strings := []string{"", "a", "aa", "ab", "abc", "\x00", "\x00\x00", "a\x00b", "a\x01"}
	for i := 0; i < len(strings); i++ {
		for j := 0; j < len(strings); j++ {
			if i == j {
				continue
			}
			a := EncodeString(strings[i])
			b := EncodeString(strings[j])
			if bytes.HasPrefix(b, a) {
				t.Fatalf("%q encoded as prefix of %q: %q / %q", strings[i], strings[j], KeyString(a), KeyString(b))
			}
		}
	}
}

func TestEncodeKey(t *testing.T) {
	id := bytes.Repeat([]byte{0xAB}, 16)
	key, err := EncodeKey("hello", id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(key, id) {
		t.Fatal("key does not end with id")
	}
	// Equality prefix must cover all keys for that value, regardless of id.
	prefix := mustEncode(t, "hello")
	if !bytes.HasPrefix(key, prefix) {
		t.Fatal("key does not start with encoded value")
	}
	start, end := PrefixRange(prefix)
	if bytes.Compare(key, start) < 0 {
		t.Fatal("key below its equality range")
	}
	if end != nil && bytes.Compare(key, end) >= 0 {
		t.Fatal("key above its equality range")
	}

	if _, err := EncodeKey("x", []byte{1, 2, 3}); err == nil {
		t.Fatal("expected invalid id length error")
	}
}

func TestEncodeIndexValuesMultikey(t *testing.T) {
	got, err := EncodeIndexValues([]any{"go", "svelte", "go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 unique values, got %d", len(got))
	}
	got, err = EncodeIndexValues([]any{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty array should produce no entries, got %d", len(got))
	}
	if _, err := EncodeIndexValues(map[string]any{"a": 1}); err == nil {
		t.Fatal("objects should not be indexable")
	}
	if _, err := EncodeIndexValues([]any{map[string]any{"a": 1}}); err == nil {
		t.Fatal("object array elements should not be indexable")
	}
}

func TestPrefixSuccessor(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc", "abd"},
		{"\x01\x02\x03", "\x01\x02\x04"},
		{"\xff\xff", ""},
		{"a\xff", "b"},
	}
	for _, tc := range cases {
		got := PrefixSuccessor([]byte(tc.in))
		if string(got) != tc.want {
			t.Fatalf("PrefixSuccessor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCompareMatchesBytes(t *testing.T) {
	values := []any{nil, false, true, -5.0, 0.0, 2.5, "", "a", "b", "aa", "\x00", "é"}
	for i := 0; i < len(values); i++ {
		for j := 0; j < len(values); j++ {
			got := Compare(values[i], values[j])
			want := bytes.Compare(mustEncode(t, values[i]), mustEncode(t, values[j]))
			wantSign := sign(want)
			if got != wantSign {
				t.Fatalf("Compare(%v,%v)=%d, encoded comparison=%d", values[i], values[j], got, wantSign)
			}
		}
	}
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestRandomStringOrderProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	alphabet := []byte{0x00, 0x01, 0x02, 'a', 'b', 'z', 0x7F, 0xC3, 0xA9, 0xFF}
	for n := 0; n < 5000; n++ {
		a := randomString(rng, alphabet, rng.Intn(32))
		b := randomString(rng, alphabet, rng.Intn(32))
		want := stringsCompare(a, b)
		got := bytes.Compare(EncodeString(a), EncodeString(b))
		if sign(got) != want {
			t.Fatalf("order mismatch for %q vs %q: bytes=%d lexical=%d", a, b, sign(got), want)
		}
	}
}

func TestRandomNumberOrderProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for n := 0; n < 10000; n++ {
		a := (rng.Float64() - 0.5) * math.Pow(10, float64(rng.Intn(40)-20))
		b := (rng.Float64() - 0.5) * math.Pow(10, float64(rng.Intn(40)-20))
		ea := mustEncode(t, a)
		eb := mustEncode(t, b)
		want := 0
		if a < b {
			want = -1
		} else if a > b {
			want = 1
		}
		if sign(bytes.Compare(ea, eb)) != want {
			t.Fatalf("number order mismatch: %v vs %v", a, b)
		}
	}
}

func randomString(rng *rand.Rand, alphabet []byte, n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = alphabet[rng.Intn(len(alphabet))]
	}
	return string(b)
}

func stringsCompare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func TestJSONNumberEncoding(t *testing.T) {
	a := mustEncode(t, json.Number("42"))
	b := mustEncode(t, json.Number("43"))
	if bytes.Compare(a, b) >= 0 {
		t.Fatal("json.Number ordering mismatch")
	}
	if _, err := EncodeIndexValue(json.Number("not-a-number")); err == nil {
		t.Fatal("expected invalid number error")
	}
}

func FuzzEncodeStringOrder(f *testing.F) {
	f.Add("a", "b")
	f.Add("a\x00", "a\x01")
	f.Add("", "\x00")
	f.Fuzz(func(t *testing.T, a, b string) {
		ea := EncodeString(a)
		eb := EncodeString(b)
		if got, want := sign(bytes.Compare(ea, eb)), stringsCompare(a, b); got != want {
			t.Fatalf("order mismatch: %q vs %q", a, b)
		}
	})
}

func TestSortValuesByEncodedKey(t *testing.T) {
	values := []any{"b", 10, nil, true, false, "a", 2}
	sort.Slice(values, func(i, j int) bool {
		return bytes.Compare(mustEncode(t, values[i]), mustEncode(t, values[j])) < 0
	})
	want := []any{nil, false, true, 2, 10, "a", "b"}
	for i := range want {
		if values[i] != want[i] {
			t.Fatalf("sorted[%d]=%v, want %v (all: %v)", i, values[i], want[i], values)
		}
	}
}
