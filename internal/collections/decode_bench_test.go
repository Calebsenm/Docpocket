package collections

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func BenchmarkDecodeCurrent(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Decode(benchRaw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeBytesReader(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dec := json.NewDecoder(strings.NewReader(string(benchRaw)))
		dec.UseNumber()
		var d Document
		if err := dec.Decode(&d); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeBytesReader2(b *testing.B) {
	reader := strings.NewReader(string(benchRaw))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader.Reset(string(benchRaw))
		dec := json.NewDecoder(reader)
		dec.UseNumber()
		var d Document
		if err := dec.Decode(&d); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeStdUnmarshal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var d Document
		if err := json.Unmarshal(benchRaw, &d); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeReusedDecoder(b *testing.B) {
	reader := bytes.NewReader(nil)
	dec := json.NewDecoder(reader)
	dec.UseNumber()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader.Reset(benchRaw)
		var d Document
		if err := dec.Decode(&d); err != nil {
			b.Fatal(err)
		}
	}
}
