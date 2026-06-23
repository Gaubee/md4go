package parser

import (
	"testing"
)

// --- Mark structure tests ---

func TestMarkAddMark(t *testing.T) {
	ms := &markStacks{}
	ms.reset()
	idx := ms.addMark(0, 1, '*', markPotentialOpener)
	if idx != 0 {
		t.Errorf("first mark index = %d, want 0", idx)
	}
	if len(ms.marks) != 1 {
		t.Fatalf("marks len = %d, want 1", len(ms.marks))
	}
	m := ms.marks[0]
	if m.Beg != 0 || m.End != 1 || m.Ch != '*' || m.Flags != markPotentialOpener {
		t.Errorf("mark = {%d,%d,%q,%08b}, want {0,1,'*',00000001}", m.Beg, m.End, m.Ch, m.Flags)
	}
	if m.Prev != markSentinelIndex || m.Next != markSentinelIndex {
		t.Errorf("mark links = {prev=%d,next=%d}, want {-1,-1}", m.Prev, m.Next)
	}
}

func TestMarkResolveRange(t *testing.T) {
	ms := &markStacks{}
	ms.reset()
	opener := ms.addMark(0, 2, '*', markPotentialOpener)
	closer := ms.addMark(5, 7, '*', markPotentialCloser)
	ms.resolveRange(opener, closer)

	o := &ms.marks[opener]
	c := &ms.marks[closer]
	if o.Next != closer {
		t.Errorf("opener.Next = %d, want %d", o.Next, closer)
	}
	if c.Prev != opener {
		t.Errorf("closer.Prev = %d, want %d", c.Prev, opener)
	}
	if o.Flags&markOpener == 0 || o.Flags&markResolved == 0 {
		t.Errorf("opener flags = %08b, missing opener/resolved", o.Flags)
	}
	if c.Flags&markCloser == 0 || c.Flags&markResolved == 0 {
		t.Errorf("closer flags = %08b, missing closer/resolved", c.Flags)
	}
}

func TestMarkDisableMarks(t *testing.T) {
	ms := &markStacks{}
	ms.reset()
	ms.addMark(0, 1, '*', markPotentialOpener)
	ms.addMark(2, 3, '[', markPotentialOpener)
	ms.addMark(4, 5, ']', markPotentialCloser)
	ms.disableMarksInRange(1, 3)
	if ms.marks[1].Ch != 'D' || ms.marks[1].Flags != 0 {
		t.Errorf("mark[1] after disable = {%q, %08b}, want {'D', 0}", ms.marks[1].Ch, ms.marks[1].Flags)
	}
	if ms.marks[2].Ch != 'D' || ms.marks[2].Flags != 0 {
		t.Errorf("mark[2] after disable = {%q, %08b}, want {'D', 0}", ms.marks[2].Ch, ms.marks[2].Flags)
	}
	// mark[0] should be untouched
	if ms.marks[0].Ch != '*' {
		t.Errorf("mark[0] after disable = %q, want '*'", ms.marks[0].Ch)
	}
}

// --- Stack tests ---

func TestMarkStackPushPop(t *testing.T) {
	ms := &markStacks{}
	ms.reset()
	m0 := ms.addMark(0, 1, '*', markPotentialOpener|markEmphMod3_0)
	m1 := ms.addMark(2, 3, '*', markPotentialOpener|markEmphMod3_1)

	// Push onto asterisk OO mod3_0 stack
	stackIdx := emphStack('*', markPotentialOpener|markEmphMod3_0)
	ms.push(stackIdx, m0)
	ms.push(stackIdx, m1)

	// Pop should return LIFO order
	top := ms.pop(stackIdx)
	if top != m1 {
		t.Errorf("first pop = %d, want %d", top, m1)
	}
	top = ms.pop(stackIdx)
	if top != m0 {
		t.Errorf("second pop = %d, want %d", top, m0)
	}
	top = ms.pop(stackIdx)
	if top != markSentinelIndex {
		t.Errorf("empty pop = %d, want -1", top)
	}
}

func TestEmphStackIndex(t *testing.T) {
	tests := []struct {
		ch     byte
		flags  markFlags
		expect int
	}{
		{'*', markPotentialOpener | markEmphMod3_0, asteriskOO0},
		{'*', markPotentialOpener | markEmphMod3_1, asteriskOO1},
		{'*', markPotentialOpener | markEmphMod3_2, asteriskOO2},
		{'*', markPotentialOpener | markEmphOC | markEmphMod3_0, asteriskOC0},
		{'*', markPotentialOpener | markEmphOC | markEmphMod3_1, asteriskOC1},
		{'*', markPotentialOpener | markEmphOC | markEmphMod3_2, asteriskOC2},
		{'_', markPotentialOpener | markEmphMod3_0, underscoreOO0},
		{'_', markPotentialOpener | markEmphOC | markEmphMod3_2, underscoreOC2},
	}
	for _, tt := range tests {
		got := emphStack(tt.ch, tt.flags)
		if got != tt.expect {
			t.Errorf("emphStack(%q, %08b) = %d, want %d", tt.ch, tt.flags, got, tt.expect)
		}
	}
}

func TestMarkStackReset(t *testing.T) {
	ms := &markStacks{}
	ms.addMark(0, 1, '*', markPotentialOpener)
	ms.push(asteriskOO0, 0)
	ms.reset()
	if len(ms.marks) != 0 {
		t.Errorf("marks len after reset = %d, want 0", len(ms.marks))
	}
	for i := range ms.stacks {
		if ms.stacks[i] != markSentinelIndex {
			t.Errorf("stack[%d] after reset = %d, want -1", i, ms.stacks[i])
		}
	}
}

func TestPopOpeners(t *testing.T) {
	ms := &markStacks{}
	ms.reset()
	m0 := ms.addMark(0, 1, '*', markPotentialOpener)
	m1 := ms.addMark(2, 3, '*', markPotentialOpener)
	m2 := ms.addMark(4, 5, '*', markPotentialOpener)
	ms.push(asteriskOO0, m0)
	ms.push(asteriskOO0, m1)
	ms.push(asteriskOO0, m2)

	// Pop all openers >= m1
	ms.popOpeners(m1)

	top := ms.pop(asteriskOO0)
	if top != m0 {
		t.Errorf("after popOpeners(1), top = %d, want %d", top, m0)
	}
	top = ms.pop(asteriskOO0)
	if top != markSentinelIndex {
		t.Errorf("stack should be empty, got %d", top)
	}
}

// --- collectMarks tests ---

func TestCollectMarksBackslashEscape(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte(`foo\*bar`), &markCharMap, 0)
	// Should have: resolved '\' mark + sentinel
	found := false
	for _, m := range ms.marks {
		if m.Ch == '\\' && m.Flags&markResolved != 0 {
			found = true
			if m.Beg != 3 || m.End != 5 {
				t.Errorf("backslash mark = {%d,%d}, want {3,5}", m.Beg, m.End)
			}
		}
	}
	if !found {
		t.Error("no resolved backslash mark found")
	}
}

func TestCollectMarksEmphAsterisk(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantOO  int    // opener-only marks
		wantOC  int    // opener+closer marks
		wantMod [3]int // mod3 counts
	}{
		{
			name:   "left-flanking opener",
			input:  "*foo",
			wantOO: 1,
			wantOC: 0,
		},
		{
			name:   "right-flanking closer",
			input:  "foo*",
			wantOO: 0,
			wantOC: 0, // it's closer-only, not OC
		},
		{
			name:   "intra-word underscore not opener",
			input:  "foo_bar",
			wantOO: 0,
			wantOC: 0,
		},
		{
			name:   "underscore with spaces is not emphasis",
			input:  "foo _ bar",
			wantOO: 0,
			wantOC: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := &markStacks{}
			collectMarks(ms, []byte(tt.input), &markCharMap, 0)
			ooCount := 0
			ocCount := 0
			for _, m := range ms.marks {
				if m.Ch == '*' || m.Ch == '_' {
					if m.Flags&markPotentialOpener != 0 && m.Flags&markPotentialCloser == 0 {
						ooCount++
					}
					if m.Flags&markEmphOC != 0 {
						ocCount++
					}
				}
			}
			if ooCount != tt.wantOO {
				t.Errorf("opener-only count = %d, want %d", ooCount, tt.wantOO)
			}
			if ocCount != tt.wantOC {
				t.Errorf("opener+closer count = %d, want %d", ocCount, tt.wantOC)
			}
		})
	}
}

func TestCollectMarksCodeSpan(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("foo `code` bar"), &markCharMap, 0)

	// Should have resolved code span
	found := false
	for _, m := range ms.marks {
		if m.Ch == '`' && m.Flags&markResolved != 0 && m.Flags&markOpener != 0 {
			found = true
			closerIdx := m.Next
			if closerIdx < 0 || closerIdx >= len(ms.marks) {
				t.Errorf("code span opener.Next = %d, invalid", closerIdx)
			} else {
				closer := ms.marks[closerIdx]
				if closer.Flags&markCloser == 0 {
					t.Error("code span closer missing CLOSER flag")
				}
			}
		}
	}
	if !found {
		t.Error("no resolved code span found")
	}
}

func TestCollectMarksCodeSpanDoubleBacktick(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("``code``"), &markCharMap, 0)

	// Double backtick code span should resolve
	found := false
	for _, m := range ms.marks {
		if m.Ch == '`' && m.Flags&markResolved != 0 && m.Flags&markOpener != 0 {
			found = true
			if m.End-m.Beg != 2 {
				t.Errorf("opener len = %d, want 2", m.End-m.Beg)
			}
		}
	}
	if !found {
		t.Error("no resolved double-backtick code span found")
	}
}

func TestCollectMarksUnmatchedBacktick(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("foo `bar"), &markCharMap, 0)

	// Unmatched backtick should NOT be resolved
	for _, m := range ms.marks {
		if m.Ch == '`' && m.Flags&markResolved != 0 {
			t.Error("unmatched backtick should not be resolved")
		}
	}
}

func TestCollectMarksBrackets(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("[text](url)"), &markCharMap, 0)

	// Should have '[' opener and ']' closer
	var foundOpen, foundClose bool
	for _, m := range ms.marks {
		if m.Ch == '[' && m.Flags&markPotentialOpener != 0 {
			foundOpen = true
		}
		if m.Ch == ']' && m.Flags&markPotentialCloser != 0 {
			foundClose = true
		}
	}
	if !foundOpen {
		t.Error("no '[' opener found")
	}
	if !foundClose {
		t.Error("no ']' closer found")
	}
}

func TestCollectMarksImageBracket(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("![alt](url)"), &markCharMap, 0)

	// '[' after '!' should have CANBEIMAGE flag
	found := false
	for _, m := range ms.marks {
		if m.Ch == '[' && m.Flags&markBracketCanBeImage != 0 {
			found = true
		}
	}
	if !found {
		t.Error("no image bracket (CANBEIMAGE) found")
	}
}

func TestCollectMarksEntity(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("foo &amp; bar"), &markCharMap, 0)

	var foundAmp, foundSemi bool
	for _, m := range ms.marks {
		if m.Ch == '&' && m.Flags&markPotentialOpener != 0 {
			foundAmp = true
		}
		if m.Ch == ';' && m.Flags&markPotentialCloser != 0 {
			foundSemi = true
		}
	}
	if !foundAmp {
		t.Error("no '&' opener found")
	}
	if !foundSemi {
		t.Error("no ';' closer found")
	}
}

func TestCollectMarksTilde(t *testing.T) {
	ms := &markStacks{}
	mc := buildMarkChars(FlagStrikethrough)
	collectMarks(ms, []byte("~~strike~~"), &mc, FlagStrikethrough)

	// Double tilde should create a mark with length 2
	found := false
	for _, m := range ms.marks {
		if m.Ch == '~' && m.End-m.Beg == 2 {
			found = true
		}
	}
	if !found {
		t.Error("no double-tilde mark found")
	}
}

func TestCollectMarksSentinel(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("hello"), &markCharMap, 0)

	last := ms.marks[len(ms.marks)-1]
	if last.Ch != 127 {
		t.Errorf("sentinel ch = %d, want 127", last.Ch)
	}
	if last.Flags&markResolved == 0 {
		t.Error("sentinel should be resolved")
	}
	if last.Beg != 5 || last.End != 5 {
		t.Errorf("sentinel = {%d,%d}, want {5,5}", last.Beg, last.End)
	}
}

func TestCollectMarksNoMarks(t *testing.T) {
	ms := &markStacks{}
	collectMarks(ms, []byte("hello world"), &markCharMap, 0)

	// Should only have the sentinel
	if len(ms.marks) != 1 {
		t.Errorf("marks count = %d, want 1 (sentinel only)", len(ms.marks))
	}
}

// --- charFlankLevel tests ---

func TestCharFlankLevel(t *testing.T) {
	tests := []struct {
		text   string
		off    int
		dir    int
		expect int
	}{
		{"*foo", 0, -1, 0}, // before *: boundary → 0
		{"*foo", 0, 1, 2},  // after *: 'f' → 2 (other)
		{"foo*", 3, -1, 2}, // before *: 'o' → 2 (other)
		{"foo*", 3, 1, 0},  // after *: boundary → 0
		{"*!", 0, 1, 1},    // after *: '!' → 1 (punct)
		{"!*", 1, -1, 1},   // before *: '!' → 1 (punct)
		{" *", 1, -1, 0},   // before *: ' ' → 0 (whitespace)
		{"* ", 0, 1, 0},    // after *: ' ' → 0 (whitespace)
	}
	for _, tt := range tests {
		got := charFlankLevel([]byte(tt.text), tt.off, tt.dir)
		if got != tt.expect {
			t.Errorf("charFlankLevel(%q, %d, %d) = %d, want %d", tt.text, tt.off, tt.dir, got, tt.expect)
		}
	}
}

// --- isASCIIPunct tests ---

func TestIsASCIIPunct(t *testing.T) {
	if !isASCIIPunct('!') || !isASCIIPunct('@') || !isASCIIPunct('~') {
		t.Error("expected punctuation chars to return true")
	}
	if isASCIIPunct('a') || isASCIIPunct('0') || isASCIIPunct(' ') {
		t.Error("expected non-punctuation chars to return false")
	}
}

// --- codespanMaxLen test ---

func TestCodeSpanMaxLen(t *testing.T) {
	// Create a long run of backticks
	longRun := make([]byte, 40)
	for i := range longRun {
		longRun[i] = '`'
	}
	ms := &markStacks{}
	collectMarks(ms, longRun, &markCharMap, 0)
	// Mark length should be capped at codespanMaxLen
	for _, m := range ms.marks {
		if m.Ch == '`' && m.End-m.Beg > codespanMaxLen {
			t.Errorf("backtick mark length = %d, exceeds max %d", m.End-m.Beg, codespanMaxLen)
		}
	}
}
