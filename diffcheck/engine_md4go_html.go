package diffcheck

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
)

// Md4goHTMLEngine converts markdown→HTML using md4go's HTML renderer, then
// extracts plain text via goquery — the same pipeline as GoldmarkEngine.
//
// This isolates parser-level differences from goquery-pipeline differences:
// both engines use identical HTML→text extraction, so any remaining diff is
// purely from the markdown→HTML parsing stage.
type Md4goHTMLEngine struct {
	flags      parser.Flags
	timeout    time.Duration
	imgPattern *regexp.Regexp
}

// Md4goHTMLEngineOption configures a Md4goHTMLEngine.
type Md4goHTMLEngineOption func(*Md4goHTMLEngine)

// WithMd4goHTMLFlags sets parser flags for the md4go-html engine.
func WithMd4goHTMLFlags(flags parser.Flags) Md4goHTMLEngineOption {
	return func(e *Md4goHTMLEngine) { e.flags = flags }
}

// NewMd4goHTMLEngine creates an md4go-html engine. Defaults to DialectGitHub.
func NewMd4goHTMLEngine(timeout time.Duration, opts ...Md4goHTMLEngineOption) *Md4goHTMLEngine {
	e := &Md4goHTMLEngine{
		flags:      parser.DialectGitHub,
		timeout:    timeout,
		imgPattern: regexp.MustCompile(`!\[.*?\]\(.*?\)|!\[.*?\]\[.*?\]`),
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

func (e *Md4goHTMLEngine) Name() string {
	if e.flags&parser.GoldmarkCompat != 0 {
		return "md4go-html(goldmark-compat)"
	}
	return "md4go-html"
}

func (e *Md4goHTMLEngine) Convert(ctx context.Context, input []byte) (string, error) {
	result, err := RunWithTimeout(e.timeout, func() (string, error) {
		// Remove image syntax before conversion (mirrors GoldmarkEngine).
		noImages := e.imgPattern.ReplaceAllString(string(input), "")

		// Markdown → HTML via md4go
		var htmlBuf bytes.Buffer
		rFlags := html.FlagXHTML
		if err := html.Convert([]byte(noImages), &htmlBuf,
			html.WithFlags(e.flags),
			html.WithRendererFlags(rFlags),
		); err != nil {
			return "", err
		}

		// HTML → plain text via goquery (same as GoldmarkEngine)
		htmlBytes := htmlBuf.Bytes()
		doc, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBytes))
		if err != nil {
			// goquery's HTML parser may fail on deeply nested/malformed HTML
			// (e.g. "open stack of elements exceeds 512 nodes"). Fall back to
			// a simple tag-stripping approach that extracts text content.
			return simpleHTMLText(htmlBytes), nil
		}

		var textParts []string
		walkNodes(doc.Find("body"), &textParts)

		plainText := strings.Join(textParts, " ")
		plainText = strings.ReplaceAll(plainText, "\ufeff", "")
		return strings.Join(strings.Fields(plainText), " "), nil
	})
	return result, err
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
