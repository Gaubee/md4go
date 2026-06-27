package parser

import (
	"errors"
	"slices"
	"strings"
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
func (s *lineListSource) LineNumber() int          { return s.i }
func (s *lineListSource) DocEndsWithNewline() bool { return true }

type failDocEnterRenderer struct {
	renderer.NopRenderer
	err error
}

func (r *failDocEnterRenderer) EnterBlock(t ast.BlockType, _ any) error {
	if t == ast.BlockDoc {
		return r.err
	}
	return nil
}

// blockStateRec tracks the block span stack and classifies each Text event as
// protected (inside a code/html block) or top-level.
type blockStateRec struct {
	renderer.NopRenderer
	stack          []ast.BlockType
	entered        []ast.BlockType
	left           []ast.BlockType
	protectedTexts []string
	liftTexts      []string
}

func (r *blockStateRec) EnterBlock(t ast.BlockType, _ any) error {
	r.entered = append(r.entered, t)
	r.stack = append(r.stack, t)
	return nil
}
func (r *blockStateRec) LeaveBlock(t ast.BlockType, _ any) error {
	r.left = append(r.left, t)
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

func countBlock(events []ast.BlockType, target ast.BlockType) int {
	count := 0
	for _, event := range events {
		if event == target {
			count++
		}
	}
	return count
}

// TestParseStreamContinue_CodeBlockAcrossChunks proves a fenced code block
// opened in one chunk and closed in another keeps its protected state for the
// body lines in between — the failure mode of one-shot ParseStream.
func TestParseStreamContinue_CodeBlockAcrossChunks(t *testing.T) {
	t.Parallel()
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
	joinedProtected := strings.Join(rec.protectedTexts, "")
	joinedLift := strings.Join(rec.liftTexts, "")
	if !strings.Contains(joinedProtected, "<DSML-INSIDE>") {
		t.Fatalf("code body must be protected, protectedTexts=%q", rec.protectedTexts)
	}
	if !strings.Contains(joinedLift, "intro") || !strings.Contains(joinedLift, "after") {
		t.Fatalf("prose must be lift, liftTexts=%q", rec.liftTexts)
	}
	if strings.Contains(joinedLift, "<DSML-INSIDE>") {
		t.Fatalf("code body leaked into lift, liftTexts=%q", rec.liftTexts)
	}
}

// TestParseStreamContinue_ParagraphAcrossChunks proves a paragraph split across
// chunks is emitted as one paragraph (state preserved), not re-opened each chunk.
func TestParseStreamContinue_ParagraphAcrossChunks(t *testing.T) {
	t.Parallel()
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
	joined := strings.Join(rec.liftTexts, "")
	for _, w := range []string{"word1", "word2", "word3"} {
		if !strings.Contains(joined, w) {
			t.Fatalf("%q missing from lift text: %q", w, joined)
		}
	}
	if got := countBlock(rec.entered, ast.BlockP); got != 1 {
		t.Fatalf("paragraph must enter once across chunks, got %d enters: %v", got, rec.entered)
	}
	if got := countBlock(rec.left, ast.BlockP); got != 1 {
		t.Fatalf("paragraph must leave once across chunks, got %d leaves: %v", got, rec.left)
	}
}

// TestParseStreamContinue_MatchesOneShot proves the continuation API produces
// the same Text events as a one-shot ParseStream of the whole document, when the
// lines are fed incrementally. Comparison is element-wise (slices.Equal), not
// joined-string, so fragment-boundary differences are caught (a joined compare
// would treat ["a","b"] and ["ab"] as equal — a false negative).
func TestParseStreamContinue_MatchesOneShot(t *testing.T) {
	t.Parallel()
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

	if !slices.Equal(cont.liftTexts, oneShot.liftTexts) {
		t.Fatalf("lift text differs (element-wise)\n one-shot: %q\n continue: %q",
			oneShot.liftTexts, cont.liftTexts)
	}
	if !slices.Equal(cont.protectedTexts, oneShot.protectedTexts) {
		t.Fatalf("protected text differs (element-wise)\n one-shot: %q\n continue: %q",
			oneShot.protectedTexts, cont.protectedTexts)
	}
}

// TestParseStreamContinue_ReusableAfterEnd proves End clears state so a fresh
// stream can be started on the same Parser.
func TestParseStreamContinue_ReusableAfterEnd(t *testing.T) {
	t.Parallel()
	p := New(0)
	// First stream.
	rec1 := &blockStateRec{}
	_ = p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("first")}}, rec1)
	_ = p.ParseStreamEnd(rec1)
	// Second stream on the same parser.
	rec2 := &blockStateRec{}
	_ = p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("second")}}, rec2)
	_ = p.ParseStreamEnd(rec2)
	joined2 := strings.Join(rec2.liftTexts, "")
	if !strings.Contains(joined2, "second") {
		t.Fatalf("second stream lost text: %q", rec2.liftTexts)
	}
	if strings.Contains(joined2, "first") {
		t.Fatalf("first stream leaked into second: %q", rec2.liftTexts)
	}
}

func TestParseStreamContinue_EmptyFirstChunkPreservesBOMStripping(t *testing.T) {
	t.Parallel()
	p := New(FlagStripBOM)
	rec := &blockStateRec{}

	if err := p.ParseStreamContinue(&lineListSource{}, rec); err != nil {
		t.Fatalf("empty Continue: %v", err)
	}
	if err := p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("\xef\xbb\xbfHello")}}, rec); err != nil {
		t.Fatalf("Continue with first real line: %v", err)
	}
	if err := p.ParseStreamEnd(rec); err != nil {
		t.Fatalf("End: %v", err)
	}

	joined := strings.Join(rec.liftTexts, "")
	if strings.Contains(joined, "\ufeff") {
		t.Fatalf("BOM should be stripped from first real line after empty chunk: %q", joined)
	}
	if !strings.Contains(joined, "Hello") {
		t.Fatalf("first real line text missing: %q", joined)
	}
}

func TestParseStreamContinue_DocEnterErrorDoesNotStartStream(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("doc enter failed")
	p := New(0)

	err := p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("lost")}}, &failDocEnterRenderer{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if p.InProtectedBlock() {
		t.Fatal("failed Doc entry must not leave a started protected-block state")
	}

	rec := &blockStateRec{}
	if err := p.ParseStreamContinue(&lineListSource{lines: [][]byte{[]byte("fresh")}}, rec); err != nil {
		t.Fatalf("retry Continue after failed Doc entry: %v", err)
	}
	if err := p.ParseStreamEnd(rec); err != nil {
		t.Fatalf("End: %v", err)
	}
	joined := strings.Join(rec.liftTexts, "")
	if !strings.Contains(joined, "fresh") {
		t.Fatalf("fresh retry text missing: %q", joined)
	}
	if strings.Contains(joined, "lost") {
		t.Fatalf("failed stream input leaked into retry: %q", joined)
	}
}

var _ stream.LineSource = (*lineListSource)(nil)
