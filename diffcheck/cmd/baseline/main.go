// +build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"diffcheck"
)

const testTimeout = 10 * time.Second

func main() {
	// Data is in diffcheck/data/ relative to the diffcheck dir
	_, thisFile, _, _ := runtime.Caller(0)
	diffcheckDir := filepath.Join(filepath.Dir(thisFile), "../..")
	dataDir := filepath.Join(diffcheckDir, "data")

	var allCases []diffcheck.TestCase
	for _, name := range []string{"aidata_content.jsonl", "testdata1.jsonl", "testdata2.jsonl"} {
		p := filepath.Join(dataDir, name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		cases, err := diffcheck.LoadJSONL(p)
		if err != nil {
			fmt.Printf("skip %s: %v\n", name, err)
			continue
		}
		allCases = append(allCases, cases...)
	}
	if len(allCases) == 0 {
		fmt.Println("no JSONL data available")
		return
	}

	fmt.Printf("Testing %d cases (NO GoldmarkCompat)...\n", len(allCases))

	// Test WITHOUT GoldmarkCompat (pure GFM standard alignment with md4c)
	engines := []diffcheck.Engine{
		diffcheck.NewMd4goHTMLEngine(testTimeout, diffcheck.WithMd4goHTMLFlags(diffcheck.DialectGitHubFlags)),
	}
	md4cHeng, err := diffcheck.NewMd4cHTMLEngine(testTimeout)
	if err != nil {
		fmt.Printf("md4c-html engine unavailable: %v\n", err)
		return
	}
	engines = append(engines, md4cHeng)

	md4goVsMd4c := 0
	timeouts := 0
	errors_ := 0

	for i, tc := range allCases {
		if i%2000 == 0 {
			fmt.Printf("  Progress: %d/%d (diffs so far: %d)\n", i, len(allCases), md4goVsMd4c)
		}
		cr := diffcheck.RunCase(context.Background(), engines, tc, diffcheck.NormalizeLoose)

		for _, err := range cr.Errors {
			if err != nil {
				if diffcheck.IsTimeout(err) {
					timeouts++
				} else {
					errors_++
				}
			}
		}

		for _, p := range cr.Pairs {
			if p != nil && !p.Same {
				a, b := p.NameA, p.NameB
				if (hasPrefix(a, "md4go") && hasPrefix(b, "md4c")) ||
					(hasPrefix(a, "md4c") && hasPrefix(b, "md4go")) {
					md4goVsMd4c++
					if md4goVsMd4c <= 20 {
						fmt.Printf("\n=== Diff #%d (Case #%d) ===\n", md4goVsMd4c, tc.Index)
						fmt.Printf("Input: %s\n", ellipsize(string(tc.Input), 300))
						fmt.Printf("md4go-html: %s\n", ellipsize(cr.Outputs[p.NameA], 300))
						fmt.Printf("md4c-html:  %s\n", ellipsize(cr.Outputs[p.NameB], 300))
					}
				}
			}
		}
	}

	fmt.Printf("\n=== FINAL RESULTS ===\n")
	fmt.Printf("Total: %d | md4go≠md4c (NO compat): %d | timeouts: %d | errors: %d\n",
		len(allCases), md4goVsMd4c, timeouts, errors_)
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func ellipsize(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
