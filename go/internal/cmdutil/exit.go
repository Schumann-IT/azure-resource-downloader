package cmdutil

import "errors"

// ExitCodeError carries the process exit code a command wants alongside the
// error it returns from RunE. Commands return it (via WithExitCode) instead of
// calling os.Exit inline, so deferred cleanup runs and the command stays
// testable end-to-end; the root Execute unwraps it with errors.As to pick the
// process exit code, keeping exactly one os.Exit site in the program.
type ExitCodeError struct {
	Code int
	Err  error
}

// Error returns the wrapped error's message; the exit code is process plumbing,
// not part of what the user reads.
func (e *ExitCodeError) Error() string { return e.Err.Error() }

// Unwrap exposes the wrapped error so errors.Is/errors.As see through the code.
func (e *ExitCodeError) Unwrap() error { return e.Err }

// WithExitCode wraps err so that Execute exits with the given code instead of
// the default 1. A nil err returns nil, so it can wrap a call site directly.
func WithExitCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitCodeError{Code: code, Err: err}
}

// ExitCode returns the process exit code for err: 0 for nil, the carried code
// when err wraps an ExitCodeError, and 1 for any other error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitCodeError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}
