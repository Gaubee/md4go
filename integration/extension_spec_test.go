package integration_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"md4go"
	"md4go/extension"
	"md4go/html"
	"md4go/parser"
)

// extSpecCase represents a single fenced example from md4c spec-*.txt files.
type extSpecCase struct {
	Markdown    string
	HTML        string
	Options     string // e.g. "--ftables --fstrikethrough"
	Section     string
	Example     int
	NoNormalize bool
}

// parseExtSpec parses a md4c fenced example spec file (spec-*.txt, regressions.txt).
// Format mirrors md4c run-testsuite.py get_tests():
//
//	```````````````````````````````` example [no-normalize]
//	markdown input
//	.
//	expected HTML output
//	.
//	--flag1 --flag2
//	````````````````````````````````
func parseExtSpec(t *testing.T, path string) []extSpecCase {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read spec %s: %v", path, err)
	}

	var cases []extSpecCase
	lines := strings.Split(string(data), "\n")

	state := 0 // 0=text, 1=markdown, 2=html, 3=options
	exampleNum := 0
	var mdLines, htmlLines, optLines []string
	var section string
	noNormalize := false
	fenceRe := regexp.MustCompile("^`{32} example( [a-z\\-]+)?$")
	headerRe := regexp.MustCompile("^#+ ")

	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")

		if state == 0 && fenceRe.MatchString(trimmed) {
			state = 1
			noNormalize = strings.Contains(trimmed, "[no-normalize]")
			mdLines = nil
			htmlLines = nil
			optLines = nil
			continue
		}

		if state >= 2 && trimmed == strings.Repeat("`", 32) {
			// End of example
			exampleNum++
			cases = append(cases, extSpecCase{
				Markdown:    strings.Join(mdLines, "\n"),
				HTML:        strings.Join(htmlLines, "\n"),
				Options:     strings.Join(optLines, "\n"),
				Section:     section,
				Example:     exampleNum,
				NoNormalize: noNormalize,
			})
			state = 0
			continue
		}

		switch state {
		case 1:
			if trimmed == "." {
				state = 2
			} else {
				// Replace → with tab (md4c convention)
				mdLines = append(mdLines, strings.ReplaceAll(line, "→", "\t"))
			}
		case 2:
			if trimmed == "." {
				state = 3
			} else {
				htmlLines = append(htmlLines, strings.ReplaceAll(line, "→", "\t"))
			}
		case 3:
			optLines = append(optLines, trimmed)
		case 0:
			if headerRe.MatchString(trimmed) {
				section = headerRe.ReplaceAllString(trimmed, "")
				section = strings.TrimSpace(section)
			}
		}
	}

	return cases
}

// buildConfigFromOptions maps md4c command-line flags to Go parser flags + extenders.
// Mirrors md4c md2html/cmdline.c flag parsing.
func buildConfigFromOptions(opts string) (parser.Flags, []parser.Extender) {
	var flags parser.Flags
	var extenders []parser.Extender

	// Split options by whitespace and newlines
	for _, opt := range strings.Fields(opts) {
		switch opt {
		case "--ftables":
			flags |= parser.FlagTables
			extenders = append(extenders, &extension.Table{})
		case "--fstrikethrough":
			flags |= parser.FlagStrikethrough
			extenders = append(extenders, &extension.Strikethrough{})
		case "--ftasklists":
			flags |= parser.FlagTasklists
			extenders = append(extenders, &extension.TaskList{})
		case "--fpermissive-email-autolinks":
			flags |= parser.FlagPermissiveEmailAutolinks
			extenders = append(extenders, &extension.PermissiveAutolinks{})
		case "--fpermissive-url-autolinks":
			flags |= parser.FlagPermissiveURLAutolinks
			extenders = append(extenders, &extension.PermissiveAutolinks{})
		case "--fpermissive-www-autolinks":
			flags |= parser.FlagPermissiveWWWAutolinks
			extenders = append(extenders, &extension.PermissiveAutolinks{})
		case "--fpermissive-autolinks":
			// md4c shorthand for all three permissive autolink types
			flags |= parser.PermissiveAutolinks
			extenders = append(extenders, &extension.PermissiveAutolinks{})
		case "--ffootnotes":
			flags |= parser.FlagFootnotes
			extenders = append(extenders, &extension.Footnote{})
		case "--flatex-math":
			flags |= parser.FlagLatexMathSpans
			extenders = append(extenders, &extension.LatexMath{})
		case "--fwiki-links":
			flags |= parser.FlagWikilinks
			extenders = append(extenders, &extension.Wikilink{})
		case "--funderline":
			flags |= parser.FlagUnderline
		case "--fhard-soft-breaks":
			flags |= parser.FlagHardSoftBreaks
		case "--fspoilers":
			flags |= parser.FlagSpoilers
			extenders = append(extenders, &extension.Spoiler{})
		case "--fsuperscripts":
			flags |= parser.FlagSuperscripts
			extenders = append(extenders, &extension.Superscript{})
		case "--fsubscripts":
			flags |= parser.FlagSubscripts
			extenders = append(extenders, &extension.Subscript{})
		case "--fadmonitions":
			flags |= parser.FlagAdmonitions
			extenders = append(extenders, &extension.Admonition{})
		case "--fhighlight":
			flags |= parser.FlagHighlight
			extenders = append(extenders, &extension.Highlight{})
		case "--fcollapse-whitespace":
			flags |= parser.FlagCollapseWhitespace
		case "--fpermissive-atx-headers":
			flags |= parser.FlagPermissiveATXHeaders
		case "--fno-indented-code-blocks":
			flags |= parser.FlagNoIndentedCodeBlocks
		case "--fno-html-blocks":
			flags |= parser.FlagNoHTMLBlocks
		case "--fno-html-spans":
			flags |= parser.FlagNoHTMLSpans
		case "--github":
			// MD_DIALECT_GITHUB equivalent
			flags |= parser.DialectGitHub
			extenders = append(extenders, extension.GFM...)
		}
	}

	return flags, extenders
}

// runExtSpecTests runs extension spec tests for a given spec file.
func runExtSpecTests(t *testing.T, specFile string) {
	cases := parseExtSpec(t, specFile)
	fileName := filepath.Base(specFile)

	passed := 0
	failed := 0
	skipped := 0

	for _, c := range cases {
		flags, extenders := buildConfigFromOptions(c.Options)

		// Deduplicate extenders (multiple flags may add same extender)
		extenders = dedupExtenders(extenders)

		t.Run(fmt.Sprintf("%s/Example_%d", fileName, c.Example), func(t *testing.T) {
			opts := []md4go.Option{md4go.WithFlags(flags)}
			if len(extenders) > 0 {
				opts = append(opts, md4go.WithExtensions(extenders...))
			}
			md := md4go.New(opts...)

			var got bytes.Buffer
			// Use HTML5 mode (no XHTML) to match md4c's test runner behavior.
			// md4c spec files expect HTML5 output (bare boolean attributes, no /> on void elements).
			h := html.NewWithFlags(&got, 0)
			if err := md.Parse([]byte(c.Markdown), h); err != nil {
				t.Fatalf("parse error: %v", err)
			}
			_ = h.Flush()

			gotStr := got.String()
			wantStr := c.HTML

			// md4c's test runner expects each output line to end with \n.
			// Our parseExtSpec joins lines with \n but doesn't add a trailing one.
			// Normalize by stripping a single trailing \n from both sides so that
			// non-block-level HTML tags like <gi> compare correctly.
			gotStr = strings.TrimSuffix(gotStr, "\n")
			wantStr = strings.TrimSuffix(wantStr, "\n")

			if !c.NoNormalize {
				gotStr = normalizeHTML(gotStr)
				wantStr = normalizeHTML(wantStr)
			}

			if gotStr != wantStr {
				failed++
				t.Errorf("section=%s example=%d\n--- options ---\n%s\n--- input ---\n%q\n--- want ---\n%q\n--- got ---\n%q",
					c.Section, c.Example, c.Options, c.Markdown, wantStr, gotStr)
			} else {
				passed++
			}
		})
	}

	t.Logf("%s: %d passed, %d failed, %d skipped", fileName, passed, failed, skipped)
}

// dedupExtenders removes duplicate extender types.
func dedupExtenders(exts []parser.Extender) []parser.Extender {
	seen := make(map[string]bool)
	var result []parser.Extender
	for _, e := range exts {
		key := fmt.Sprintf("%T", e)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

// TestGFMAndExtensionSpec runs all md4c spec-*.txt extension spec files.
// Corresponds to SOP §5.3 I19/I41 and md4c test/run-testsuite.py.
func TestGFMAndExtensionSpec(t *testing.T) {
	specDir := "../testdata/spec"
	specFiles, err := filepath.Glob(filepath.Join(specDir, "spec-*.txt"))
	if err != nil {
		t.Fatalf("glob spec files: %v", err)
	}
	if len(specFiles) == 0 {
		t.Fatal("no spec-*.txt files found in ../testdata/spec/")
	}

	for _, sf := range specFiles {
		sf := sf
		t.Run(filepath.Base(sf), func(t *testing.T) {
			runExtSpecTests(t, sf)
		})
	}
}

// TestRegressions runs md4c's regressions.txt against the Go implementation.
// Corresponds to SOP §6.1 item 4 and md4c test/regressions.txt.
func TestRegressions(t *testing.T) {
	regFile := "../testdata/spec/regressions.txt"
	if _, err := os.Stat(regFile); os.IsNotExist(err) {
		t.Skip("regressions.txt not found")
	}
	runExtSpecTests(t, regFile)
}
