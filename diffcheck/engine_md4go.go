package diffcheck

import (
	"bytes"
	"context"
	"time"

	"github.com/userpro/md4go/parser"
	"github.com/userpro/md4go/text"
)

// Md4goEngine uses the local md4go library.
type Md4goEngine struct {
	flags   parser.Flags
	timeout time.Duration
}

// Md4goEngineOption configures a Md4goEngine.
type Md4goEngineOption func(*Md4goEngine)

// WithMd4goFlags sets parser flags for the md4go engine (overrides DialectGitHub).
func WithMd4goFlags(flags parser.Flags) Md4goEngineOption {
	return func(e *Md4goEngine) { e.flags = flags }
}

// NewMd4goEngine creates an md4go engine. Defaults to DialectGitHub.
func NewMd4goEngine(timeout time.Duration, opts ...Md4goEngineOption) *Md4goEngine {
	e := &Md4goEngine{
		flags:   parser.DialectGitHub,
		timeout: timeout,
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

func (e *Md4goEngine) Name() string {
	if e.flags&parser.GoldmarkCompat != 0 {
		return "md4go(goldmark-compat)"
	}
	return "md4go"
}

func (e *Md4goEngine) Convert(ctx context.Context, input []byte) (string, error) {
	result, err := RunWithTimeout(e.timeout, func() (string, error) {
		var buf bytes.Buffer
		if err := text.Convert(input, &buf, text.WithFlags(e.flags)); err != nil {
			return "", err
		}
		return buf.String(), nil
	})
	return result, err
}
