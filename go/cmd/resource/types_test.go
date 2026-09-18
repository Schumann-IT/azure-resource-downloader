package resource

import (
	"context"
	"testing"

	"azure-resource-downloader/internal/models"
)

// fakeHandler is a minimal models.ResourceHandler used to pin down the derived
// handler identity without any Azure dependency.
type fakeHandler struct{}

func (fakeHandler) GetType() string                { return "Microsoft.Test/fakes" }
func (fakeHandler) GetDocumentationPrompt() string { return "" }
func (fakeHandler) List(context.Context) ([]string, error) {
	return nil, nil
}
func (fakeHandler) Fetch(context.Context, string) (interface{}, error) {
	return nil, nil
}
func (fakeHandler) Transform(interface{}) (*models.TransformedResource, error) {
	return nil, nil
}

// TestHandlerIdentity guards the decision to derive the handler column from the
// registered handler's Go type instead of widening the ResourceHandler
// interface: the derivation must be readable (package-qualified, no pointer
// noise) or that decision needs revisiting.
func TestHandlerIdentity(t *testing.T) {
	if got := handlerIdentity(fakeHandler{}); got != "resource.fakeHandler" {
		t.Errorf("handlerIdentity(value) = %q, want %q", got, "resource.fakeHandler")
	}
	if got := handlerIdentity(&fakeHandler{}); got != "resource.fakeHandler" {
		t.Errorf("handlerIdentity(pointer) = %q, want %q", got, "resource.fakeHandler")
	}
}

// TestTenantCountsNoTypes pins the earliest degradation of the counting path:
// with nothing selected there is nothing to count, and the caller must receive
// nil counts plus a reason for its omission note — never an error, because
// failing to count may never fail the command.
func TestTenantCountsNoTypes(t *testing.T) {
	counts, unknown, reason := tenantCounts(context.Background(), "", "", "", nil, 4)
	if counts != nil || unknown != nil {
		t.Errorf("tenantCounts() = %v, %v; want nil counts for an empty selection", counts, unknown)
	}
	if reason == "" {
		t.Error("tenantCounts() returned no omission reason for an empty selection")
	}
}
