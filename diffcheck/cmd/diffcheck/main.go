package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"diffcheck"
)

func main() {
	src := flag.String("src", "all", "test source: jsonl, fuzz, constructed, or all")
	filePath := flag.String("file", "", "path to a markdown file to test (overrides --src)")
	stdinMode := flag.Bool("stdin", false, "read markdown from stdin (overrides --src and --file)")
	timeout := flag.Duration("timeout", 10*time.Second, "per-case timeout")
	normMode := flag.String("normalize", "loose", "normalization mode: loose or strict")
	dialect := flag.String("dialect", "github", "markdown dialect: github or commonmark")
	compat := flag.String("compat", "none", "md4go compat mode: none or goldmark")
	withMd4goHTML := flag.Bool("md4go-html", false, "include md4go-html engine (md4go→HTML→goquery text pipeline)")
	withMd4cHTML := flag.Bool("md4c-html", false, "include md4c-html engine (md4c→HTML→goquery text pipeline)")
	outputPath := flag.String("output", "", "write report to this file instead of stdout")
	verbose := flag.Bool("v", false, "show identical cases too")
	flag.Parse()

	// Output writer: file or stdout.
	var out io.Writer = os.Stdout
	if *outputPath != "" {
		f, err := os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "create output file %s: %v\n", *outputPath, err)
			os.Exit(1)
		}
		defer f.Close()
		out = f
	}

	var mode diffcheck.NormalizeMode
	switch *normMode {
	case "strict":
		mode = diffcheck.NormalizeStrict
	case "loose":
		mode = diffcheck.NormalizeLoose
	default:
		fmt.Fprintf(os.Stderr, "unknown normalize mode %q, use loose or strict\n", *normMode)
		os.Exit(1)
	}

	// Build md4go base flags based on dialect.
	var md4goFlags diffcheck.Flags
	var md4cOpts []diffcheck.Md4cEngineOption
	var md4cHTMLOpts []diffcheck.Md4cHTMLEngineOption
	var goldmarkOpts []diffcheck.GoldmarkEngineOption

	switch *dialect {
	case "commonmark":
		md4goFlags = diffcheck.DialectCommonMarkFlags
		md4cOpts = append(md4cOpts, diffcheck.WithMd4cCommonMark())
		md4cHTMLOpts = append(md4cHTMLOpts, diffcheck.WithMd4cHTMLCommonMark())
		goldmarkOpts = append(goldmarkOpts, diffcheck.WithGoldmarkCommonMark())
	case "github":
		md4goFlags = diffcheck.DialectGitHubFlags
	default:
		fmt.Fprintf(os.Stderr, "unknown dialect %q, use github or commonmark\n", *dialect)
		os.Exit(1)
	}

	// Add compat flags (only meaningful for GFM dialect).
	switch *compat {
	case "goldmark":
		md4goFlags |= diffcheck.GoldmarkCompatFlags
	case "none":
		// default only
	default:
		fmt.Fprintf(os.Stderr, "unknown compat mode %q, use none or goldmark\n", *compat)
		os.Exit(1)
	}

	// Load test cases.
	var cases []diffcheck.TestCase
	switch {
	case *stdinMode:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
			os.Exit(1)
		}
		cases = []diffcheck.TestCase{{Index: 0, Source: "stdin", Input: data}}
	case *filePath != "":
		data, err := os.ReadFile(*filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read file %s: %v\n", *filePath, err)
			os.Exit(1)
		}
		cases = []diffcheck.TestCase{{Index: 0, Source: "file", Input: data}}
	default:
		loaded, err := loadBuiltinCases(*src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		cases = loaded
	}

	if len(cases) == 0 {
		fmt.Fprintln(out, "No test cases loaded.")
		return
	}

	fmt.Fprintf(out, "Loaded %d test cases (source=%s, dialect=%s, compat=%s, normalize=%s, timeout=%s)\n\n",
		len(cases), sourceLabel(*stdinMode, *filePath, *src), *dialect, *compat, *normMode, *timeout)

	var engines []diffcheck.Engine

	engines = append(engines, diffcheck.NewMd4goEngine(*timeout, diffcheck.WithMd4goFlags(md4goFlags)))

	// Optional: md4go-html engine (md4go→HTML→goquery text, same pipeline as goldmark)
	if *withMd4goHTML {
		engines = append(engines, diffcheck.NewMd4goHTMLEngine(*timeout, diffcheck.WithMd4goHTMLFlags(md4goFlags)))
	}

	// Optional: md4c-html engine (md4c→HTML→goquery text, same pipeline as goldmark)
	if *withMd4cHTML {
		md4cHexg, err := diffcheck.NewMd4cHTMLEngine(*timeout, md4cHTMLOpts...)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		} else {
			engines = append(engines, md4cHexg)
		}
	}

	md4cEng, err := diffcheck.NewMd4cEngine(*timeout, md4cOpts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	} else {
		engines = append(engines, md4cEng)
	}

	engines = append(engines, diffcheck.NewGoldmarkEngine(*timeout, goldmarkOpts...))

	if len(engines) < 2 {
		fmt.Fprintf(os.Stderr, "Need at least 2 engines, got %d\n", len(engines))
		os.Exit(1)
	}

	names := make([]string, len(engines))
	for i, e := range engines {
		names[i] = e.Name()
	}
	fmt.Fprintf(out, "Engines: %v\n\n", names)

	summary := &diffcheck.Summary{
		TotalCases: len(cases),
		PairStats:  make(map[string]int),
	}

	for _, tc := range cases {
		cr := diffcheck.RunCase(context.Background(), engines, tc, mode)

		// Track timeout/error
		if cr.HasError() {
			for engName, engErr := range cr.Errors {
				if engErr != nil {
					if diffcheck.IsTimeout(engErr) {
						summary.TimeoutCases++
					} else {
						summary.ErrorCases++
					}
					fmt.Fprintf(os.Stderr, "Case #%d [%s] %s: %v\n", tc.Index, tc.Source, engName, engErr)
				}
			}
		}

		if cr.HasDiff() {
			summary.DiffCases++
			fmt.Fprint(out, diffcheck.FormatCaseReport(cr))
			fmt.Fprintln(out)
		} else if *verbose {
			fmt.Fprintf(out, "Case #%d [%s]: all identical\n", tc.Index, tc.Source)
		}

		for _, p := range cr.Pairs {
			if p != nil && !p.Same {
				key := fmt.Sprintf("%s≠%s", p.NameA, p.NameB)
				summary.PairStats[key]++
			}
		}
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "━━━ Summary ━━━")
	fmt.Fprintln(out, summary.FormatSummary())

	if summary.DiffCases > 0 {
		os.Exit(1)
	}
}

// loadBuiltinCases loads cases from a built-in data source.
func loadBuiltinCases(src string) ([]diffcheck.TestCase, error) {
	switch src {
	case "jsonl":
		loaded, err := diffcheck.LoadAllJSONL()
		if err != nil {
			return nil, fmt.Errorf("load jsonl: %w", err)
		}
		return loaded, nil
	case "fuzz":
		return diffcheck.LoadFuzzSeeds(), nil
	case "constructed":
		return diffcheck.LoadFuzzConstructed(), nil
	case "all":
		loaded, err := diffcheck.LoadAllJSONL()
		if err != nil {
			return nil, fmt.Errorf("load jsonl: %w", err)
		}
		var cases []diffcheck.TestCase
		cases = append(cases, loaded...)
		cases = append(cases, diffcheck.LoadFuzzSeeds()...)
		cases = append(cases, diffcheck.LoadFuzzConstructed()...)
		return cases, nil
	default:
		return nil, fmt.Errorf("unknown source %q, use jsonl, fuzz, constructed, or all", src)
	}
}

func sourceLabel(stdinMode bool, filePath, src string) string {
	if stdinMode {
		return "stdin"
	}
	if filePath != "" {
		return "file:" + filePath
	}
	return src
}
