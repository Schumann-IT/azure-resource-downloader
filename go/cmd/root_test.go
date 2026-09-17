package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/cmdutil"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// captureRoot points the root command's output streams at buffers for one
// execute() run and restores the default streams afterwards.
func captureRoot(t *testing.T) (outBuf, errBuf *bytes.Buffer) {
	t.Helper()
	outBuf, errBuf = &bytes.Buffer{}, &bytes.Buffer{}
	rootCmd.SetOut(outBuf)
	rootCmd.SetErr(errBuf)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		viper.Reset()
	})
	return outBuf, errBuf
}

// TestErrorsPrintedExactlyOnce guards the single print site: Cobra's own error
// printing is silenced on root, so a returned error must appear exactly once
// (previously Cobra printed it and Execute printed it again). A flag error is
// used because it exercises the earliest error path; usage must still be shown
// for it, since the mistake is in the invocation.
func TestErrorsPrintedExactlyOnce(t *testing.T) {
	outBuf, errBuf := captureRoot(t)

	rootCmd.SetArgs([]string{"resource", "list", "--bogus"})
	if code := execute(); code != 1 {
		t.Errorf("execute() = %d, want 1", code)
	}

	combined := outBuf.String() + errBuf.String()
	if got := strings.Count(combined, "unknown flag: --bogus"); got != 1 {
		t.Errorf("error printed %d times, want exactly once; output:\n%s", got, combined)
	}
	if !strings.Contains(combined, "Usage:") {
		t.Errorf("a flag error must still print usage; output:\n%s", combined)
	}
}

// TestExecuteCarriesExitCode verifies the exit-code plumbing end-to-end: a
// command returning an exit-code-carrying error must surface that code from
// execute(), with the message printed once and no usage (a runtime failure is
// not an invocation mistake).
func TestExecuteCarriesExitCode(t *testing.T) {
	probe := &cobra.Command{
		Use: "exit-code-probe",
		RunE: func(*cobra.Command, []string) error {
			return cmdutil.WithExitCode(3, errors.New("probe: carried exit code"))
		},
	}
	rootCmd.AddCommand(probe)
	t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

	outBuf, errBuf := captureRoot(t)

	rootCmd.SetArgs([]string{"exit-code-probe"})
	if code := execute(); code != 3 {
		t.Errorf("execute() = %d, want the carried exit code 3", code)
	}

	combined := outBuf.String() + errBuf.String()
	if got := strings.Count(combined, "probe: carried exit code"); got != 1 {
		t.Errorf("error printed %d times, want exactly once; output:\n%s", got, combined)
	}
	if strings.Contains(combined, "Usage:") {
		t.Errorf("a runtime error must not print usage; output:\n%s", combined)
	}
}

// TestExecuteCancelsOnInterrupt verifies the graceful-shutdown wiring: an
// interrupt must cancel the context every command receives via cmd.Context(),
// so listing and fetching stop cleanly instead of the process dying mid-write.
// The probe command sends SIGINT to the test process itself; if the signal were
// not caught by execute()'s NotifyContext, it would kill the test run.
func TestExecuteCancelsOnInterrupt(t *testing.T) {
	probe := &cobra.Command{
		Use: "interrupt-probe",
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := os.FindProcess(os.Getpid())
			if err != nil {
				return err
			}
			if err := p.Signal(os.Interrupt); err != nil {
				return err
			}
			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case <-time.After(5 * time.Second):
				return errors.New("interrupt did not cancel the command context")
			}
		},
	}
	rootCmd.AddCommand(probe)
	t.Cleanup(func() { rootCmd.RemoveCommand(probe) })

	_, errBuf := captureRoot(t)

	rootCmd.SetArgs([]string{"interrupt-probe"})
	if code := execute(); code != 1 {
		t.Errorf("execute() = %d, want 1 (cancelled run)", code)
	}
	if !strings.Contains(errBuf.String(), "context canceled") {
		t.Errorf("want the cancellation to surface as the command's error; output:\n%s", errBuf.String())
	}
}
