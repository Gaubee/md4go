package parser_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/userpro/md4go/html"
	"github.com/userpro/md4go/parser"
)

// TestEmailAutolinkWithUnderscore verifies that _ in email usernames
// does NOT prevent permissive email autolink detection.
// This is the known bug from DIFFCHECK_REPORT.md §3.5.1.
func TestEmailAutolinkWithUnderscore(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"user_name@d.com\n"},
		{"hello_world user_name@d.com\n"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p := parser.New(parser.DialectGitHub)
			if err := p.Parse([]byte(tc.input), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			h.Flush()
			got := buf.String()
			t.Logf("Output: %q", got)
			if !strings.Contains(got, "mailto:") {
				t.Errorf("Expected mailto link in output, got: %q", got)
			}
			if !strings.Contains(got, "user_name@d.com") {
				t.Errorf("Expected full email in link, got: %q", got)
			}
		})
	}
}

// TestEmailAutolinkInTable verifies email autolinks inside table cells.
func TestEmailAutolinkInTable(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"| Name |\n|---|---|\n| user_name@d.com |\n"},
		{"| Name | Email |\n|---|---|\n| Foo | user_name@d.com |\n"},
	}

	for _, tc := range tests {
		t.Run("table", func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p := parser.New(parser.DialectGitHub)
			if err := p.Parse([]byte(tc.input), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			h.Flush()
			got := buf.String()
			t.Logf("Table output: %q", got)
			if !strings.Contains(got, "mailto:") {
				t.Errorf("Expected mailto link in table, got: %q", got)
			}
		})
	}
}

// TestAllEmailDelimitersWork verifies all valid email username delimiters.
func TestAllEmailDelimitersWork(t *testing.T) {
	delimiters := []string{"-", "+", ".", "_"}
	for _, delim := range delimiters {
		email := "user" + delim + "name@d.com"
		t.Run(email, func(t *testing.T) {
			var buf bytes.Buffer
			h := html.NewHTML(&buf)
			p := parser.New(parser.DialectGitHub)
			if err := p.Parse([]byte(email+"\n"), h); err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			h.Flush()
			got := buf.String()
			if !strings.Contains(got, "mailto:") {
				t.Errorf("Delimiter %q: expected mailto link, got: %q", delim, got)
			}
		})
	}
}
