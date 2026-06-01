package ui

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

// Spinner wraps the spinner library for consistent UI.
type Spinner struct {
	s *spinner.Spinner
}

// NewSpinner creates a new spinner with the given message.
func NewSpinner(msg string) *Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " " + msg
	s.Writer = Out
	return &Spinner{s: s}
}

// Start begins the spinner animation.
func (sp *Spinner) Start() {
	sp.s.Start()
}

// Stop stops the spinner with a success message.
func (sp *Spinner) Stop(msg string) {
	sp.s.Stop()
	if msg != "" {
		fmt.Fprintf(Out, "%s %s\n", Green(IconSuccess), msg)
	}
}

// StopError stops the spinner with an error message.
func (sp *Spinner) StopError(msg string) {
	sp.s.Stop()
	if msg != "" {
		fmt.Fprintf(Err, "%s %s\n", Red(IconError), msg)
	}
}

// UpdateMessage updates the spinner suffix.
func (sp *Spinner) UpdateMessage(msg string) {
	sp.s.Suffix = " " + msg
}
