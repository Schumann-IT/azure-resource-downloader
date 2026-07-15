package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// fakeTokenCredential is an offline azcore.TokenCredential for constructor
// tests; no network calls are made at client construction time.
type fakeTokenCredential struct{}

func (fakeTokenCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "fake-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// newTestGraphCollectionHandler builds a handler against the DeviceCategory
// model, which is representative of all simple Graph collections.
func newTestGraphCollectionHandler() *GraphCollectionHandler {
	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceCategories",
		listIDs: func(_ context.Context) ([]string, error) {
			return []string{"id-1", "id-2"}, nil
		},
		fetchItem: func(_ context.Context, itemID string) (serialization.Parsable, error) {
			category := betamodels.NewDeviceCategory()
			category.SetId(&itemID)
			return category, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.DeviceCategoryable); ok {
				return safeStringValue(c.GetDisplayName())
			}
			return ""
		},
	}
}

func TestGraphCollectionHandlerGetters(t *testing.T) {
	h := newTestGraphCollectionHandler()

	if got := h.GetType(); got != "Microsoft.Graph/deviceCategories" {
		t.Errorf("GetType() = %q, want %q", got, "Microsoft.Graph/deviceCategories")
	}
}

func TestGraphCollectionHandlerList(t *testing.T) {
	h := newTestGraphCollectionHandler()

	ids, err := h.List(context.Background())
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("List() returned %d IDs, want 2", len(ids))
	}
}

func TestGraphCollectionHandlerFetch(t *testing.T) {
	h := newTestGraphCollectionHandler()

	resource, err := h.Fetch(context.Background(), "/deviceManagement/deviceCategories/cat-1")
	if err != nil {
		t.Fatalf("Fetch() unexpected error: %v", err)
	}
	category, ok := resource.(betamodels.DeviceCategoryable)
	if !ok {
		t.Fatalf("Fetch() returned %T, want DeviceCategoryable", resource)
	}
	if safeStringValue(category.GetId()) != "cat-1" {
		t.Errorf("Fetch() item ID = %q, want %q", safeStringValue(category.GetId()), "cat-1")
	}
}

func TestGraphCollectionHandlerFetchInvalidID(t *testing.T) {
	h := newTestGraphCollectionHandler()

	if _, err := h.Fetch(context.Background(), ""); err == nil {
		t.Error("Fetch(\"\") expected error, got nil")
	}
}

func TestGraphCollectionHandlerFetchError(t *testing.T) {
	h := newTestGraphCollectionHandler()
	h.fetchItem = func(_ context.Context, _ string) (serialization.Parsable, error) {
		return nil, errors.New("boom")
	}

	if _, err := h.Fetch(context.Background(), "cat-1"); err == nil {
		t.Error("Fetch() expected error from fetchItem, got nil")
	}
}

func TestGraphCollectionHandlerTransform(t *testing.T) {
	h := newTestGraphCollectionHandler()

	id := "cat-1"
	name := "Corporate Devices"
	description := "All corporate-owned devices"
	category := betamodels.NewDeviceCategory()
	category.SetId(&id)
	category.SetDisplayName(&name)
	category.SetDescription(&description)

	result, err := h.Transform(category)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}

	if result.ID != id {
		t.Errorf("Transform() ID = %q, want %q", result.ID, id)
	}
	if result.Type != "Microsoft.Graph/deviceCategories" {
		t.Errorf("Transform() Type = %q, want %q", result.Type, "Microsoft.Graph/deviceCategories")
	}
	if result.DisplayName != name {
		t.Errorf("Transform() DisplayName = %q, want %q", result.DisplayName, name)
	}
	if got, _ := result.Properties["displayName"].(string); got != name {
		t.Errorf("Transform() Properties[displayName] = %q, want %q", got, name)
	}
	if got, _ := result.Properties["description"].(string); got != description {
		t.Errorf("Transform() Properties[description] = %q, want %q", got, description)
	}
}

func TestGraphCollectionHandlerTransformInvalidType(t *testing.T) {
	h := newTestGraphCollectionHandler()

	if _, err := h.Transform("not a graph model"); err == nil {
		t.Error("Transform() expected error for non-Parsable resource, got nil")
	}
}

func TestGraphCollectionHandlerTransformEmptyDisplayName(t *testing.T) {
	h := newTestGraphCollectionHandler()

	category := betamodels.NewDeviceCategory()
	if _, err := h.Transform(category); err == nil {
		t.Error("Transform() expected error for empty display name, got nil")
	}
}

func TestExtractGraphItemID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		want       string
	}{
		{"bare ID", "abc-123", "abc-123"},
		{"full path", "/deviceManagement/assignmentFilters/abc-123", "abc-123"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractGraphItemID(tt.resourceID); got != tt.want {
				t.Errorf("extractGraphItemID(%q) = %q, want %q", tt.resourceID, got, tt.want)
			}
		})
	}
}
