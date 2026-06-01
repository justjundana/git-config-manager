package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Test hooks for terminal raw mode operations.
var (
	makeRawFn             = func() (*term.State, error) { return term.MakeRaw(int(os.Stdin.Fd())) }
	restoreFn             = func(s *term.State) error { return term.Restore(int(os.Stdin.Fd()), s) }
	stdinReader io.Reader = os.Stdin
)

// interactiveSelect renders a single- or multi-select menu and returns the
// index of the highlighted option on Enter. When selected != nil the menu is
// multi-select: space toggles the entry at the cursor and Enter submits the
// whole selection (selected slice is mutated in place).
//
// If the terminal does not support raw mode (e.g. dumb terminals, older Windows,
// CI/CD, piped input), falls back to a numbered list prompt.
func interactiveSelect(msg string, options []string, selected []bool) (int, error) {
	oldState, err := makeRawFn()
	if err != nil {
		// Fallback: numbered list for terminals that don't support raw mode
		return fallbackSelect(msg, options)
	}
	defer restoreFn(oldState)

	cursor := 0
	multi := selected != nil

	drawMenu(msg, options, cursor, selected, false)

	buf := make([]byte, 3)
	for {
		n, err := stdinReader.Read(buf)
		if err != nil {
			clearMenu(len(options) + 2)
			return 0, err
		}

		key := decodeKey(buf[:n])
		switch key {
		case keyUp:
			if cursor > 0 {
				cursor--
			} else {
				cursor = len(options) - 1
			}
		case keyDown:
			if cursor < len(options)-1 {
				cursor++
			} else {
				cursor = 0
			}
		case keySpace:
			if multi {
				selected[cursor] = !selected[cursor]
			}
		case keyEnter:
			clearMenu(len(options) + 2)
			drawMenu(msg, options, cursor, selected, true)
			return cursor, nil
		case keyCtrlC, keyEsc:
			clearMenu(len(options) + 2)
			return 0, ErrInterrupted
		}

		clearMenu(len(options) + 2)
		drawMenu(msg, options, cursor, selected, false)
	}
}

type key int

const (
	keyUnknown key = iota
	keyUp
	keyDown
	keyEnter
	keySpace
	keyEsc
	keyCtrlC
)

func decodeKey(b []byte) key {
	if len(b) == 0 {
		return keyUnknown
	}
	switch {
	case len(b) == 1 && b[0] == 0x03:
		return keyCtrlC
	case len(b) == 1 && (b[0] == '\r' || b[0] == '\n'):
		return keyEnter
	case len(b) == 1 && b[0] == ' ':
		return keySpace
	case len(b) == 1 && b[0] == 0x1b:
		return keyEsc
	case len(b) == 3 && b[0] == 0x1b && b[1] == '[':
		switch b[2] {
		case 'A':
			return keyUp
		case 'B':
			return keyDown
		}
	}
	return keyUnknown
}

func drawMenu(msg string, options []string, cursor int, selected []bool, final bool) {
	fmt.Fprintf(PromptOut, "%s %s\r\n", Cyan("?"), msg)

	if final {
		if selected != nil {
			var picked []string
			for i, ok := range selected {
				if ok {
					picked = append(picked, options[i])
				}
			}
			if len(picked) == 0 {
				fmt.Fprintf(PromptOut, "  %s\r\n", Dim("(none)"))
			} else {
				for _, p := range picked {
					fmt.Fprintf(PromptOut, "  %s %s\r\n", Green("✓"), p)
				}
			}
		} else {
			fmt.Fprintf(PromptOut, "  %s %s\r\n", Green("✓"), options[cursor])
		}
		fmt.Fprint(PromptOut, "\r\n")
		return
	}

	for i, opt := range options {
		pointer := "  "
		if i == cursor {
			pointer = Cyan("❯ ")
		}
		if selected != nil {
			box := "[ ]"
			if selected[i] {
				box = Green("[x]")
			}
			fmt.Fprintf(PromptOut, "%s%s %s\r\n", pointer, box, opt)
		} else {
			line := opt
			if i == cursor {
				line = Cyan(opt)
			}
			fmt.Fprintf(PromptOut, "%s%s\r\n", pointer, line)
		}
	}
	fmt.Fprint(PromptOut, Dim("  (use ↑/↓ to move, enter to confirm"))
	if selected != nil {
		fmt.Fprint(PromptOut, Dim(", space to toggle"))
	}
	fmt.Fprint(PromptOut, Dim(")\r\n"))
}

// clearMenu moves the cursor up n lines and clears each one. Uses raw ANSI
// since we're in raw mode during interactiveSelect.
func clearMenu(n int) {
	// Move to start of current line, then up n-1 lines, clearing each.
	fmt.Fprint(PromptOut, "\r")
	for i := 0; i < n; i++ {
		fmt.Fprint(PromptOut, "\x1b[1A\x1b[2K")
	}
}

// fallbackSelect provides a simple numbered-list selection for terminals
// that don't support raw mode (dumb terminals, CI/CD, older Windows, pipes).
func fallbackSelect(msg string, options []string) (int, error) {
	fmt.Fprintf(PromptOut, "%s %s\n", Cyan("?"), msg)
	for i, opt := range options {
		fmt.Fprintf(PromptOut, "  %d) %s\n", i+1, opt)
	}
	fmt.Fprintf(PromptOut, "Enter number [1-%d]: ", len(options))

	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return 0, err
	}
	line = strings.TrimSpace(line)
	num, err := strconv.Atoi(line)
	if err != nil || num < 1 || num > len(options) {
		return 0, fmt.Errorf("invalid selection %q", line)
	}
	return num - 1, nil
}
