package profile

import (
	"fmt"
	"testing"
)

func TestActivationScope_String(t *testing.T) {
	tests := []struct {
		scope ActivationScope
		want  string
	}{
		{ScopeSession, "session"},
		{ScopeGlobal, "global"},
		{ScopeLocal, "local"},
		{ActivationScope(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.scope.String(); got != tt.want {
			t.Errorf("ActivationScope(%d).String() = %q, want %q", tt.scope, got, tt.want)
		}
	}
}

func TestProfileError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *ProfileError
		want string
	}{
		{
			"basic",
			&ProfileError{Code: 1001, Message: "not found"},
			"[1001] not found",
		},
		{
			"with profile",
			&ProfileError{Code: 1001, Message: "not found", Profile: "work"},
			"[1001] not found (profile: work)",
		},
		{
			"with cause",
			&ProfileError{Code: 1003, Message: "invalid", Cause: fmt.Errorf("bad data")},
			"[1003] invalid: bad data",
		},
		{
			"with profile and cause",
			&ProfileError{Code: 1001, Message: "not found", Profile: "work", Cause: fmt.Errorf("io error")},
			"[1001] not found (profile: work): io error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProfileError_Unwrap(t *testing.T) {
	cause := fmt.Errorf("root cause")
	e := &ProfileError{Code: 1001, Message: "test", Cause: cause}
	if e.Unwrap() != cause {
		t.Error("Unwrap should return the Cause")
	}

	e2 := &ProfileError{Code: 1001, Message: "test"}
	if e2.Unwrap() != nil {
		t.Error("Unwrap should return nil when no Cause")
	}
}

func TestErrNotFound(t *testing.T) {
	e := errNotFound("myprofile")
	if e.Code != ErrCodeNotFound {
		t.Errorf("code = %d", e.Code)
	}
	if e.Profile != "myprofile" {
		t.Errorf("profile = %q", e.Profile)
	}
}

func TestErrAlreadyExists(t *testing.T) {
	e := errAlreadyExists("dup")
	if e.Code != ErrCodeAlreadyExists {
		t.Errorf("code = %d", e.Code)
	}
}

func TestErrCannotDeleteActive(t *testing.T) {
	e := errCannotDeleteActive("active")
	if e.Code != ErrCodeActive {
		t.Errorf("code = %d", e.Code)
	}
}

func TestErrCannotDeleteDefault(t *testing.T) {
	e := errCannotDeleteDefault("default")
	if e.Code != ErrCodeDefault {
		t.Errorf("code = %d", e.Code)
	}
}

func TestSentinelErrors(t *testing.T) {
	if ErrProfileNameEmpty().Code != ErrCodeNameEmpty {
		t.Error("ErrProfileNameEmpty code mismatch")
	}
	if ErrGitUserNameEmpty().Code != ErrCodeUserNameEmpty {
		t.Error("ErrGitUserNameEmpty code mismatch")
	}
	if ErrGitUserEmailEmpty().Code != ErrCodeEmailEmpty {
		t.Error("ErrGitUserEmailEmpty code mismatch")
	}
	if ErrGitUserEmailInvalid().Code != ErrCodeEmailInvalid {
		t.Error("ErrGitUserEmailInvalid code mismatch")
	}
}

func TestBoolPtrValues(t *testing.T) {
	truePtr := BoolPtr(true)
	if truePtr == nil || !*truePtr {
		t.Error("BoolPtr(true) should return pointer to true")
	}
	falsePtr := BoolPtr(false)
	if falsePtr == nil || *falsePtr {
		t.Error("BoolPtr(false) should return pointer to false")
	}
}
