package codec

import (
	"encoding/json"
	"testing"
)

func BenchmarkEncodeStringShort(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EncodeString("active")
	}
}

func BenchmarkEncodeStringLong(b *testing.B) {
	s := "a moderately long string value used as an index key for benchmarking purposes"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = EncodeString(s)
	}
}

func BenchmarkEncodeNumber(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := EncodeIndexValue(json.Number("123456789")); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeIndexValuesArray(b *testing.B) {
	v := []any{"go", "database", "api", "svelte", "typescript", "bun", "performance", "benchmark"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := EncodeIndexValues(v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCompareNumbers(b *testing.B) {
	a := json.Number("1234")
	c := json.Number("5678")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Compare(a, c)
	}
}

func BenchmarkCompareStrings(b *testing.B) {
	a := "alpha"
	c := "beta"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Compare(a, c)
	}
}

func BenchmarkPrefixSuccessor(b *testing.B) {
	prefix := []byte{0x04, 'a', 'c', 't', 'i', 'v', 'e', 0x00, 0x01}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = PrefixSuccessor(prefix)
	}
}
