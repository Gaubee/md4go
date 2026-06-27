package parser

import (
	"testing"

	"github.com/userpro/md4go/ast"
	"github.com/userpro/md4go/renderer"
	"github.com/userpro/md4go/stream"
)

// stream_continue_test.go verifies ParseStreamContinue/End preserve block
// state across chunk boundaries — the core capability the continuation API
// adds over one-shot ParseStream (which resets context each call).

// lineListSource returns the given lines then signals end-of-source.
type lineListSource struct {
	lines [][]byte
	i     int
}

func (s *lineListSource) NextLine() ([]byte, bool, error) {
	if s.i >= len(s.lines) {
		return nil, false, nil
	}
	l := s.lines[s.i]
	s.i++
	return l, true, nil
}
func (s *lineListSource) LineNumber() int            { return s.i }
func (s *lineListSource) DocEndsWithNewline() bool   { return true }

// blockStateRec tracks the block span stack and classifies each Text event as
// protected (inside a code/html block) or top-level.
type blockStateRec struct {
	renderer.NopRenderer
	stack   []ast.BlockType
	protectedTexts []string
	liftTexts      []string
}

func (r *blockStateRec) EnterBlock(t ast.BlockType, _ any) error {
	r.stack = append(r.stack, t)
	return nil
}
func (r *blockStateRec) LeaveBlock(ast.BlockType, any) error {
	if len(r.stack) > 0 {
		r.stack = r.stack[:len(r.stack)-1]
	}
	return nil
}
func (r *blockStateRec) Text(_ ast.TextType, b []byte) error {
	protected := false
	for _, t := range r.stack {
		if t == ast.BlockCode || t == ast.BlockHTML {
			protected = true
			break
		}
	}
	if protected {
		r.protectedTexts = append(r.protectedTexts, string(b))
	} else {
		r.liftTexts = append(r.liftTexts, string(b))
	}
	return nil
}

// TestParseStreamContinue_CodeBlockAcrossChunks proves a fenced code block
// opened in one chunk and closed in another keeps its protected state for the
// body lines in between — the failure mode of one-shot ParseStream.
func TestParseStreamContinue_CodeBlockAcrossChunks(t *testing.T) {
	// Same logical document split into 3 chunks:
	//   chunk0: prose + code fence open
	//   chunk1: code body (must be protected even though it arrives in its own chunk)
	//   chunk2: code fence close + trailing prose
	chunks := [][][]byte{
		{[]byte("intro"), []byte("```")},
		{[]byte("<DSML-INSIDE>")},
		{[]byte("```"), []byte("after")},
	}
	p := New(0)
	rec := &blockStateRec{}
	for _, chunk := range chunks {
		if err := p.ParseStreamContinue(&lineListSource{lines: chunk}, rec); err != nil {
			t.Fatalf("Continue: %v", err)
		}
	}
	if err := p.ParseStreamEnd(rec); err != nil {
		t.Fatalf("End: %v", err)
	}

	// The code body must be classified protected; the prose must be lift.
	joinedProtected := join(rec.protectedTexts)
	joinedLift := join(rec.liftTexts)
	if !contains(joinedProtected, "<DSML-INSIDE>") {
		t.Fatalf("code body must be protected, protectedTexts=%q", rec.protectedTexts)
	}
	if !contains(joinedLift, "intro") || !contains(joinedLift, "after") {
		t.Fatalf("prose must be lift, liftTexts=%q", rec.liftTexts)
	}
	if contains(joinedLift, "<DSML-INSIDE>") {
		t.Fatalf("code body leaked into lift, liftTexts=%q", rec.liftTexts)
	}
}

// TestParseStreamContinue_ParagraphAcrossChunks proves a paragraph split across
// chunks is emitted as one paragraph (state preserved), not re-opened each chunk.
func TestParseStreamContinue_ParagraphAcrossChunks(t *testing.T) {
	chunks := [][][]byte{
		{[]byte("word1")},
		{[]byte("word2")},
		{[]byte("word3")},
	}
	p := New(0)
	rec := &blockStateRec{}
	for _, c := range chunks {
		if err := p.ParseStreamContinue(&lineListSource{lines: c}, rec); err != nil {
			t.Fatalf("Continue: %v", err)
		}
	}
	if err := p.ParseStreamEnd(rec); err != nil {
		t.Fatalf("End: %v", err)
	}
	joined := join(rec.liftTexts)
	for _, w := range []string{"word1", "word2", "word3"} {
		if !contains(joined, w) {
			t.Fatalf("%q missing from lift text: %q", w, joined)
		}
	}
}

// TestParseStreamContinue_MatchesOneShot proves the continuation API produces
// the same Text events as a one-shot ParseStream of the whole document, when the
// lines are fed incrementally.
func TestParseStreamContinue_MatchesOneShot(t *testing.T) {
	doc := [][]byte{
		[]byte("# Heading"),
		[]byte(""),
		[]byte("paragraph one"),
		[]byte(""),
		[]byte("```"),
		[]byte("code line"),
		[]byte("```"),
		[]byte(""),
		[]byte("> quote"),
		[]byte("> "),
		[]byte("> more"),
	}

	// One-shot baseline.
	oneShot := &blockStateRec{}
	p1 := New(0)
	if err := p1.ParseStream(&lineListSource{lines: doc}, oneShot); err != nil {
		t.Fatalf("one-shot: %v", err)
	}

	// Chunked continuation, one line per chunk.
	cont := &blockStateRec{}
	p2 := New(0)
	for _, line := range doc {
		if err := p2.ParseStreamContinue(&lineListSource{lines: [][]byte{line}}, cont); err != nil {
			t.Fatalf("Continue: %v", err)
		}
	}
	if err := p2.ParseStreamEnd(cont); err != nil {
		t.Fatalf("End: %v", err)
	}

	if join(cont.liftTexts) != join(oneShot.liftTexts) {
		t.Fatalf("lift text differs\n one-shot: %q\n continue: %q",
			join(oneShot.liftTexts), join(cont.liftTexts))
	}
	if join(cont.protectedTexts) != join(oneShot.protectedTexts) {
		t.Fatalf("protected text differs\n one-shot: %q\n continue: %q",
			join(oneShot.protectedTexts), join(cont.protectedTexts))
	}
}

// TestParseStreamContinue_ReusableAfterEnd proves End clears state so a fresh
// stream can be started on the same Parser.
func TestParseStreamContinue_ReusableAfterEnd(t *testing.T) {
	p := New(0)
	// First stream.
	rec1 := &blockStateRec{}
	_ = p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("first")}}, rec1)
	_ = p.ParseStreamEnd(rec1)
	// Second stream on the same parser.
	rec2 := &blockStateRec{}
	_ = p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("second")}}, rec2)
	_ = p.ParseStreamEnd(rec2)
	if !contains(join(rec2.liftTexts), "second") {
		t.Fatalf("second stream lost text: %q", rec2.liftTexts)
	}
	if contains(join(rec2.liftTexts), "first") {
		t.Fatalf("first stream leaked into second: %q", rec2.liftTexts)
	}
}

func join(ss []string) string {
	out := ""
	for _, s := range ss {
		out += s
	}
	return out
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ stream.LineSource = (*lineListSource)(nil)
