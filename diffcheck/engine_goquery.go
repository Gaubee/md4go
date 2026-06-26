package diffcheck

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/userpro/md4go/parser"
)

// walkNodes recursively traverses DOM nodes, extracting text content.
// Ported from common_test/markdown/strip.go.
func walkNodes(s *goquery.Selection, result *[]string) {
	s.Contents().Each(func(i int, node *goquery.Selection) {
		if goquery.NodeName(node) == "#text" {
			txt := strings.TrimSpace(node.Text())
			if txt != "" {
				*result = append(*result, txt)
			}
		} else {
			walkNodes(node, result)
		}
	})
}

// simpleHTMLText extracts text from HTML by stripping tags — a fallback
// for when goquery's DOM parser rejects the input (deep nesting limit).
// Inserts a space at each tag boundary to preserve word separation, then
// collapses whitespace. Uses parser.ScanHTMLTag for HTML construct detection.
func simpleHTMLText(htmlBytes []byte) string {
	var out []byte
	i := 0
	for i < len(htmlBytes) {
		if htmlBytes[i] != '<' {
			out = append(out, htmlBytes[i])
			i++
			continue
		}
		if end := parser.ScanHTMLTag(htmlBytes, i); end > i {
			out = append(out, ' ')
			i = end
		} else {
			out = append(out, '<')
			i++
		}
	}
	return strings.Join(strings.Fields(string(out)), " ")
}
