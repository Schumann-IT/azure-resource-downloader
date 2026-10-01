package handlers

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"azure-resource-downloader/internal/logger"
)

// TestEmptyTypeLoggedOnceAtInfo guards that a type which listed to zero is
// reported as a plain empty at info level: the access check before listing
// refuses a run without access, so the old permissions caveat no longer
// applies, and the run summary reports the empty type again.
func TestEmptyTypeLoggedOnceAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger.Default.SetOutput(&buf)
	t.Cleanup(func() { logger.Default.SetOutput(os.Stderr) })

	r := NewEmptyRegistry()
	r.Register("Microsoft.Graph/empties", &MockHandler{resourceType: "Microsoft.Graph/empties"})

	_, _, empty, err := r.BuildFetchRequests(context.Background(), nil, "", []string{"Microsoft.Graph/empties"}, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 1 {
		t.Fatalf("empty types = %v, want the one type", empty)
	}
	out := buf.String()
	if n := strings.Count(out, "No resources found"); n != 1 {
		t.Errorf("\"No resources found\" logged %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "INFO No resources found") {
		t.Errorf("the empty type is not logged at info level:\n%s", out)
	}
	if strings.Contains(out, "Insufficient permissions") || strings.Contains(out, "note=") {
		t.Errorf("the empty type still carries the permissions note:\n%s", out)
	}
}
