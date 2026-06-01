package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"golang.org/x/term"
)

// ErrInterrupted is returned when the user cancels a prompt (Ctrl-C / EOF).
var ErrInterrupted = errors.New("prompt interrupted")

// PromptIn and PromptOut allow tests to inject readers / writers.
var (
	PromptIn  io.Reader = os.Stdin
	PromptOut io.Writer = os.Stderr // prompts go to stderr so they don't pollute piped stdout
)

// isTerminalFn is a test hook for overriding terminal detection.
var isTerminalFn = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// readPasswordFn is a test hook for overriding password reading.
var readPasswordFn = func() ([]byte, error) {
	return term.ReadPassword(int(os.Stdin.Fd()))
}

// interactiveSelectFn is a test hook for overriding interactive selection.
var interactiveSelectFn = interactiveSelect

// signalNotifyFn is a test hook for overriding signal notification setup.
var signalNotifyFn = func(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}

// signalStopFn is a test hook for overriding signal stop.
var signalStopFn = signal.Stop

// lineResult holds the return values from a ReadString call.
type lineResult struct {
	line string
	err  error
}

// readLineInterruptible reads a line from the given reader while listening for
// OS interrupt signals. If SIGINT arrives before input is available, it returns
// ErrInterrupted immediately.
func readLineInterruptible(reader *bufio.Reader) (string, error) {
	ch := make(chan lineResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- lineResult{line, err}
	}()

	sigCh := make(chan os.Signal, 1)
	signalNotifyFn(sigCh)
	defer signalStopFn(sigCh)

	select {
	case r := <-ch:
		return r.line, r.err
	case <-sigCh:
		fmt.Fprintln(PromptOut)
		return "", ErrInterrupted
	}
}

// AskString prompts the user for a string input. If the user presses Enter
// without typing anything, defaultVal is returned.
func AskString(msg, defaultVal string) (string, error) {
	if _, err := fmt.Fprint(PromptOut, formatLabel(msg, defaultVal)); err != nil {
		return "", err
	}

	reader := bufio.NewReader(PromptIn)
	line, err := readLineInterruptible(reader)
	if err != nil {
		if errors.Is(err, io.EOF) && line == "" {
			return "", ErrInterrupted
		}
		if errors.Is(err, ErrInterrupted) {
			return "", ErrInterrupted
		}
		if !errors.Is(err, io.EOF) {
			return "", err
		}
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return defaultVal, nil
	}
	return line, nil
}

// AskPassword prompts the user for a secret input. Echo is suppressed when
// stdin is a terminal.
func AskPassword(msg string) (string, error) {
	if _, err := fmt.Fprintf(PromptOut, "%s %s: ", Cyan("?"), msg); err != nil {
		return "", err
	}

	if !isTerminalFn() {
		reader := bufio.NewReader(PromptIn)
		line, err := readLineInterruptible(reader)
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				return "", ErrInterrupted
			}
			if !errors.Is(err, io.EOF) {
				return "", err
			}
		}
		return strings.TrimRight(line, "\r\n"), nil
	}

	b, err := readPasswordFn()
	fmt.Fprintln(PromptOut)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// AskConfirm prompts the user for a yes/no confirmation. An empty answer
// returns defaultVal.
func AskConfirm(msg string, defaultVal bool) (bool, error) {
	suffix := "[y/N]"
	if defaultVal {
		suffix = "[Y/n]"
	}
	reader := bufio.NewReader(PromptIn)
	for {
		if _, err := fmt.Fprintf(PromptOut, "%s %s %s: ", Cyan("?"), msg, Dim(suffix)); err != nil {
			return false, err
		}
		line, err := readLineInterruptible(reader)
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return false, ErrInterrupted
			}
			if errors.Is(err, ErrInterrupted) {
				return false, ErrInterrupted
			}
			if !errors.Is(err, io.EOF) {
				return false, err
			}
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return defaultVal, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(PromptOut, Dim("  please answer y or n"))
	}
}

// AskSelect presents a single-choice menu.
func AskSelect(msg string, options []string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("no options to select from")
	}

	if !isTerminalFn() {
		return selectFallback(msg, options)
	}

	idx, err := interactiveSelectFn(msg, options, nil)
	if err != nil {
		return "", err
	}
	return options[idx], nil
}

// AskMultiSelect presents a multi-select menu.
func AskMultiSelect(msg string, options []string) ([]string, error) {
	if len(options) == 0 {
		return nil, errors.New("no options to select from")
	}

	if !isTerminalFn() {
		return multiSelectFallback(msg, options)
	}

	selected := make([]bool, len(options))
	if _, err := interactiveSelectFn(msg, options, selected); err != nil {
		return nil, err
	}

	var picked []string
	for i, ok := range selected {
		if ok {
			picked = append(picked, options[i])
		}
	}
	return picked, nil
}

func formatLabel(msg, defaultVal string) string {
	if defaultVal != "" {
		return fmt.Sprintf("%s %s %s ", Cyan("?"), msg, Dim("("+defaultVal+")"))
	}
	return fmt.Sprintf("%s %s ", Cyan("?"), msg)
}

func selectFallback(msg string, options []string) (string, error) {
	fmt.Fprintln(PromptOut, Cyan("?"), msg)
	for i, opt := range options {
		fmt.Fprintf(PromptOut, "  %d) %s\n", i+1, opt)
	}
	raw, err := AskString("Enter number:", "1")
	if err != nil {
		return "", err
	}
	var idx int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &idx); err != nil || idx < 1 || idx > len(options) {
		return "", fmt.Errorf("invalid choice: %s", raw)
	}
	return options[idx-1], nil
}

func multiSelectFallback(msg string, options []string) ([]string, error) {
	fmt.Fprintln(PromptOut, Cyan("?"), msg)
	for i, opt := range options {
		fmt.Fprintf(PromptOut, "  %d) %s\n", i+1, opt)
	}
	raw, err := AskString("Enter comma-separated numbers (empty for none):", "")
	if err != nil {
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var picked []string
	for _, p := range strings.Split(raw, ",") {
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx); err != nil || idx < 1 || idx > len(options) {
			return nil, fmt.Errorf("invalid choice: %s", p)
		}
		picked = append(picked, options[idx-1])
	}
	return picked, nil
}
