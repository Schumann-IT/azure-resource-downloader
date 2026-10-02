package graph

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/logger"

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

func TestGraphCollectionHandlerAccessProbe(t *testing.T) {
	t.Run("without a probe the listing stands in", func(t *testing.T) {
		listed := 0
		h := &GraphCollectionHandler{listIDs: func(context.Context) ([]string, error) {
			listed++
			return nil, errors.New("refused")
		}}
		if h.HasAccessProbe() {
			t.Error("HasAccessProbe() = true without a probe")
		}
		if err := h.ProbeAccess(context.Background()); err == nil || listed != 1 {
			t.Errorf("ProbeAccess() = %v after %d listings, want the listing's error after one", err, listed)
		}
	})
	t.Run("a probe replaces the listing", func(t *testing.T) {
		listed, probed := 0, 0
		h := &GraphCollectionHandler{
			listIDs: func(context.Context) ([]string, error) { listed++; return nil, nil },
			probe:   func(context.Context) error { probed++; return errors.New("refused") },
		}
		if !h.HasAccessProbe() {
			t.Error("HasAccessProbe() = false with a probe")
		}
		if err := h.ProbeAccess(context.Background()); err == nil || probed != 1 || listed != 0 {
			t.Errorf("ProbeAccess() = %v (probed %d, listed %d), want the probe's error, one probe, no listing", err, probed, listed)
		}
	})
}

// TestAccessProbesPerGroup guards that every collection permission group the
// plan names has a type with a dedicated probe, so the access check costs one
// small request per group rather than a full listing.
func TestAccessProbesPerGroup(t *testing.T) {
	cred := fakeTokenCredential{}
	dc, err := NewDeviceConfigurationHandler(cred, false)
	if err != nil {
		t.Fatal(err)
	}
	constructors := map[string]func(azcore.TokenCredential) (*GraphCollectionHandler, error){
		"DeviceManagementApps":           NewMobileAppHandler,
		"DeviceManagementServiceConfig":  NewTermsAndConditionsHandler,
		"DeviceManagementScripts":        NewWindowsPlatformScriptHandler,
		"DeviceManagementRBAC":           NewRoleScopeTagHandler,
		"DeviceManagementManagedDevices": NewDeviceCategoryHandler,
		"Policy":                         NewConditionalAccessPolicyHandler,
		"Agreement":                      NewTermsOfUseAgreementHandler,
		"Group":                          NewGroupHandler,
	}
	if !dc.HasAccessProbe() {
		t.Error("DeviceManagementConfiguration: device configurations have no access probe")
	}
	for group, newHandler := range constructors {
		h, err := newHandler(cred)
		if err != nil {
			t.Fatalf("%s: %v", group, err)
		}
		if !h.HasAccessProbe() {
			t.Errorf("%s: %s has no access probe", group, h.GetType())
		}
		if perms := h.RequiredPermissions(); len(perms) == 0 || !strings.HasPrefix(perms[0], group+".") {
			t.Errorf("%s: %s declares %v, not a permission of the group", group, h.GetType(), perms)
		}
	}
}

// TestWarnUnexpectedAssignments verifies the warning for assignments returned
// by a type without an assignments concept: logged with type, id and count when
// entries came back, silent for none. Not parallel: it swaps the default logger's
// output.
func TestWarnUnexpectedAssignments(t *testing.T) {
	var buf bytes.Buffer
	logger.Default.SetOutput(&buf)
	t.Cleanup(func() { logger.Default.SetOutput(os.Stderr) })

	warnUnexpectedAssignments("Microsoft.Graph/deviceComplianceScripts", "script-1", 2)
	out := buf.String()
	for _, want := range []string{
		"assignments returned for a type without an assignments concept; exported but not documented",
		"Microsoft.Graph/deviceComplianceScripts",
		"script-1",
		"count=2",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("warning %q missing %q", out, want)
		}
	}

	buf.Reset()
	warnUnexpectedAssignments("Microsoft.Graph/deviceComplianceScripts", "script-1", 0)
	if buf.Len() != 0 {
		t.Errorf("unexpected output for no assignments: %q", buf.String())
	}
}
