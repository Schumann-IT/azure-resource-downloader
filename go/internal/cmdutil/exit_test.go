package cmdutil

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "nil error is success", err: nil, want: 0},
		{name: "plain error defaults to 1", err: errors.New("boom"), want: 1},
		{name: "carried code wins", err: WithExitCode(3, errors.New("stale")), want: 3},
		{
			name: "carried code survives wrapping",
			err:  fmt.Errorf("outer: %w", WithExitCode(2, errors.New("cannot answer"))),
			want: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWithExitCodeNilIsNil(t *testing.T) {
	if err := WithExitCode(2, nil); err != nil {
		t.Errorf("WithExitCode(2, nil) = %v, want nil", err)
	}
}

func TestExitCodeErrorTransparency(t *testing.T) {
	sentinel := errors.New("sentinel")
	err := WithExitCode(3, fmt.Errorf("context: %w", sentinel))

	if !errors.Is(err, sentinel) {
		t.Error("errors.Is must see through ExitCodeError to the wrapped sentinel")
	}
	if got, want := err.Error(), "context: sentinel"; got != want {
		t.Errorf("Error() = %q, want %q (the exit code must not leak into the message)", got, want)
	}
}
