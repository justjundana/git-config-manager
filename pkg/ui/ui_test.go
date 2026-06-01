package ui

import (
	"bytes"
	"strings"
	"testing"
)

func captureOutput(fn func()) string {
	var buf bytes.Buffer
	oldOut, oldErr := Out, Err
	Out = &buf
	Err = &buf
	defer func() {
		Out = oldOut
		Err = oldErr
	}()
	fn()
	return buf.String()
}

func TestSuccess(t *testing.T) {
	out := captureOutput(func() { Success("done %s", "now") })
	if !strings.Contains(out, "done now") {
		t.Errorf("Success output = %q", out)
	}
}

func TestError(t *testing.T) {
	out := captureOutput(func() { Error("bad %s", "thing") })
	if !strings.Contains(out, "bad thing") {
		t.Errorf("Error output = %q", out)
	}
}

func TestWarning(t *testing.T) {
	out := captureOutput(func() { Warning("warn %d", 42) })
	if !strings.Contains(out, "warn 42") {
		t.Errorf("Warning output = %q", out)
	}
}

func TestInfo(t *testing.T) {
	out := captureOutput(func() { Info("info %s", "msg") })
	if !strings.Contains(out, "info msg") {
		t.Errorf("Info output = %q", out)
	}
}

func TestPrint(t *testing.T) {
	out := captureOutput(func() { Print("hello %s", "world") })
	if !strings.Contains(out, "hello world") {
		t.Errorf("Print output = %q", out)
	}
}

func TestHeader(t *testing.T) {
	out := captureOutput(func() { Header("Title %s", "here") })
	if !strings.Contains(out, "Title here") {
		t.Errorf("Header output = %q", out)
	}
}

func TestSubHeader(t *testing.T) {
	out := captureOutput(func() { SubHeader("Sub %s", "title") })
	if !strings.Contains(out, "Sub title") {
		t.Errorf("SubHeader output = %q", out)
	}
	if !strings.Contains(out, "─") {
		t.Error("SubHeader should contain separator")
	}
}

func TestDetail(t *testing.T) {
	out := captureOutput(func() { Detail("Key", "Value") })
	if !strings.Contains(out, "Value") {
		t.Errorf("Detail output = %q", out)
	}
}

func TestBlank(t *testing.T) {
	out := captureOutput(func() { Blank() })
	if out == "" {
		t.Error("Blank should produce output")
	}
}

func TestDivider(t *testing.T) {
	out := captureOutput(func() { Divider() })
	if !strings.Contains(out, "─") {
		t.Error("Divider should contain line")
	}
}

func TestNextSteps(t *testing.T) {
	out := captureOutput(func() { NextSteps([]string{"step 1", "step 2"}) })
	if !strings.Contains(out, "step 1") || !strings.Contains(out, "step 2") {
		t.Errorf("NextSteps output = %q", out)
	}
	if !strings.Contains(out, "1.") || !strings.Contains(out, "2.") {
		t.Error("NextSteps should number steps")
	}
}

func TestDisableColor(t *testing.T) {
	// Just ensure it doesn't panic
	DisableColor()
}

func TestSimpleTable(t *testing.T) {
	out := captureOutput(func() {
		SimpleTable([]string{"Name", "Age"}, [][]string{{"Alice", "30"}, {"Bob", "25"}})
	})
	if !strings.Contains(out, "Alice") || !strings.Contains(out, "Bob") {
		t.Errorf("SimpleTable output = %q", out)
	}
}

func TestSimpleTable_Empty(t *testing.T) {
	out := captureOutput(func() {
		SimpleTable([]string{}, nil)
	})
	if out != "" {
		t.Errorf("expected no output for empty headers, got %q", out)
	}
}

func TestNextSteps_Empty(t *testing.T) {
	out := captureOutput(func() { NextSteps(nil) })
	if !strings.Contains(out, "Next steps") {
		t.Errorf("NextSteps should still print header: %q", out)
	}
}

func TestDetail_KeyValue(t *testing.T) {
	out := captureOutput(func() { Detail("Name", "Alice") })
	if !strings.Contains(out, "Alice") {
		t.Errorf("Detail output = %q", out)
	}
}

func TestBlank_ProducesNewline(t *testing.T) {
	out := captureOutput(func() { Blank() })
	if out != "\n" {
		t.Errorf("Blank output = %q, want newline", out)
	}
}
