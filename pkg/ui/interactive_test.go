package ui

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"golang.org/x/term"
)

func TestDrawMenu_SingleSelect_CursorAtFirst(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	drawMenu("Choose:", []string{"alpha", "beta", "gamma"}, 0, nil, false)
	s := buf.String()
	if !strings.Contains(s, "Choose:") {
		t.Errorf("missing prompt in output: %q", s)
	}
	if !strings.Contains(s, "enter to confirm") {
		t.Errorf("missing hint: %q", s)
	}
}

func TestDrawMenu_SingleSelect_CursorAtLast(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	drawMenu("Choose:", []string{"alpha", "beta", "gamma"}, 2, nil, false)
	s := buf.String()
	if !strings.Contains(s, "gamma") {
		t.Errorf("expected gamma in output: %q", s)
	}
}

func TestDrawMenu_MultiSelect_AllSelected(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	sel := []bool{true, true, true}
	drawMenu("Pick all:", []string{"a", "b", "c"}, 0, sel, false)
	s := buf.String()
	if !strings.Contains(s, "space to toggle") {
		t.Errorf("missing toggle hint in multi-select: %q", s)
	}
}

func TestDrawMenu_MultiSelect_NoneSelected(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	sel := []bool{false, false}
	drawMenu("Pick:", []string{"a", "b"}, 1, sel, false)
	s := buf.String()
	if !strings.Contains(s, "a") || !strings.Contains(s, "b") {
		t.Errorf("expected options in output: %q", s)
	}
}

func TestDrawMenu_Final_SingleSelect_FirstItem(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	drawMenu("Done:", []string{"first", "second", "third"}, 0, nil, true)
	s := buf.String()
	if !strings.Contains(s, "first") {
		t.Errorf("final should show selected item 'first': %q", s)
	}
}

func TestDrawMenu_Final_MultiSelect_AllSelected(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	sel := []bool{true, true}
	drawMenu("Done:", []string{"x", "y"}, 0, sel, true)
	s := buf.String()
	if !strings.Contains(s, "x") || !strings.Contains(s, "y") {
		t.Errorf("expected both selected items: %q", s)
	}
}

func TestClearMenu_SingleLine(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	clearMenu(1)
	s := buf.String()
	if !strings.Contains(s, "\x1b[1A") {
		t.Errorf("expected ANSI cursor-up sequence: %q", s)
	}
	if !strings.Contains(s, "\x1b[2K") {
		t.Errorf("expected ANSI clear-line sequence: %q", s)
	}
}

func TestClearMenu_MultipleLines(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	clearMenu(5)
	s := buf.String()
	count := strings.Count(s, "\x1b[1A")
	if count != 5 {
		t.Errorf("expected 5 cursor-up sequences, got %d", count)
	}
}

func TestDecodeKey_AllKeys(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  key
	}{
		{"ctrl-c", []byte{0x03}, keyCtrlC},
		{"enter-cr", []byte{'\r'}, keyEnter},
		{"enter-lf", []byte{'\n'}, keyEnter},
		{"space", []byte{' '}, keySpace},
		{"escape", []byte{0x1b}, keyEsc},
		{"arrow-up", []byte{0x1b, '[', 'A'}, keyUp},
		{"arrow-down", []byte{0x1b, '[', 'B'}, keyDown},
		{"arrow-right", []byte{0x1b, '[', 'C'}, keyUnknown},
		{"arrow-left", []byte{0x1b, '[', 'D'}, keyUnknown},
		{"regular-char", []byte{'x'}, keyUnknown},
		{"empty", []byte{}, keyUnknown},
		{"two-byte-unknown", []byte{0x1b, 'O'}, keyUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeKey(tt.input)
			if got != tt.want {
				t.Errorf("decodeKey(%v) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestClearMenu_Zero(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	clearMenu(0)
	s := buf.String()
	// Only the initial \r should be present, no cursor-up sequences
	if strings.Contains(s, "\x1b[1A") {
		t.Errorf("expected no cursor-up for n=0: %q", s)
	}
}

// =============================================================================
// interactiveSelect tests using test hooks
// =============================================================================

// keystrokeReader delivers input one keystroke at a time: escape sequences
// (3 bytes) are delivered as a single Read, and single-byte keys as 1.
type keystrokeReader struct {
	data []byte
	pos  int
}

func (r *keystrokeReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, errors.New("EOF")
	}
	// If current byte is ESC and there are at least 3 bytes, deliver 3 at once
	if r.data[r.pos] == 0x1b && r.pos+2 < len(r.data) {
		n := copy(p, r.data[r.pos:r.pos+3])
		r.pos += 3
		return n, nil
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

func withMockTerminal(input []byte, fn func()) {
	oldMakeRaw := makeRawFn
	oldRestore := restoreFn
	oldReader := stdinReader
	oldOut := PromptOut
	defer func() {
		makeRawFn = oldMakeRaw
		restoreFn = oldRestore
		stdinReader = oldReader
		PromptOut = oldOut
	}()

	makeRawFn = func() (*term.State, error) { return nil, nil }
	restoreFn = func(_ *term.State) error { return nil }
	stdinReader = &keystrokeReader{data: input}
	PromptOut = &bytes.Buffer{}
	fn()
}

func TestInteractiveSelect_Enter(t *testing.T) {
	withMockTerminal([]byte{'\r'}, func() {
		idx, err := interactiveSelect("pick", []string{"a", "b", "c"}, nil)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if idx != 0 {
			t.Errorf("got idx=%d, want 0", idx)
		}
	})
}

func TestInteractiveSelect_DownThenEnter(t *testing.T) {
	withMockTerminal([]byte{0x1b, '[', 'B', '\r'}, func() {
		idx, err := interactiveSelect("pick", []string{"a", "b", "c"}, nil)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if idx != 1 {
			t.Errorf("got idx=%d, want 1", idx)
		}
	})
}

func TestInteractiveSelect_DownThenUp(t *testing.T) {
	// Move down (cursor=1), then up (cursor=0, hitting cursor-- branch), then enter
	withMockTerminal([]byte{0x1b, '[', 'B', 0x1b, '[', 'A', '\r'}, func() {
		idx, err := interactiveSelect("pick", []string{"a", "b", "c"}, nil)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if idx != 0 {
			t.Errorf("got idx=%d, want 0", idx)
		}
	})
}

func TestInteractiveSelect_UpWraps(t *testing.T) {
	withMockTerminal([]byte{0x1b, '[', 'A', '\r'}, func() {
		idx, err := interactiveSelect("pick", []string{"a", "b", "c"}, nil)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if idx != 2 {
			t.Errorf("got idx=%d, want 2", idx)
		}
	})
}

func TestInteractiveSelect_DownWraps(t *testing.T) {
	withMockTerminal([]byte{
		0x1b, '[', 'B',
		0x1b, '[', 'B',
		0x1b, '[', 'B',
		'\r',
	}, func() {
		idx, err := interactiveSelect("pick", []string{"a", "b", "c"}, nil)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if idx != 0 {
			t.Errorf("got idx=%d, want 0", idx)
		}
	})
}

func TestInteractiveSelect_CtrlC(t *testing.T) {
	withMockTerminal([]byte{0x03}, func() {
		_, err := interactiveSelect("pick", []string{"a", "b"}, nil)
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestInteractiveSelect_Escape(t *testing.T) {
	withMockTerminal([]byte{0x1b}, func() {
		_, err := interactiveSelect("pick", []string{"a", "b"}, nil)
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestInteractiveSelect_MultiSelect_SpaceToggle(t *testing.T) {
	withMockTerminal([]byte{
		' ',
		0x1b, '[', 'B',
		' ',
		'\r',
	}, func() {
		sel := make([]bool, 3)
		_, err := interactiveSelect("pick", []string{"a", "b", "c"}, sel)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if !sel[0] || !sel[1] || sel[2] {
			t.Errorf("selected = %v, want [true, true, false]", sel)
		}
	})
}

func TestInteractiveSelect_MakeRawError(t *testing.T) {
	oldMakeRaw := makeRawFn
	defer func() { makeRawFn = oldMakeRaw }()

	makeRawFn = func() (*term.State, error) {
		return nil, errors.New("not a terminal")
	}

	_, err := interactiveSelect("pick", []string{"a", "b"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInteractiveSelect_ReadError(t *testing.T) {
	withMockTerminal([]byte{}, func() {
		_, err := interactiveSelect("pick", []string{"a"}, nil)
		if err == nil {
			t.Fatal("expected error on EOF")
		}
	})
}

func TestInteractiveSelect_SpaceInSingleSelect(t *testing.T) {
	withMockTerminal([]byte{' ', '\r'}, func() {
		idx, err := interactiveSelect("pick", []string{"a", "b"}, nil)
		if err != nil {
			t.Fatalf("interactiveSelect: %v", err)
		}
		if idx != 0 {
			t.Errorf("got idx=%d, want 0", idx)
		}
	})
}

func TestFallbackSelect_Valid(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	oldStdin := os.Stdin
	oldOut := PromptOut
	defer func() {
		os.Stdin = oldStdin
		PromptOut = oldOut
	}()

	os.Stdin = r
	var out bytes.Buffer
	PromptOut = &out

	if _, err := w.Write([]byte("2\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	idx, err := fallbackSelect("pick one", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("fallbackSelect: %v", err)
	}
	if idx != 1 {
		t.Fatalf("idx=%d, want 1", idx)
	}
}

func TestFallbackSelect_Invalid(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer r.Close()

	oldStdin := os.Stdin
	oldOut := PromptOut
	defer func() {
		os.Stdin = oldStdin
		PromptOut = oldOut
	}()

	os.Stdin = r
	PromptOut = &bytes.Buffer{}

	if _, err := w.Write([]byte("99\n")); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	_ = w.Close()

	_, err = fallbackSelect("pick one", []string{"a", "b", "c"})
	if err == nil {
		t.Fatal("expected invalid selection error")
	}
}

func TestMakeRawFn_DefaultBody(t *testing.T) {
	// Call the default makeRawFn to cover its function body.
	// It will fail because stdin is not a terminal in tests.
	_, err := makeRawFn()
	if err == nil {
		// If it somehow succeeds (in a TTY context), restore immediately
		t.Log("makeRawFn succeeded unexpectedly in test")
	}
}

func TestRestoreFn_DefaultBody(t *testing.T) {
	// Call the default restoreFn to cover its function body.
	// Pass a valid (empty) state to avoid nil dereference.
	state := &term.State{}
	_ = restoreFn(state)
}

func TestInteractiveSelect_StdinReadError(t *testing.T) {
	origMakeRaw := makeRawFn
	origRestore := restoreFn
	origReader := stdinReader
	defer func() {
		makeRawFn = origMakeRaw
		restoreFn = origRestore
		stdinReader = origReader
	}()

	makeRawFn = func() (*term.State, error) { return nil, nil }
	restoreFn = func(s *term.State) error { return nil }
	stdinReader = &errReaderUI{}

	oldOut := PromptOut
	PromptOut = &bytes.Buffer{}
	defer func() { PromptOut = oldOut }()

	_, err := interactiveSelect("test", []string{"a", "b"}, nil)
	if err == nil {
		t.Fatal("expected read error")
	}
}

type errReaderUI struct{}

func (errReaderUI) Read([]byte) (int, error) { return 0, errors.New("read fail") }
