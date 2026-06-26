package diffcheck

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// GoldmarkEngine uses goldmark→HTML→goquery to extract plain text.
// Ported from common_test/markdown/strip.go.
type GoldmarkEngine struct {
	timeout    time.Duration
	engine     goldmark.Markdown
	imgPattern *regexp.Regexp
	commonmark bool // true = no extensions (CommonMark only)
}

// GoldmarkEngineOption configures a GoldmarkEngine.
type GoldmarkEngineOption func(*GoldmarkEngine)

// WithGoldmarkCommonMark sets the goldmark engine to CommonMark-only mode (no extensions).
func WithGoldmarkCommonMark() GoldmarkEngineOption {
	return func(e *GoldmarkEngine) { e.commonmark = true }
}

func NewGoldmarkEngine(timeout time.Duration, opts ...GoldmarkEngineOption) *GoldmarkEngine {
	e := &GoldmarkEngine{
		timeout:    timeout,
		imgPattern: regexp.MustCompile(`!\[.*?\]\(.*?\)|!\[.*?\]\[.*?\]`),
	}
	for _, o := range opts {
		o(e)
	}
	if e.commonmark {
		e.engine = goldmark.New()
	} else {
		e.engine = goldmark.New(
			goldmark.WithExtensions(extension.Table, extension.GFM),
			goldmark.WithRendererOptions(
				html.WithHardWraps(),
			),
		)
	}
	return e
}

func (e *GoldmarkEngine) Name() string {
	if e.commonmark {
		return "goldmark(commonmark)"
	}
	return "goldmark"
}

func (e *GoldmarkEngine) Convert(ctx context.Context, input []byte) (string, error) {
	result, err := RunWithTimeout(e.timeout, func() (string, error) {
		// Remove image syntax before conversion.
		noImages := e.imgPattern.ReplaceAllString(string(input), "")

		// Markdown → HTML
		var htmlBuf bytes.Buffer
		if err := e.engine.Convert([]byte(noImages), &htmlBuf); err != nil {
			return "", err
		}

		// HTML → plain text via goquery
		doc, err := goquery.NewDocumentFromReader(&htmlBuf)
		if err != nil {
			return "", err
		}

		var textParts []string
		walkNodes(doc.Find("body"), &textParts)

		plainText := strings.Join(textParts, " ")
		plainText = strings.ReplaceAll(plainText, "\ufeff", "")
		return strings.Join(strings.Fields(plainText), " "), nil
	})
	return result, err
}