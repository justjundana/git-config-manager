package profile

import "fmt"

// Error codes for profile operations.
const (
	ErrCodeNotFound      = 1001
	ErrCodeAlreadyExists = 1002
	ErrCodeInvalid       = 1003
	ErrCodeActive        = 1004
	ErrCodeDefault       = 1005
	ErrCodeNameEmpty     = 1010
	ErrCodeEmailEmpty    = 1011
	ErrCodeEmailInvalid  = 1012
	ErrCodeUserNameEmpty = 1013
	ErrCodeSSHKeyPath    = 2001
	ErrCodeGPGKeyID      = 3001
)

// ProfileError is a structured error type for profile operations.
type ProfileError struct {
	Code       int
	Message    string
	Profile    string
	Cause      error
	Suggestion string
}

func (e *ProfileError) Error() string {
	msg := fmt.Sprintf("[%d] %s", e.Code, e.Message)
	if e.Profile != "" {
		msg += fmt.Sprintf(" (profile: %s)", e.Profile)
	}
	if e.Cause != nil {
		msg += fmt.Sprintf(": %v", e.Cause)
	}
	return msg
}

func (e *ProfileError) Unwrap() error {
	return e.Cause
}

// Common errors — return fresh instances to avoid mutation of shared state.

func ErrProfileNameEmpty() *ProfileError {
	return &ProfileError{
		Code:       ErrCodeNameEmpty,
		Message:    "profile name cannot be empty",
		Suggestion: "Provide a profile name, e.g.: gcm profile create work",
	}
}

func ErrGitUserNameEmpty() *ProfileError {
	return &ProfileError{
		Code:       ErrCodeUserNameEmpty,
		Message:    "git user name cannot be empty",
		Suggestion: "Use --name flag or interactive mode: gcm profile create work -i",
	}
}

func ErrGitUserEmailEmpty() *ProfileError {
	return &ProfileError{
		Code:       ErrCodeEmailEmpty,
		Message:    "git user email cannot be empty",
		Suggestion: "Use --email flag or interactive mode: gcm profile create work -i",
	}
}

func ErrGitUserEmailInvalid() *ProfileError {
	return &ProfileError{
		Code:       ErrCodeEmailInvalid,
		Message:    "git user email format is invalid",
		Suggestion: "Provide a valid email address, e.g.: john@company.com",
	}
}

func errNotFound(name string) *ProfileError {
	return &ProfileError{
		Code:       ErrCodeNotFound,
		Message:    fmt.Sprintf("profile %q not found", name),
		Profile:    name,
		Suggestion: fmt.Sprintf("Create it with: gcm profile create %s", name),
	}
}

func errAlreadyExists(name string) *ProfileError {
	return &ProfileError{
		Code:       ErrCodeAlreadyExists,
		Message:    fmt.Sprintf("profile %q already exists", name),
		Profile:    name,
		Suggestion: fmt.Sprintf("Edit it with: gcm profile edit %s", name),
	}
}

func errCannotDeleteActive(name string) *ProfileError {
	return &ProfileError{
		Code:       ErrCodeActive,
		Message:    fmt.Sprintf("cannot delete active profile %q", name),
		Profile:    name,
		Suggestion: "Switch to a different profile first: gcm use <other-profile>",
	}
}

func errCannotDeleteDefault(name string) *ProfileError {
	return &ProfileError{
		Code:       ErrCodeDefault,
		Message:    fmt.Sprintf("cannot delete default profile %q", name),
		Profile:    name,
		Suggestion: "Change the default profile first: gcm use <other-profile> --global",
	}
}
