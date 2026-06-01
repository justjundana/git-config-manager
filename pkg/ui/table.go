package ui

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// ansiRegex matches ANSI escape sequences used for terminal colors/styles.
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// visibleLen returns the visible display width of a string, excluding ANSI escape codes.
// Uses rune count to properly handle multi-byte UTF-8 characters (●, ✓, ✗, etc.).
func visibleLen(s string) int {
	stripped := ansiRegex.ReplaceAllString(s, "")
	return utf8.RuneCountInString(stripped)
}

// stripANSI removes ANSI escape codes from a string.
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// truncateVisible truncates a string to maxLen visible characters, adding "…" if truncated.
func truncateVisible(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	plain := stripANSI(s)
	if utf8.RuneCountInString(plain) <= maxLen {
		return s
	}
	// For colored strings, we need to walk rune-by-rune tracking visible vs ANSI
	if plain == s {
		// No ANSI codes, safe to truncate directly by runes
		if maxLen <= 1 {
			return "…"
		}
		runes := []rune(s)
		return string(runes[:maxLen-1]) + "…"
	}
	// Has ANSI codes - truncate visible content rune by rune
	visible := 0
	result := strings.Builder{}
	i := 0
	for i < len(s) && visible < maxLen-1 {
		if s[i] == '\x1b' {
			// Copy entire escape sequence
			j := i
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				j++ // include 'm'
			}
			result.WriteString(s[i:j])
			i = j
		} else {
			r, size := utf8.DecodeRuneInString(s[i:])
			result.WriteRune(r)
			visible++
			i += size
		}
	}
	result.WriteString("…")
	result.WriteString("\x1b[0m") // reset color after truncation
	return result.String()
}

// terminalWidthFn is a test hook for overriding terminal width detection.
var terminalWidthFn = terminalWidthReal

// Test hooks for terminal size detection.
var (
	stdoutIsTerminalFn = func(fd int) bool { return term.IsTerminal(fd) }
	getTermSizeFn      = func(fd int) (int, int, error) { return term.GetSize(fd) }
)

// terminalWidthReal returns the current terminal width.
// Returns a large value when stdout is not a terminal (pipes, tests) to disable responsive behavior.
func terminalWidthReal() int {
	fd := int(os.Stdout.Fd())
	if !stdoutIsTerminalFn(fd) {
		return 999 // non-terminal: don't truncate/hide
	}
	w, _, err := getTermSizeFn(fd)
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// tableWidth calculates the total width of a table given column widths and padding.
func tableWidth(widths []int, padding int) int {
	total := 0
	for _, w := range widths {
		total += w + padding
	}
	return total
}

// ColumnPriority defines which columns are essential vs optional.
// Lower number = higher priority (will be kept when terminal is narrow).
type ColumnPriority struct {
	Index    int
	Priority int // 1 = must keep, 2 = important, 3 = optional
	MinWidth int // minimum width before truncation kicks in
}

// PrintTable renders a formatted table to the given writer.
// It auto-adapts to terminal width: truncates long cells, hides optional columns,
// or switches to vertical card layout when the terminal is very narrow.
func PrintTable(w io.Writer, headers []string, rows [][]string) {
	PrintTableWithPriority(w, headers, rows, nil)
}

// PrintTableWithPriority renders a table with column priority hints for responsive layout.
// If priorities is nil, all columns get default priorities (first col = 1, rest = 2, last two = 3).
func PrintTableWithPriority(w io.Writer, headers []string, rows [][]string, priorities []ColumnPriority) {
	if len(headers) == 0 {
		return
	}

	const colPadding = 3
	termW := terminalWidthFn()

	// Assign default priorities if not provided
	if priorities == nil {
		priorities = make([]ColumnPriority, len(headers))
		for i := range headers {
			priorities[i] = ColumnPriority{Index: i, Priority: 2, MinWidth: 4}
			if i == 0 {
				priorities[i].Priority = 1
			} else if i >= len(headers)-2 {
				priorities[i].Priority = 3
			}
		}
	}

	// Calculate natural column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && visibleLen(cell) > widths[i] {
				widths[i] = visibleLen(cell)
			}
		}
	}

	// Determine which columns to show
	visibleCols := make([]bool, len(headers))
	for i := range visibleCols {
		visibleCols[i] = true
	}

	totalW := tableWidth(widths, colPadding)

	// Strategy 1: Truncate wide columns to fit
	if totalW > termW {
		for i := range widths {
			maxForCol := widths[i]
			if maxForCol > 20 && totalW > termW {
				shrink := maxForCol - 20
				excess := totalW - termW
				if shrink > excess {
					shrink = excess
				}
				widths[i] -= shrink
				totalW -= shrink
			}
		}
	}

	// Strategy 2: Hide low-priority columns
	if totalW > termW {
		// Remove priority 3 columns first, then 2
		for prio := 3; prio >= 2 && totalW > termW; prio-- {
			for i := len(headers) - 1; i >= 0 && totalW > termW; i-- {
				if priorities[i].Priority >= prio && visibleCols[i] {
					totalW -= widths[i] + colPadding
					visibleCols[i] = false
				}
			}
		}
	}

	// Strategy 3: If still too wide (extreme case), switch to vertical card layout
	if totalW > termW || termW < 40 {
		printVerticalCards(w, headers, rows)
		return
	}

	// Print header
	for i, h := range headers {
		if !visibleCols[i] {
			continue
		}
		colored := Gray(strings.ToUpper(h))
		padding := widths[i] + colPadding - visibleLen(colored)
		if padding < 0 {
			padding = 0
		}
		fmt.Fprintf(w, "%s%s", colored, strings.Repeat(" ", padding))
	}
	fmt.Fprintln(w)

	// Print rows
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) || !visibleCols[i] {
				continue
			}
			// Truncate cell if wider than allowed width
			if visibleLen(cell) > widths[i] {
				cell = truncateVisible(cell, widths[i])
			}
			padding := widths[i] + colPadding - visibleLen(cell)
			fmt.Fprintf(w, "%s%s", cell, strings.Repeat(" ", padding))
		}
		fmt.Fprintln(w)
	}
}

// printVerticalCards renders rows as vertical key-value cards (for very narrow terminals).
func printVerticalCards(w io.Writer, headers []string, rows [][]string) {
	for i, row := range rows {
		if i > 0 {
			fmt.Fprintln(w)
		}
		for j, cell := range row {
			if j < len(headers) && stripANSI(cell) != "" && stripANSI(cell) != "-" {
				fmt.Fprintf(w, "  %s: %s\n", Gray(headers[j]), cell)
			}
		}
	}
}
