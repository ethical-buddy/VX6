package identity

import "testing"

func BenchmarkIdentityGenerate(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Generate(); err != nil {
			b.Fatal(err)
		}
	}
}
