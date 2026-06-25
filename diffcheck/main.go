package diffcheck

import (
	"context"
	"errors"
	"fmt"
)

// RunCase executes all engines on a single test case and produces a CaseResult.
func RunCase(ctx context.Context, engines []Engine, tc TestCase, mode NormalizeMode) *CaseResult {
	cr := &CaseResult{
		Index:   tc.Index,
		Source:  tc.Source,
		Input:   tc.Input,
		Outputs: make(map[string]string),
		Errors:  make(map[string]error),
	}

	// Run each engine, store normalized output
	for _, eng := range engines {
		output, err := eng.Convert(ctx, tc.Input)
		if err != nil {
			cr.Errors[eng.Name()] = err
			cr.Outputs[eng.Name()] = fmt.Sprintf("[ERROR: %v]", err)
		} else {
			cr.Outputs[eng.Name()] = Normalize(output, mode)
		}
	}

	// Pairwise comparisons
	if len(engines) >= 2 {
		for i := 0; i < len(engines); i++ {
			for j := i + 1; j < len(engines); j++ {
				a, b := engines[i], engines[j]
				cr.Pairs = append(cr.Pairs, Compare(
					a.Name(), cr.Outputs[a.Name()],
					b.Name(), cr.Outputs[b.Name()],
					mode,
				))
			}
		}
	}

	return cr
}

// IsTimeout checks if an error is a timeout error.
func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout)
}
