package collections

import (
	"encoding/json"
	"testing"
)

var benchRaw = []byte(`{"id":"01ARZ3NDEKTSV4RRFFQ69G5FAV","created":"2026-01-01T00:00:00.000Z","updated":"2026-01-01T00:00:00.000Z","title":"Hello world","views":1234,"status":"active","tags":["go","db","api"],"author":{"name":"Ana","email":"ana@example.com"},"published":true}`)

func benchDoc() Document {
	d, err := Decode(benchRaw)
	if err != nil {
		panic(err)
	}
	return d
}

func BenchmarkDecode(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(benchRaw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncode(b *testing.B) {
	d := benchDoc()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.Encode(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClone(b *testing.B) {
	d := benchDoc()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if d.Clone() == nil {
			b.Fatal("clone returned nil")
		}
	}
}

func BenchmarkSanitize(b *testing.B) {
	d := benchDoc()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if d.Sanitize() == nil {
			b.Fatal("sanitize returned nil")
		}
	}
}

func BenchmarkGetPathTop(b *testing.B) {
	d := benchDoc()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetPath(d, "title")
	}
}

func BenchmarkGetPathNested(b *testing.B) {
	d := benchDoc()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetPath(d, "author.name")
	}
}

func BenchmarkMergePatch(b *testing.B) {
	d := benchDoc()
	patch := Document{"title": "Updated", "views": json.Number("4321"), "meta": map[string]any{"a": 1}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		MergePatch(d, patch)
	}
}

func BenchmarkValidID(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !ValidID("01ARZ3NDEKTSV4RRFFQ69G5FAV") {
			b.Fatal("invalid id")
		}
	}
}
