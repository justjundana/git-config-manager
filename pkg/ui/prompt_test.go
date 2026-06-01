package ui

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// withStdio swaps PromptIn/PromptOut for the duration of fn and restores them.
func withStdio(in string, fn func(out *bytes.Buffer)) {
	oldIn, oldOut := PromptIn, PromptOut
	var out bytes.Buffer
	PromptIn = strings.NewReader(in)
	PromptOut = &out
	defer func() {
		PromptIn = oldIn
		PromptOut = oldOut
	}()
	fn(&out)
}

func TestAskString_Default(t *testing.T) {
	withStdio("\n", func(_ *bytes.Buffer) {
		got, err := AskString("Name", "Alice")
		if err != nil {
			t.Fatalf("AskString returned error: %v", err)
		}
		if got != "Alice" {
			t.Errorf("got %q, want default %q", got, "Alice")
		}
	})
}

func TestAskString_UserInput(t *testing.T) {
	withStdio("Bob\n", func(_ *bytes.Buffer) {
		got, err := AskString("Name", "Alice")
		if err != nil {
			t.Fatalf("AskString returned error: %v", err)
		}
		if got != "Bob" {
			t.Errorf("got %q, want %q", got, "Bob")
		}
	})
}

func TestAskString_EOF(t *testing.T) {
	// Empty input triggers EOF before any bytes are read.
	withStdio("", func(_ *bytes.Buffer) {
		_, err := AskString("Name", "")
		if err != ErrInterrupted {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestAskConfirm_DefaultFalse(t *testing.T) {
	withStdio("\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("Proceed?", false)
		if err != nil {
			t.Fatalf("AskConfirm returned error: %v", err)
		}
		if ok {
			t.Error("expected false when default is false and user hits Enter")
		}
	})
}

func TestAskConfirm_DefaultTrue(t *testing.T) {
	withStdio("\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("Proceed?", true)
		if err != nil {
			t.Fatalf("AskConfirm returned error: %v", err)
		}
		if !ok {
			t.Error("expected true when default is true and user hits Enter")
		}
	})
}

func TestAskConfirm_Yes(t *testing.T) {
	withStdio("y\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("Proceed?", false)
		if err != nil {
			t.Fatalf("AskConfirm: %v", err)
		}
		if !ok {
			t.Error("expected true for 'y'")
		}
	})
}

func TestAskConfirm_No(t *testing.T) {
	withStdio("n\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("Proceed?", true)
		if err != nil {
			t.Fatalf("AskConfirm: %v", err)
		}
		if ok {
			t.Error("expected false for 'n'")
		}
	})
}

func TestAskConfirm_InvalidThenYes(t *testing.T) {
	withStdio("maybe\ny\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("Proceed?", false)
		if err != nil {
			t.Fatalf("AskConfirm: %v", err)
		}
		if !ok {
			t.Error("expected true after retrying")
		}
	})
}

func TestAskConfirm_EOF(t *testing.T) {
	withStdio("", func(_ *bytes.Buffer) {
		_, err := AskConfirm("Proceed?", false)
		if err != ErrInterrupted {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestFormatLabel(t *testing.T) {
	got := formatLabel("Name", "default")
	if !strings.Contains(got, "Name") || !strings.Contains(got, "default") {
		t.Errorf("formatLabel = %q", got)
	}

	got2 := formatLabel("Name", "")
	if !strings.Contains(got2, "Name") {
		t.Errorf("formatLabel no default = %q", got2)
	}
}

func TestSelectFallback(t *testing.T) {
	withStdio("2\n", func(_ *bytes.Buffer) {
		got, err := selectFallback("Pick:", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("selectFallback: %v", err)
		}
		if got != "b" {
			t.Errorf("got %q, want %q", got, "b")
		}
	})
}

func TestSelectFallback_Default(t *testing.T) {
	withStdio("\n", func(_ *bytes.Buffer) {
		got, err := selectFallback("Pick:", []string{"x", "y"})
		if err != nil {
			t.Fatalf("selectFallback: %v", err)
		}
		if got != "x" {
			t.Errorf("got %q, want default %q", got, "x")
		}
	})
}

func TestSelectFallback_Invalid(t *testing.T) {
	withStdio("9\n", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a"})
		if err == nil {
			t.Error("expected error for out of range")
		}
	})
}

func TestAskSelect_EmptyOptionsList(t *testing.T) {
	_, err := AskSelect("Pick:", []string{})
	if err == nil {
		t.Error("expected error for empty options slice")
	}
}

func TestAskMultiSelect_EmptyOptionsList(t *testing.T) {
	_, err := AskMultiSelect("Pick:", []string{})
	if err == nil {
		t.Error("expected error for empty options slice")
	}
}

func TestSelectFallback_FirstOption(t *testing.T) {
	withStdio("1\n", func(_ *bytes.Buffer) {
		got, err := selectFallback("Pick:", []string{"first", "second"})
		if err != nil {
			t.Fatalf("selectFallback: %v", err)
		}
		if got != "first" {
			t.Errorf("got %q, want %q", got, "first")
		}
	})
}

func TestSelectFallback_LastOption(t *testing.T) {
	withStdio("3\n", func(_ *bytes.Buffer) {
		got, err := selectFallback("Pick:", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("selectFallback: %v", err)
		}
		if got != "c" {
			t.Errorf("got %q, want %q", got, "c")
		}
	})
}

func TestMultiSelectFallback_AllOptions(t *testing.T) {
	withStdio("1,2,3\n", func(_ *bytes.Buffer) {
		got, err := multiSelectFallback("Pick:", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("multiSelectFallback: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("expected 3, got %d", len(got))
		}
	})
}

func TestSelectFallback_ZeroIndex(t *testing.T) {
	withStdio("0\n", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Error("expected error for index 0")
		}
	})
}

func TestMultiSelectFallback_ZeroIndex(t *testing.T) {
	withStdio("0\n", func(_ *bytes.Buffer) {
		_, err := multiSelectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Error("expected error for index 0")
		}
	})
}

func TestMultiSelectFallback(t *testing.T) {
	withStdio("1,3\n", func(_ *bytes.Buffer) {
		got, err := multiSelectFallback("Pick:", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("multiSelectFallback: %v", err)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "c" {
			t.Errorf("got %v", got)
		}
	})
}

func TestMultiSelectFallback_Empty(t *testing.T) {
	withStdio("\n", func(_ *bytes.Buffer) {
		got, err := multiSelectFallback("Pick:", []string{"a", "b"})
		if err != nil {
			t.Fatalf("multiSelectFallback: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})
}

func TestMultiSelectFallback_Invalid(t *testing.T) {
	withStdio("5\n", func(_ *bytes.Buffer) {
		_, err := multiSelectFallback("Pick:", []string{"a"})
		if err == nil {
			t.Error("expected error for out of range")
		}
	})
}

func TestAskSelect_NoOptions(t *testing.T) {
	_, err := AskSelect("Pick:", nil)
	if err == nil {
		t.Error("expected error for empty options")
	}
}

func TestAskMultiSelect_NoOptions(t *testing.T) {
	_, err := AskMultiSelect("Pick:", nil)
	if err == nil {
		t.Error("expected error for empty options")
	}
}

func TestAskPassword_NonTerminal(t *testing.T) {
	// In test context, stdin is not a terminal, so it falls back to reader
	withStdio("secret\n", func(_ *bytes.Buffer) {
		got, err := AskPassword("Password")
		if err != nil {
			t.Fatalf("AskPassword: %v", err)
		}
		if got != "secret" {
			t.Errorf("got %q, want %q", got, "secret")
		}
	})
}

func TestDecodeKey(t *testing.T) {
	tests := []struct {
		input []byte
		want  key
	}{
		{[]byte{0x03}, keyCtrlC},
		{[]byte{'\r'}, keyEnter},
		{[]byte{'\n'}, keyEnter},
		{[]byte{' '}, keySpace},
		{[]byte{0x1b}, keyEsc},
		{[]byte{0x1b, '[', 'A'}, keyUp},
		{[]byte{0x1b, '[', 'B'}, keyDown},
		{[]byte{'a'}, keyUnknown},
		{[]byte{}, keyUnknown},
	}
	for _, tt := range tests {
		got := decodeKey(tt.input)
		if got != tt.want {
			t.Errorf("decodeKey(%v) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

// Ensure io import is referenced in case of future trimming.
var _ io.Reader = (*bytes.Buffer)(nil)

func TestAskSelect_FallbackPath(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("2\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskSelect("Pick one:", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("AskSelect: %v", err)
	}
	if got != "b" {
		t.Errorf("AskSelect = %q, want %q", got, "b")
	}
}

func TestAskMultiSelect_FallbackPath(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("1,3\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskMultiSelect("Pick:", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("AskMultiSelect: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("AskMultiSelect = %v, want [a c]", got)
	}
}

func TestAskMultiSelect_EmptyFallback(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskMultiSelect("Pick:", []string{"a", "b"})
	if err != nil {
		t.Fatalf("AskMultiSelect: %v", err)
	}
	if got != nil {
		t.Errorf("AskMultiSelect = %v, want nil", got)
	}
}

func TestAskPassword_NonTerminal_WithInput(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("secret123\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskPassword("Enter password")
	if err != nil {
		t.Fatalf("AskPassword: %v", err)
	}
	if got != "secret123" {
		t.Errorf("AskPassword = %q, want %q", got, "secret123")
	}
}

func TestDrawMenu_SingleSelect(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	drawMenu("Pick:", []string{"a", "b", "c"}, 1, nil, false)
	s := buf.String()
	if !strings.Contains(s, "Pick:") {
		t.Errorf("missing prompt: %q", s)
	}
}

func TestDrawMenu_MultiSelect(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	sel := []bool{true, false, true}
	drawMenu("Pick:", []string{"a", "b", "c"}, 0, sel, false)
	s := buf.String()
	if !strings.Contains(s, "space to toggle") {
		t.Errorf("missing toggle hint: %q", s)
	}
}

func TestDrawMenu_Final_Single(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	drawMenu("Pick:", []string{"a", "b"}, 1, nil, true)
	s := buf.String()
	if !strings.Contains(s, "b") {
		t.Errorf("final should show selected: %q", s)
	}
}

func TestDrawMenu_Final_MultiNone(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	sel := []bool{false, false}
	drawMenu("Pick:", []string{"a", "b"}, 0, sel, true)
	s := buf.String()
	if !strings.Contains(s, "(none)") {
		t.Errorf("expected (none): %q", s)
	}
}

func TestDrawMenu_Final_MultiSome(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	sel := []bool{true, false, true}
	drawMenu("Pick:", []string{"a", "b", "c"}, 0, sel, true)
	s := buf.String()
	if !strings.Contains(s, "a") || !strings.Contains(s, "c") {
		t.Errorf("expected a,c: %q", s)
	}
}

func TestClearMenu(t *testing.T) {
	var buf bytes.Buffer
	oldOut := PromptOut
	PromptOut = &buf
	defer func() { PromptOut = oldOut }()

	clearMenu(3)
	s := buf.String()
	if !strings.Contains(s, "\x1b[1A") {
		t.Errorf("expected ANSI up: %q", s)
	}
}

func TestDecodeKey_UnknownEscapeSeq(t *testing.T) {
	got := decodeKey([]byte{0x1b, '[', 'C'})
	if got != keyUnknown {
		t.Errorf("expected keyUnknown for right arrow, got %d", got)
	}
}

func TestSelectFallback_NonNumeric(t *testing.T) {
	withStdio("abc\n", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Error("expected error for non-numeric input")
		}
	})
}

func TestMultiSelectFallback_NonNumeric(t *testing.T) {
	withStdio("abc\n", func(_ *bytes.Buffer) {
		_, err := multiSelectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Error("expected error for non-numeric input")
		}
	})
}

func TestAskString_EOFWithPartialInput(t *testing.T) {
	withStdio("partial", func(_ *bytes.Buffer) {
		got, err := AskString("Name", "")
		if err != nil {
			t.Fatalf("AskString: %v", err)
		}
		if got != "partial" {
			t.Errorf("got %q, want %q", got, "partial")
		}
	})
}

func TestAskConfirm_YesFull(t *testing.T) {
	withStdio("yes\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("ok?", false)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !ok {
			t.Error("expected true")
		}
	})
}

func TestAskConfirm_NoFull(t *testing.T) {
	withStdio("no\n", func(_ *bytes.Buffer) {
		ok, err := AskConfirm("ok?", true)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ok {
			t.Error("expected false")
		}
	})
}

func TestAskString_NoDefault(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("hello\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskString("Enter:", "")
	if err != nil {
		t.Fatalf("AskString: %v", err)
	}
	if got != "hello" {
		t.Errorf("AskString = %q, want %q", got, "hello")
	}
}

func TestAskConfirm_YesVariant(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("yes\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskConfirm("Sure?", false)
	if err != nil {
		t.Fatalf("AskConfirm: %v", err)
	}
	if !got {
		t.Error("expected true for 'yes'")
	}
}

func TestAskConfirm_NoVariant(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("no\n")
	PromptOut = &bytes.Buffer{}

	got, err := AskConfirm("Sure?", true)
	if err != nil {
		t.Fatalf("AskConfirm: %v", err)
	}
	if got {
		t.Error("expected false for 'no'")
	}
}

func TestAskPassword_EOF(t *testing.T) {
	withStdio("", func(_ *bytes.Buffer) {
		got, err := AskPassword("secret")
		if err != nil && err != io.EOF {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})
}

func TestAskString_EOFPartialInput(t *testing.T) {
	// Input without newline triggers EOF path with partial data
	withStdio("partial", func(_ *bytes.Buffer) {
		got, err := AskString("name", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "partial" {
			t.Errorf("expected 'partial', got %q", got)
		}
	})
}

func TestAskConfirm_EOFPartialYes(t *testing.T) {
	// "y" without newline triggers EOF path
	withStdio("y", func(_ *bytes.Buffer) {
		got, err := AskConfirm("ok?", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected true for 'y'")
		}
	})
}

func TestAskConfirm_DefaultTrueEmpty(t *testing.T) {
	withStdio("\n", func(_ *bytes.Buffer) {
		got, err := AskConfirm("ok?", true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Error("expected true for default=true with empty input")
		}
	})
}

func TestSelectFallback_Choice2(t *testing.T) {
	withStdio("2\n", func(_ *bytes.Buffer) {
		got, err := selectFallback("pick", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("selectFallback: %v", err)
		}
		if got != "b" {
			t.Errorf("expected 'b', got %q", got)
		}
	})
}

func TestMultiSelectFallback_Multiple(t *testing.T) {
	withStdio("1,3\n", func(_ *bytes.Buffer) {
		got, err := multiSelectFallback("pick", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("multiSelectFallback: %v", err)
		}
		if len(got) != 2 || got[0] != "a" || got[1] != "c" {
			t.Errorf("expected [a, c], got %v", got)
		}
	})
}

func TestMultiSelectFallback_All(t *testing.T) {
	withStdio("1,2\n", func(_ *bytes.Buffer) {
		got, err := multiSelectFallback("pick", []string{"x", "y"})
		if err != nil {
			t.Fatalf("multiSelectFallback: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected all 2 options, got %d", len(got))
		}
	})
}

// errWriter is a writer that always returns an error.
type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("write error")
}

// errReader is a reader that always returns a non-EOF error.
type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read error")
}

func TestAskString_WriteError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("test\n")
	PromptOut = errWriter{}

	_, err := AskString("Name", "")
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "write error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAskString_ReadError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = errReader{}
	PromptOut = &bytes.Buffer{}

	_, err := AskString("Name", "")
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAskPassword_WriteError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("secret\n")
	PromptOut = errWriter{}

	_, err := AskPassword("Password")
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "write error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAskPassword_ReadError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = errReader{}
	PromptOut = &bytes.Buffer{}

	_, err := AskPassword("Password")
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAskConfirm_WriteError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = strings.NewReader("y\n")
	PromptOut = errWriter{}

	_, err := AskConfirm("Proceed?", false)
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "write error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAskConfirm_ReadError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = errReader{}
	PromptOut = &bytes.Buffer{}

	_, err := AskConfirm("Proceed?", false)
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "read error") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSelectFallback_EOF(t *testing.T) {
	withStdio("", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error on EOF")
		}
	})
}

func TestMultiSelectFallback_EOF(t *testing.T) {
	withStdio("", func(_ *bytes.Buffer) {
		_, err := multiSelectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error on EOF")
		}
	})
}

func TestAskPassword_EOFWithPartialInput(t *testing.T) {
	// EOF with partial input (no newline) - should return the partial input
	withStdio("partial", func(_ *bytes.Buffer) {
		got, err := AskPassword("secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "partial" {
			t.Errorf("got %q, want %q", got, "partial")
		}
	})
}

func TestAskConfirm_InvalidThenEOF(t *testing.T) {
	// First line is invalid, second is EOF
	withStdio("maybe\n", func(_ *bytes.Buffer) {
		_, err := AskConfirm("ok?", false)
		if err != ErrInterrupted {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestAskString_WindowsLineEnding(t *testing.T) {
	withStdio("hello\r\n", func(_ *bytes.Buffer) {
		got, err := AskString("Name", "")
		if err != nil {
			t.Fatalf("AskString: %v", err)
		}
		if got != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})
}

func TestAskConfirm_CaseInsensitive(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"Y\n", true},
		{"N\n", false},
		{"YES\n", true},
		{"NO\n", false},
	}
	for _, tc := range cases {
		withStdio(tc.input, func(_ *bytes.Buffer) {
			got, err := AskConfirm("ok?", false)
			if err != nil {
				t.Fatalf("AskConfirm(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("AskConfirm(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestSelectFallback_ReadError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = errReader{}
	PromptOut = &bytes.Buffer{}

	_, err := selectFallback("Pick:", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMultiSelectFallback_ReadError(t *testing.T) {
	oldIn, oldOut := PromptIn, PromptOut
	defer func() { PromptIn, PromptOut = oldIn, oldOut }()

	PromptIn = errReader{}
	PromptOut = &bytes.Buffer{}

	_, err := multiSelectFallback("Pick:", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// =============================================================================
// Tests for terminal-detected branches using test hooks
// =============================================================================

func withTerminal(fn func()) {
	oldIsTerm := isTerminalFn
	oldReadPw := readPasswordFn
	defer func() {
		isTerminalFn = oldIsTerm
		readPasswordFn = oldReadPw
	}()
	isTerminalFn = func() bool { return true }
	fn()
}

func TestAskPassword_TerminalPath(t *testing.T) {
	withTerminal(func() {
		readPasswordFn = func() ([]byte, error) {
			return []byte("secret123"), nil
		}
		oldOut := PromptOut
		defer func() { PromptOut = oldOut }()
		PromptOut = &bytes.Buffer{}

		got, err := AskPassword("Enter password")
		if err != nil {
			t.Fatalf("AskPassword: %v", err)
		}
		if got != "secret123" {
			t.Errorf("got %q, want %q", got, "secret123")
		}
	})
}

func TestAskPassword_TerminalReadError(t *testing.T) {
	withTerminal(func() {
		readPasswordFn = func() ([]byte, error) {
			return nil, errors.New("terminal error")
		}
		oldOut := PromptOut
		defer func() { PromptOut = oldOut }()
		PromptOut = &bytes.Buffer{}

		_, err := AskPassword("Enter password")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "terminal error") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestAskSelect_EmptyOptions(t *testing.T) {
	_, err := AskSelect("pick", []string{})
	if err == nil {
		t.Fatal("expected error for empty options")
	}
}

func TestAskMultiSelect_EmptyOptions(t *testing.T) {
	_, err := AskMultiSelect("pick", []string{})
	if err == nil {
		t.Fatal("expected error for empty options")
	}
}

func TestSelectFallback_OutOfRangeHigh(t *testing.T) {
	withStdio("99\n", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error for out-of-range selection")
		}
	})
}

func TestSelectFallback_OutOfRangeLow(t *testing.T) {
	withStdio("0\n", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error for out-of-range selection")
		}
	})
}

func TestSelectFallback_NonNumericInput(t *testing.T) {
	withStdio("abc\n", func(_ *bytes.Buffer) {
		_, err := selectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error for non-numeric input")
		}
	})
}

func TestMultiSelectFallback_OutOfRange(t *testing.T) {
	withStdio("1,99\n", func(_ *bytes.Buffer) {
		_, err := multiSelectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error for out-of-range selection")
		}
	})
}

func TestMultiSelectFallback_NonNumericInput(t *testing.T) {
	withStdio("abc\n", func(_ *bytes.Buffer) {
		_, err := multiSelectFallback("Pick:", []string{"a", "b"})
		if err == nil {
			t.Fatal("expected error for non-numeric input")
		}
	})
}

func TestPrintTable_NegativePadding(t *testing.T) {
	// Test with very long content that exceeds column width to trigger padding < 0
	buf := &bytes.Buffer{}
	headers := []string{"H"}
	rows := [][]string{{"short"}, {"this is a really really long cell value that exceeds everything"}}
	PrintTable(buf, headers, rows)
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestAskSelect_TerminalPath(t *testing.T) {
	oldIsTerm := isTerminalFn
	oldSelect := interactiveSelectFn
	defer func() { isTerminalFn = oldIsTerm; interactiveSelectFn = oldSelect }()

	isTerminalFn = func() bool { return true }
	interactiveSelectFn = func(msg string, options []string, selected []bool) (int, error) {
		return 1, nil
	}

	got, err := AskSelect("pick", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("AskSelect: %v", err)
	}
	if got != "b" {
		t.Errorf("got %q, want %q", got, "b")
	}
}

func TestAskSelect_TerminalError(t *testing.T) {
	oldIsTerm := isTerminalFn
	oldSelect := interactiveSelectFn
	defer func() { isTerminalFn = oldIsTerm; interactiveSelectFn = oldSelect }()

	isTerminalFn = func() bool { return true }
	interactiveSelectFn = func(msg string, options []string, selected []bool) (int, error) {
		return 0, errors.New("cancelled")
	}

	_, err := AskSelect("pick", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAskMultiSelect_TerminalPath(t *testing.T) {
	oldIsTerm := isTerminalFn
	oldSelect := interactiveSelectFn
	defer func() { isTerminalFn = oldIsTerm; interactiveSelectFn = oldSelect }()

	isTerminalFn = func() bool { return true }
	interactiveSelectFn = func(msg string, options []string, selected []bool) (int, error) {
		selected[0] = true
		selected[2] = true
		return 0, nil
	}

	got, err := AskMultiSelect("pick", []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("AskMultiSelect: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "c" {
		t.Errorf("got %v, want [a, c]", got)
	}
}

func TestAskMultiSelect_TerminalError(t *testing.T) {
	oldIsTerm := isTerminalFn
	oldSelect := interactiveSelectFn
	defer func() { isTerminalFn = oldIsTerm; interactiveSelectFn = oldSelect }()

	isTerminalFn = func() bool { return true }
	interactiveSelectFn = func(msg string, options []string, selected []bool) (int, error) {
		return 0, errors.New("cancelled")
	}

	_, err := AskMultiSelect("pick", []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAskMultiSelect_TerminalNoneSelected(t *testing.T) {
	oldIsTerm := isTerminalFn
	oldSelect := interactiveSelectFn
	defer func() { isTerminalFn = oldIsTerm; interactiveSelectFn = oldSelect }()

	isTerminalFn = func() bool { return true }
	interactiveSelectFn = func(msg string, options []string, selected []bool) (int, error) {
		return 0, nil
	}

	got, err := AskMultiSelect("pick", []string{"a", "b"})
	if err != nil {
		t.Fatalf("AskMultiSelect: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestReadPasswordFn_DefaultBody(t *testing.T) {
	// Call the default readPasswordFn to cover its function body.
	// It will fail because stdin is not a terminal in tests.
	_, err := readPasswordFn()
	if err == nil {
		t.Log("readPasswordFn succeeded unexpectedly in test")
	}
}

// withSignalInterrupt overrides signalNotifyFn to immediately send a signal,
// simulating Ctrl+C during a prompt. It also sets PromptIn to a reader that
// blocks (never returns), forcing the select to pick the signal branch.
func withSignalInterrupt(fn func()) {
	oldNotify := signalNotifyFn
	oldStop := signalStopFn
	oldIn := PromptIn
	oldOut := PromptOut
	defer func() {
		signalNotifyFn = oldNotify
		signalStopFn = oldStop
		PromptIn = oldIn
		PromptOut = oldOut
	}()

	// Use a reader that blocks forever (pipe with no writer).
	pr, _ := io.Pipe()
	PromptIn = pr
	PromptOut = &bytes.Buffer{}

	signalNotifyFn = func(ch chan<- os.Signal) {
		// Immediately fire the interrupt signal.
		go func() { ch <- os.Interrupt }()
	}
	signalStopFn = func(ch chan<- os.Signal) {}

	fn()
}

func TestReadLineInterruptible_Signal(t *testing.T) {
	withSignalInterrupt(func() {
		reader := bufio.NewReader(PromptIn)
		line, err := readLineInterruptible(reader)
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
		if line != "" {
			t.Errorf("expected empty line, got %q", line)
		}
	})
}

func TestAskString_Signal(t *testing.T) {
	withSignalInterrupt(func() {
		_, err := AskString("Name", "default")
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestAskConfirm_Signal(t *testing.T) {
	withSignalInterrupt(func() {
		_, err := AskConfirm("Continue?", true)
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}

func TestAskPassword_Signal(t *testing.T) {
	withSignalInterrupt(func() {
		_, err := AskPassword("Secret")
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got %v", err)
		}
	})
}
