package diffcheck

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrTimeout is returned when an engine exceeds the per-case timeout.
var ErrTimeout = errors.New("engine timeout")

// Engine converts Markdown input to plain text.
type Engine interface {
	// Name identifies the engine (e.g. "md4go", "md4c", "goldmark").
	Name() string
	// Convert converts the markdown input to plain text.
	// Implementations should respect ctx for cancellation.
	Convert(ctx context.Context, input []byte) (string, error)
}

// RunWithTimeout runs fn with a timeout. It returns the result or ErrTimeout.
// This is used by Go-based engines (md4go, goldmark) that cannot be
// cancelled via context natively — we wrap them in a goroutine + channel.
func RunWithTimeout(timeout time.Duration, fn func() (string, error)) (string, error) {
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		s, err := fn()
		ch <- result{s, err}
	}()

	select {
	case r := <-ch:
		return r.s, r.err
	case <-time.After(timeout):
		return "", fmt.Errorf("%w: %s", ErrTimeout, timeout)
	}
}
