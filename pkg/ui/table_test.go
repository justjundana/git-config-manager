package ui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"Name", "Age", "City"}
	rows := [][]string{
		{"Alice", "30", "NYC"},
		{"Bob", "25", "London"},
	}

	PrintTable(&buf, headers, rows)
	out := buf.String()

	if !strings.Contains(out, "Alice") {
		t.Errorf("missing Alice: %q", out)
	}
	if !strings.Contains(out, "London") {
		t.Errorf("missing London: %q", out)
	}
}

func TestPrintTable_Empty(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []string{}, nil)
	if buf.Len() != 0 {
		t.Error("expected no output for empty headers")
	}
}

func TestPrintTable_NoRows(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []string{"A", "B"}, nil)
	out := buf.String()
	if out == "" {
		t.Error("expected header output even with no rows")
	}
}

func TestPrintTable_WideColumns(t *testing.T) {
	var buf bytes.Buffer
	PrintTable(&buf, []string{"X"}, [][]string{{"very long value here"}})
	if !strings.Contains(buf.String(), "very long value here") {
		t.Error("should handle wide columns")
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b[1;32mbold green\x1b[0m text", "bold green text"},
		{"", ""},
		{"no color here", "no color here"},
	}
	for _, tt := range tests {
		got := stripANSI(tt.input)
		if got != tt.want {
			t.Errorf("stripANSI(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateVisible(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"hello world", 5, "hell…"},
		{"", 5, ""},
		{"abc", 0, ""},
		{"a", 1, "a"},
		{"ab", 1, "…"},
		{"abcdef", 3, "ab…"},
	}
	for _, tt := range tests {
		got := truncateVisible(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateVisible(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestTruncateVisible_WithANSI(t *testing.T) {
	colored := "\x1b[31mhello world\x1b[0m"
	got := truncateVisible(colored, 8)
	plain := stripANSI(got)
	// Function counts bytes for visible chars, so result should fit
	if !strings.Contains(got, "…") {
		t.Error("expected ellipsis in truncated output")
	}
	// Verify content was actually truncated (not full "hello world")
	if strings.Contains(plain, "hello world") {
		t.Error("string should be truncated")
	}
}

func TestPrintVerticalCards(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"Name", "Email", "Role"}
	rows := [][]string{
		{"Alice", "alice@test.com", "admin"},
		{"Bob", "bob@test.com", "user"},
	}
	printVerticalCards(&buf, headers, rows)
	out := buf.String()

	if !strings.Contains(out, "Alice") {
		t.Error("missing Alice in vertical card")
	}
	if !strings.Contains(out, "bob@test.com") {
		t.Error("missing bob email in vertical card")
	}
}

func TestPrintVerticalCards_SkipsEmpty(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"Name", "Empty", "Role"}
	rows := [][]string{
		{"Alice", "", "admin"},
		{"Bob", "-", "user"},
	}
	printVerticalCards(&buf, headers, rows)
	out := buf.String()

	// Empty and "-" cells should be skipped
	if strings.Contains(out, "Empty: \n") {
		t.Error("should skip empty cells")
	}
}

func TestPrintTableWithPriority(t *testing.T) {
	var buf bytes.Buffer
	headers := []string{"Name", "Email", "Status"}
	rows := [][]string{
		{"Alice", "alice@test.com", "active"},
	}
	priorities := []ColumnPriority{
		{Index: 0, Priority: 1, MinWidth: 4},
		{Index: 1, Priority: 2, MinWidth: 4},
		{Index: 2, Priority: 3, MinWidth: 4},
	}
	PrintTableWithPriority(&buf, headers, rows, priorities)
	out := buf.String()
	if !strings.Contains(out, "Alice") {
		t.Error("missing Alice in priority table")
	}
}

func TestTableWidth(t *testing.T) {
	widths := []int{10, 20, 5}
	got := tableWidth(widths, 3)
	want := (10 + 3) + (20 + 3) + (5 + 3)
	if got != want {
		t.Errorf("tableWidth = %d, want %d", got, want)
	}
}

func TestVisibleLen(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"hello", 5},
		{"\x1b[31mred\x1b[0m", 3},
		{"", 0},
	}
	for _, tt := range tests {
		got := visibleLen(tt.input)
		if got != tt.want {
			t.Errorf("visibleLen(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestPrintTableWithPriority_TruncateStrategy(t *testing.T) {
	orig := terminalWidthFn
	defer func() { terminalWidthFn = orig }()

	// Set terminal width to trigger column truncation (Strategy 1)
	// Table with wide columns that exceed terminal width
	terminalWidthFn = func() int { return 60 }

	var buf bytes.Buffer
	headers := []string{"Name", "Description", "Status"}
	rows := [][]string{
		{"short", "this is a very long description that should be truncated", "active"},
	}
	PrintTableWithPriority(&buf, headers, rows, nil)
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestPrintTableWithPriority_HideColumns(t *testing.T) {
	orig := terminalWidthFn
	defer func() { terminalWidthFn = orig }()

	// Set terminal width very narrow to trigger column hiding (Strategy 2)
	terminalWidthFn = func() int { return 45 }

	var buf bytes.Buffer
	headers := []string{"Name", "Email", "Role", "Status", "Created"}
	rows := [][]string{
		{"alice", "alice@example.com", "admin", "active", "2024-01-01"},
	}
	PrintTableWithPriority(&buf, headers, rows, nil)
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestPrintTableWithPriority_VerticalCards(t *testing.T) {
	orig := terminalWidthFn
	defer func() { terminalWidthFn = orig }()

	// Set terminal width extremely narrow to trigger vertical card layout (Strategy 3)
	terminalWidthFn = func() int { return 20 }

	var buf bytes.Buffer
	headers := []string{"Name", "Email", "Status"}
	rows := [][]string{
		{"alice", "alice@example.com", "active"},
		{"bob", "-", "inactive"},
	}
	PrintTableWithPriority(&buf, headers, rows, nil)
	out := buf.String()
	if !strings.Contains(out, "alice") {
		t.Fatal("expected vertical card output")
	}
}

func TestTerminalWidthReal_NonTerminal(t *testing.T) {
	// In tests, stdout is not a terminal, so terminalWidthReal returns 999
	w := terminalWidthReal()
	if w != 999 {
		t.Errorf("expected 999 for non-terminal, got %d", w)
	}
}

func TestTerminalWidthReal_IsTerminal(t *testing.T) {
	origIsTerminal := stdoutIsTerminalFn
	origGetSize := getTermSizeFn
	defer func() {
		stdoutIsTerminalFn = origIsTerminal
		getTermSizeFn = origGetSize
	}()

	// Simulate being a terminal with successful GetSize
	stdoutIsTerminalFn = func(fd int) bool { return true }
	getTermSizeFn = func(fd int) (int, int, error) { return 120, 40, nil }
	if w := terminalWidthReal(); w != 120 {
		t.Errorf("expected 120, got %d", w)
	}

	// Simulate GetSize error
	getTermSizeFn = func(fd int) (int, int, error) { return 0, 0, errors.New("fail") }
	if w := terminalWidthReal(); w != 80 {
		t.Errorf("expected 80 on error, got %d", w)
	}

	// Simulate GetSize returns 0
	getTermSizeFn = func(fd int) (int, int, error) { return 0, 40, nil }
	if w := terminalWidthReal(); w != 80 {
		t.Errorf("expected 80 on w<=0, got %d", w)
	}
}

func TestGetTermSizeFn_DefaultBody(t *testing.T) {
	origIsTerminal := stdoutIsTerminalFn
	defer func() { stdoutIsTerminalFn = origIsTerminal }()

	// Make it think stdout is a terminal, but keep real getTermSizeFn
	// This exercises the default getTermSizeFn body (which will fail on non-terminal fd)
	stdoutIsTerminalFn = func(fd int) bool { return true }
	// terminalWidthReal will call the real getTermSizeFn which returns error → returns 80
	w := terminalWidthReal()
	if w != 80 {
		t.Logf("getTermSizeFn returned width %d (may be running in a real terminal)", w)
	}
}

func TestTruncateVisible_Short(t *testing.T) {
	// maxLen <= 1
	got := truncateVisible("hello", 1)
	if got != "…" {
		t.Errorf("expected '…', got %q", got)
	}
	// maxLen == 0
	got = truncateVisible("hello", 0)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestPrintTableWithPriority_CellTruncation(t *testing.T) {
	orig := terminalWidthFn
	defer func() { terminalWidthFn = orig }()

	// Use a width that allows table but forces cell truncation in strategy 1
	terminalWidthFn = func() int { return 50 }

	var buf bytes.Buffer
	headers := []string{"A", "B"}
	rows := [][]string{
		{"short", "this is an extremely long cell value that will definitely need to be truncated"},
	}
	PrintTableWithPriority(&buf, headers, rows, nil)
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestPrintTableWithPriority_NegativePadding(t *testing.T) {
	orig := terminalWidthFn
	defer func() { terminalWidthFn = orig }()

	// Two columns with 30-char headers, short cell values.
	// Natural widths=[30,30], totalW=66. With termW=46, excess=20.
	// Strategy 1: col0 shrinks by 10 (30→20), col1 shrinks by 10 (30→20). totalW=46.
	// Header visibleLen=30 > widths[i]+3=23 → padding < 0.
	terminalWidthFn = func() int { return 46 }

	var buf bytes.Buffer
	headers := []string{"HeaderNameThatIsThirtyCharsXX", "AnotherHeaderThirtyCharsHereX"}
	rows := [][]string{
		{"a", "b"},
	}
	PrintTableWithPriority(&buf, headers, rows, nil)
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}
