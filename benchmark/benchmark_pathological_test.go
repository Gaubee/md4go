package benchmark

import (
	"testing"

	"github.com/userpro/md4go/parser"
)

// ─── Pathological benchmarks (26 cases × 3 engines) ───

func BenchmarkPathological(b *testing.B) {
	cases := BuildPathologicalCases()

	for _, tc := range cases {
		b.Run(tc.Name, func(b *testing.B) {
			input := tc.Input()
			src := []byte(input)

			// md4go flags and md4c flags share identical bit values by design.
			md4cFlags := uint(tc.Flags)
			gfm := tc.Flags&parser.DialectGitHub != 0

			// Pre-create parsers for this case
			md4goP := getMd4goParser(tc.Flags)
			gm := getGoldmarkParser(gfm)

			b.Run("Md4go", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					md4goHTML(md4goP, src)
				}
			})

			b.Run("Md4cCgo", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					Md4cConvertHTML(src, md4cFlags, 0)
				}
			})

			b.Run("Goldmark", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					goldmarkHTML(gm, src)
				}
			})
		})
	}
}
