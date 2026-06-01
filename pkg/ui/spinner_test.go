package ui

import (
	"bytes"
	"strings"
	"testing"
)

func TestSpinner(t *testing.T) {
	var buf bytes.Buffer
	oldOut := Out
	Out = &buf
	defer func() { Out = oldOut }()

	sp := NewSpinner("loading...")
	if sp == nil {
		t.Fatal("expected non-nil spinner")
	}

	sp.Start()
	sp.UpdateMessage("still loading...")
	sp.Stop("done")

	out := buf.String()
	if !strings.Contains(out, "done") {
		t.Errorf("Stop output = %q, want 'done'", out)
	}
}

func TestSpinner_StopError(t *testing.T) {
	var buf bytes.Buffer
	oldOut, oldErr := Out, Err
	Out = &buf
	Err = &buf
	defer func() { Out = oldOut; Err = oldErr }()

	sp := NewSpinner("loading...")
	sp.Start()
	sp.StopError("failed")

	out := buf.String()
	if !strings.Contains(out, "failed") {
		t.Errorf("StopError output = %q, want 'failed'", out)
	}
}

func TestSpinner_StopEmptyMsg(t *testing.T) {
	var buf bytes.Buffer
	oldOut := Out
	Out = &buf
	defer func() { Out = oldOut }()

	sp := NewSpinner("loading...")
	sp.Start()
	sp.Stop("")
	// No message should be printed
}

func TestSpinner_StopErrorEmptyMsg(t *testing.T) {
	var buf bytes.Buffer
	oldOut, oldErr := Out, Err
	Out = &buf
	Err = &buf
	defer func() { Out = oldOut; Err = oldErr }()

	sp := NewSpinner("loading...")
	sp.Start()
	sp.StopError("")
}
