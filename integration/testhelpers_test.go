package integration_test

import (
	"bytes"

	"md4go"
	"md4go/html"
	"md4go/text"
)

// mdConvertPlain converts src to plain text using the given parser.
// Shared by concurrency, edge-case, and regression test files.
func mdConvertPlain(md *md4go.Parser, src []byte, w *bytes.Buffer) error {
	pt := text.NewPlainText(w)
	if err := md.Parse(src, pt); err != nil {
		return err
	}
	return pt.Flush()
}

// mdConvertHTML converts src to HTML using the given parser.
// Shared by concurrency, edge-case, and regression test files.
func mdConvertHTML(md *md4go.Parser, src []byte, w *bytes.Buffer) error {
	h := html.NewHTML(w)
	if err := md.Parse(src, h); err != nil {
		return err
	}
	return h.Flush()
}
