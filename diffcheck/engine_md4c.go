package diffcheck

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// Md4cEngine calls the md4c-plain binary via os/exec.
type Md4cEngine struct {
	binPath string
	timeout time.Duration
	flag    string // "--gfm" or "--commonmark"
}

// Md4cEngineOption configures a Md4cEngine.
type Md4cEngineOption func(*Md4cEngine)

// WithMd4cCommonMark sets the md4c engine to CommonMark-only mode (no extensions).
func WithMd4cCommonMark() Md4cEngineOption {
	return func(e *Md4cEngine) { e.flag = "--commonmark" }
}

// NewMd4cEngine creates an md4c engine. Defaults to GFM mode (--gfm).
func NewMd4cEngine(timeout time.Duration, opts ...Md4cEngineOption) (*Md4cEngine, error) {
	_, thisFile, _, _ := runtime.Caller(0)
	binPath := filepath.Join(filepath.Dir(thisFile), "csrc", "md4c-plain")

	e := &Md4cEngine{binPath: binPath, timeout: timeout, flag: "--gfm"}
	for _, o := range opts {
		o(e)
	}
	if err := e.checkBinary(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Md4cEngine) Name() string {
	if e.flag == "--commonmark" {
		return "md4c(commonmark)"
	}
	return "md4c"
}

func (e *Md4cEngine) Convert(ctx context.Context, input []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.binPath, e.flag)
	cmd.Stdin = bytes.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%w: md4c timeout after %s", ErrTimeout, e.timeout)
		}
		return "", fmt.Errorf("md4c exec: %w (stderr: %s)", err, stderr.String())
	}
	return stdout.String(), nil
}

func (e *Md4cEngine) checkBinary() error {
	if _, err := exec.LookPath(e.binPath); err != nil {
		return fmt.Errorf("md4c-plain binary not found at %s\nCompile with: cd diffcheck/csrc && gcc -O2 -I../../md4c/src -o md4c-plain main.c ../../md4c/src/md4c.c",
			e.binPath)
	}
	return nil
}
