package diffcheck

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// Md4cHTMLEngine calls the md4c-html C binary via os/exec, then extracts
// plain text via goquery — the same HTML→text pipeline as Md4goHTMLEngine
// and GoldmarkEngine (including image-stripping preprocessing).
// This eliminates pipeline noise and isolates parser-level differences only.
type Md4cHTMLEngine struct {
	binPath    string
	timeout    time.Duration
	commonmark bool
	imgPattern *regexp.Regexp
}

// Md4cHTMLEngineOption configures a Md4cHTMLEngine.
type Md4cHTMLEngineOption func(*Md4cHTMLEngine)

// WithMd4cHTMLCommonMark sets the md4c-html engine to CommonMark-only mode.
func WithMd4cHTMLCommonMark() Md4cHTMLEngineOption {
	return func(e *Md4cHTMLEngine) { e.commonmark = true }
}

// NewMd4cHTMLEngine creates an md4c-html engine. Defaults to GFM mode.
// Applies the same image-stripping preprocessing as GoldmarkEngine and Md4goHTMLEngine.
func NewMd4cHTMLEngine(timeout time.Duration, opts ...Md4cHTMLEngineOption) (*Md4cHTMLEngine, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	binPath := filepath.Join(filepath.Dir(thisFile), "csrc", "md4c-html")

	e := &Md4cHTMLEngine{
		binPath:    binPath,
		timeout:    timeout,
		imgPattern: regexp.MustCompile(`!\[.*?\]\(.*?\)|!\[.*?\]\[.*?\]`),
	}
	for _, o := range opts {
		o(e)
	}
	if err := e.checkBinary(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Md4cHTMLEngine) Name() string {
	if e.commonmark {
		return "md4c-html(commonmark)"
	}
	return "md4c-html"
}

func (e *Md4cHTMLEngine) Convert(ctx context.Context, input []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	flag := "--gfm"
	if e.commonmark {
		flag = "--commonmark"
	}

	// Remove image syntax before conversion (mirrors GoldmarkEngine/Md4goHTMLEngine).
	noImages := []byte(e.imgPattern.ReplaceAllString(string(input), ""))

	cmd := exec.CommandContext(ctx, e.binPath, flag)
	cmd.Stdin = bytes.NewReader(noImages)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%w: md4c-html timeout after %s", ErrTimeout, e.timeout)
		}
		return "", fmt.Errorf("md4c-html exec: %w (stderr: %s)", err, stderr.String())
	}

	htmlBytes := stdout.Bytes()

	// HTML → plain text via goquery (same pipeline as Md4goHTMLEngine/GoldmarkEngine)
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(htmlBytes))
	if err != nil {
		// Fall back to tag-stripping for deeply nested HTML
		return simpleHTMLText(htmlBytes), nil
	}

	var textParts []string
	walkNodes(doc.Find("body"), &textParts)

	plainText := strings.Join(textParts, " ")
	plainText = strings.ReplaceAll(plainText, "\ufeff", "")
	return strings.Join(strings.Fields(plainText), " "), nil
}

func (e *Md4cHTMLEngine) checkBinary() error {
	if _, err := exec.LookPath(e.binPath); err != nil {
		return fmt.Errorf("md4c-html binary not found at %s\nCompile with: cd diffcheck/csrc && gcc -O2 -I../../md4c/src -o md4c-html main_html.c ../../md4c/src/md4c.c ../../md4c/src/md4c-html.c ../../md4c/src/entity.c",
			e.binPath)
	}
	return nil
}
