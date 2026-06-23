package integration_test

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"md4go"
	"md4go/extension"
	"md4go/text"
)

// TestConcurrentUse verifies that a single Markdown instance can be used safely
// from multiple goroutines concurrently. The Parser must be read-only
// after construction (no mutation during Parse calls).
func TestConcurrentUse(t *testing.T) {
	md := md4go.New(md4go.WithExtensions(extension.GFM...))
	inputs := []string{
		"# Hello\n",
		"**bold** and *italic*\n",
		"- item 1\n- item 2\n",
		"> blockquote\n",
		"[link](http://example.com)\n",
		"| a | b |\n|---|---|\n| c | d |\n",
		"~~strikethrough~~\n",
		"`code`\n",
		"```\nfenced\n```\n",
		"&amp; entity\n",
	}

	var wg sync.WaitGroup
	errCh := make(chan error, runtime.NumCPU()*len(inputs))

	for i := 0; i < runtime.NumCPU(); i++ {
		for _, input := range inputs {
			wg.Add(1)
			go func(src string) {
				defer wg.Done()
				defer func() {
					if r := recover(); r != nil {
						errCh <- fmt.Errorf("panic: %v", r)
					}
				}()
				var buf bytes.Buffer
				if err := mdConvertPlain(md, []byte(src), &buf); err != nil {
					errCh <- fmt.Errorf("Convert error: %v", err)
				}
			}(input)
		}
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestConcurrentStreamUse verifies streaming mode under concurrent access.
func TestConcurrentStreamUse(t *testing.T) {
	input := []byte("Hello **world**\n\nParagraph two\n")

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic: %v", r)
				}
			}()
			var buf bytes.Buffer
			if err := text.ConvertStream(bytes.NewReader(input), &buf); err != nil {
				errCh <- fmt.Errorf("ConvertStream error: %v", err)
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestConcurrentHTMLAndPlain verifies that HTML and PlainText rendering
// can happen concurrently on the same Markdown instance.
func TestConcurrentHTMLAndPlain(t *testing.T) {
	md := md4go.New(md4go.WithExtensions(extension.GFM...))
	input := []byte("# Title\n\n**bold** and *italic*\n\n- item\n")

	var wg sync.WaitGroup
	errCh := make(chan error, 10)

	for i := 0; i < 5; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic in HTML: %v", r)
				}
			}()
			var buf bytes.Buffer
			if err := mdConvertHTML(md, input, &buf); err != nil {
				errCh <- fmt.Errorf("ConvertHTML error: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("panic in Plain: %v", r)
				}
			}()
			var buf bytes.Buffer
			if err := mdConvertPlain(md, input, &buf); err != nil {
				errCh <- fmt.Errorf("Convert error: %v", err)
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}
