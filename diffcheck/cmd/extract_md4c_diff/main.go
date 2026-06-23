package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"diffcheck"
)

func main() {
	allCases := diffcheck.LoadFuzzSeeds()
	path := diffcheck.DefaultJSONLPath()
	if jsonlCases, err := diffcheck.LoadJSONL(path); err == nil {
		allCases = append(allCases, jsonlCases...)
	}

	md4goEng := diffcheck.NewMd4goEngine(30 * time.Second)
	md4c, err := diffcheck.NewMd4cEngine(30 * time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "md4c unavailable: %v\n", err)
		return
	}

	ctx := context.Background()
	diffCount := 0
	for _, tc := range allCases {
		outGo, errGo := md4goEng.Convert(ctx, tc.Input)
		if errGo != nil {
			continue
		}
		out4, err4 := md4c.Convert(ctx, tc.Input)
		if err4 != nil {
			continue
		}
		normGo := diffcheck.Normalize(outGo, diffcheck.NormalizeLoose)
		norm4 := diffcheck.Normalize(out4, diffcheck.NormalizeLoose)
		if normGo != norm4 {
			diffCount++
			wGo := strings.Fields(normGo)
			w4 := strings.Fields(norm4)
			firstDiff := -1
			minLen := len(wGo)
			if len(w4) < minLen {
				minLen = len(w4)
			}
			for i := 0; i < minLen; i++ {
				if wGo[i] != w4[i] {
					firstDiff = i
					break
				}
			}
			if firstDiff == -1 {
				firstDiff = minLen
			}
			start := firstDiff - 3
			if start < 0 {
				start = 0
			}
			end := firstDiff + 5
			ctxGo := "[...]"
			ctx4 := "[...]"
			if end <= len(wGo) {
				ctxGo = strings.Join(wGo[start:end], " ")
			} else if start < len(wGo) {
				ctxGo = strings.Join(wGo[start:], " ") + " [END]"
			}
			if end <= len(w4) {
				ctx4 = strings.Join(w4[start:end], " ")
			} else if start < len(w4) {
				ctx4 = strings.Join(w4[start:], " ") + " [END]"
			}

			inputStr := string(tc.Input)
			if len(inputStr) > 200 {
				inputStr = inputStr[:200] + "..."
			}
			fmt.Printf("━━━ Case #%d [%s] ━━━\n", tc.Index, tc.Source)
			fmt.Printf("Input: %q\n", inputStr)
			fmt.Printf("  word count: md4go=%d md4c=%d first_diff_word=%d\n", len(wGo), len(w4), firstDiff)
			fmt.Printf("  md4go ctx: %s\n", ctxGo)
			fmt.Printf("  md4c  ctx: %s\n", ctx4)
			fmt.Println()
		}
	}
	fmt.Printf("\nTotal: %d | md4go≠md4c: %d (loose)\n", len(allCases), diffCount)
}
