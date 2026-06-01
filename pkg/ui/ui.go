// Package ui provides CLI user interface components for GCM.
package ui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
)

// Icons used throughout the CLI.
const (
	IconSuccess  = "✓"
	IconError    = "✗"
	IconWarning  = "⚠"
	IconInfo     = "→"
	IconArrow    = "▸"
	IconKey      = "🔑"
	IconGlobe    = "🌐"
	IconProfile  = "🎯"
	IconCheck    = "✅"
	IconDoctor   = "🏥"
	IconRocket   = "🚀"
	IconTemplate = "📋"
	IconShell    = "🐚"
)

var (
	// Color functions
	Green  = color.New(color.FgGreen).SprintFunc()
	Red    = color.New(color.FgRed).SprintFunc()
	Yellow = color.New(color.FgYellow).SprintFunc()
	Blue   = color.New(color.FgBlue).SprintFunc()
	Cyan   = color.New(color.FgCyan).SprintFunc()
	Gray   = color.New(color.FgHiBlack).SprintFunc()
	Bold   = color.New(color.Bold).SprintFunc()
	Dim    = color.New(color.Faint).SprintFunc()

	// Output writer
	Out io.Writer = os.Stdout
	Err io.Writer = os.Stderr
)

// DisableColor disables all color output.
func DisableColor() {
	color.NoColor = true
}

// Success prints a success message.
func Success(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(Out, "%s %s\n", Green(IconSuccess), msg)
}

// Error prints an error message.
func Error(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(Err, "%s %s\n", Red(IconError), msg)
}

// Warning prints a warning message.
func Warning(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(Out, "%s %s\n", Yellow(IconWarning), msg)
}

// Info prints an informational message.
func Info(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(Out, "%s %s\n", Blue(IconInfo), msg)
}

// Print prints a plain message.
func Print(format string, a ...interface{}) {
	fmt.Fprintf(Out, format+"\n", a...)
}

// Header prints a bold header line.
func Header(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(Out, "\n%s\n", Bold(msg))
}

// SubHeader prints a sub-header with a separator.
func SubHeader(format string, a ...interface{}) {
	msg := fmt.Sprintf(format, a...)
	fmt.Fprintf(Out, "\n%s\n%s\n", msg, strings.Repeat("─", len(msg)))
}

// Detail prints a key-value detail line.
func Detail(key, value string) {
	fmt.Fprintf(Out, "  %s: %s\n", Gray(key), value)
}

// Blank prints an empty line.
func Blank() {
	fmt.Fprintln(Out)
}

// Divider prints a horizontal divider.
func Divider() {
	fmt.Fprintln(Out, Gray(strings.Repeat("─", 50)))
}

// NextSteps prints a list of suggested next steps.
func NextSteps(steps []string) {
	fmt.Fprintln(Out)
	fmt.Fprintln(Out, Bold("Next steps:"))
	for i, step := range steps {
		fmt.Fprintf(Out, "  %d. %s\n", i+1, step)
	}
}

// Table renders a simple table. See table.go for the full Table type.
func SimpleTable(headers []string, rows [][]string) {
	PrintTable(Out, headers, rows)
}
