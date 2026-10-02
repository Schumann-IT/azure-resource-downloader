package resource

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/runprep"
)

// parseResourceType extracts the resource type from a resource ID
func parseResourceType(resourceID string) string {
	parts := strings.Split(strings.Trim(resourceID, "/"), "/")

	for i, part := range parts {
		if strings.EqualFold(part, "providers") && i+2 < len(parts) {
			return parts[i+1] + "/" + parts[i+2]
		}
	}

	return ""
}

func TestBuildFetchRequests(t *testing.T) {
	tests := []struct {
		name           string
		registry       *handlers.Registry
		resourceIDs    []string
		resourceGroup  string
		resourceTypes  []string
		subscriptionID string
		expectedCount  int
		expectError    bool
		skipTypeTest   bool // Skip tests that require Azure client
	}{
		{
			name: "single resource ID",
			resourceIDs: []string{
				"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			},
			resourceGroup:  "",
			subscriptionID: "sub-123",
			expectedCount:  1,
			expectError:    false,
		},
		{
			name: "multiple resource IDs",
			resourceIDs: []string{
				"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/account1",
				"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/account2",
			},
			resourceGroup:  "",
			subscriptionID: "sub-123",
			expectedCount:  2,
			expectError:    false,
		},
		{
			name:           "resource group",
			resourceIDs:    []string{},
			resourceGroup:  "my-rg",
			subscriptionID: "sub-123",
			expectedCount:  1,
			expectError:    false,
		},
		{
			name:           "no inputs with empty registry yields no requests",
			registry:       handlers.NewEmptyRegistry(),
			resourceIDs:    []string{},
			resourceGroup:  "",
			subscriptionID: "sub-123",
			expectedCount:  0,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip resource type tests as they require a real Azure client
			if tt.skipTypeTest {
				t.Skip("Skipping test that requires Azure client")
			}

			result, _, _, err := tt.registry.BuildFetchRequests(context.Background(), tt.resourceIDs, tt.resourceGroup, tt.resourceTypes, tt.subscriptionID, 4)

			if tt.expectError && err == nil {
				t.Errorf("BuildFetchRequests() expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("BuildFetchRequests() unexpected error: %v", err)
			}

			if len(result) != tt.expectedCount {
				t.Errorf("BuildFetchRequests() returned %d requests, want %d", len(result), tt.expectedCount)
			}

			// Verify resource IDs case
			if len(tt.resourceIDs) > 0 {
				for i, req := range result {
					if req.ResourceID != tt.resourceIDs[i] {
						t.Errorf("BuildFetchRequests() request %d has ResourceID %q, want %q", i, req.ResourceID, tt.resourceIDs[i])
					}
					if req.Subscription != tt.subscriptionID {
						t.Errorf("BuildFetchRequests() request %d has Subscription %q, want %q", i, req.Subscription, tt.subscriptionID)
					}
				}
			}

			// Verify resource group case
			if tt.resourceGroup != "" && len(result) > 0 {
				req := result[0]
				expectedID := "/subscriptions/" + tt.subscriptionID + "/resourceGroups/" + tt.resourceGroup
				if req.ResourceID != expectedID {
					t.Errorf("BuildFetchRequests() ResourceID = %q, want %q", req.ResourceID, expectedID)
				}
				if req.ResourceGroup != tt.resourceGroup {
					t.Errorf("BuildFetchRequests() ResourceGroup = %q, want %q", req.ResourceGroup, tt.resourceGroup)
				}
				if req.ResourceType != "Microsoft.Resources/resourceGroups" {
					t.Errorf("BuildFetchRequests() ResourceType = %q, want %q", req.ResourceType, "Microsoft.Resources/resourceGroups")
				}
			}
		})
	}
}

func TestParseResourceType(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "storage account",
			resourceID: "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected:   "Microsoft.Storage/storageAccounts",
		},
		{
			name:       "virtual machine",
			resourceID: "/subscriptions/sub-456/resourceGroups/vm-rg/providers/Microsoft.Compute/virtualMachines/myvm",
			expected:   "Microsoft.Compute/virtualMachines",
		},
		{
			name:       "network interface",
			resourceID: "/subscriptions/sub-789/resourceGroups/net-rg/providers/Microsoft.Network/networkInterfaces/mynic",
			expected:   "Microsoft.Network/networkInterfaces",
		},
		{
			name:       "resource group (no provider)",
			resourceID: "/subscriptions/sub-123/resourceGroups/my-rg",
			expected:   "",
		},
		{
			name:       "invalid resource ID",
			resourceID: "/invalid/path",
			expected:   "",
		},
		{
			name:       "empty string",
			resourceID: "",
			expected:   "",
		},
		{
			name:       "case insensitive providers",
			resourceID: "/subscriptions/sub-123/resourceGroups/my-rg/Providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected:   "Microsoft.Storage/storageAccounts",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseResourceType(tt.resourceID)
			if result != tt.expected {
				t.Errorf("parseResourceType(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}

func TestBuildFetchRequestsValidation(t *testing.T) {
	// Test that subscription ID is properly set
	resourceIDs := []string{
		"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/account1",
	}
	subscriptionID := "sub-123"

	var registry *handlers.Registry
	requests, _, _, err := registry.BuildFetchRequests(context.Background(), resourceIDs, "", nil, subscriptionID, 4)
	if err != nil {
		t.Fatalf("BuildFetchRequests() unexpected error: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("BuildFetchRequests() returned %d requests, want 1", len(requests))
	}

	req := requests[0]
	if req.Subscription != subscriptionID {
		t.Errorf("BuildFetchRequests() Subscription = %q, want %q", req.Subscription, subscriptionID)
	}
}

// TestRebaselineClearDrift guards the re-baseline invalidation: a run that
// updated the export baseline clears the tenant's drift/ tree (a new baseline
// supersedes the observation by definition), a dry run clears nothing, and the
// sibling trees are never touched.
func TestRebaselineClearDrift(t *testing.T) {
	makeTenant := func(t *testing.T) string {
		t.Helper()
		tenantDir := t.TempDir()
		for _, dir := range []string{"resources", "docs", "drift"} {
			if err := os.MkdirAll(filepath.Join(tenantDir, dir), 0755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(tenantDir, "drift", "metadata.yaml"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		return tenantDir
	}

	t.Run("dry run clears nothing", func(t *testing.T) {
		tenantDir := makeTenant(t)
		rebaselineClearDrift(tenantDir, true)
		if _, err := os.Stat(filepath.Join(tenantDir, "drift")); err != nil {
			t.Errorf("dry run must leave the drift tree alone: %v", err)
		}
	})

	t.Run("real run clears the drift tree and only it", func(t *testing.T) {
		tenantDir := makeTenant(t)
		rebaselineClearDrift(tenantDir, false)
		if _, err := os.Stat(filepath.Join(tenantDir, "drift")); !os.IsNotExist(err) {
			t.Error("re-baseline must clear the drift tree")
		}
		for _, dir := range []string{"resources", "docs"} {
			if _, err := os.Stat(filepath.Join(tenantDir, dir)); err != nil {
				t.Errorf("sibling %s must survive: %v", dir, err)
			}
		}
	})

	t.Run("no drift tree is a no-op", func(t *testing.T) {
		tenantDir := t.TempDir()
		rebaselineClearDrift(tenantDir, false)
	})
}

// TestRebaselineClearConsistency pins the consistency/ tree's lifecycle: a
// re-baselining download clears it (the analysis describes one export), a dry
// run clears nothing, and the sibling trees are never touched.
func TestRebaselineClearConsistency(t *testing.T) {
	makeTenant := func(t *testing.T) string {
		t.Helper()
		tenantDir := t.TempDir()
		for _, dir := range []string{"resources", "docs", "drift", "consistency"} {
			if err := os.MkdirAll(filepath.Join(tenantDir, dir), 0755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(tenantDir, "consistency", "mechanical.yaml"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		return tenantDir
	}

	t.Run("dry run clears nothing", func(t *testing.T) {
		tenantDir := makeTenant(t)
		rebaselineClearConsistency(tenantDir, true)
		if _, err := os.Stat(filepath.Join(tenantDir, "consistency", "mechanical.yaml")); err != nil {
			t.Errorf("dry run must leave the consistency tree alone: %v", err)
		}
	})

	t.Run("real run clears the consistency tree and only it", func(t *testing.T) {
		tenantDir := makeTenant(t)
		rebaselineClearConsistency(tenantDir, false)
		if _, err := os.Stat(filepath.Join(tenantDir, "consistency")); !os.IsNotExist(err) {
			t.Error("re-baseline must clear the consistency tree")
		}
		for _, dir := range []string{"resources", "docs", "drift"} {
			if _, err := os.Stat(filepath.Join(tenantDir, dir)); err != nil {
				t.Errorf("sibling %s must survive: %v", dir, err)
			}
		}
	})

	t.Run("no consistency tree is a no-op", func(t *testing.T) {
		rebaselineClearConsistency(t.TempDir(), false)
	})
}

func TestBuildFetchRequestsResourceGroup(t *testing.T) {
	subscriptionID := "sub-123"
	resourceGroup := "my-rg"

	var registry *handlers.Registry
	requests, _, _, err := registry.BuildFetchRequests(context.Background(), []string{}, resourceGroup, nil, subscriptionID, 4)
	if err != nil {
		t.Fatalf("BuildFetchRequests() unexpected error: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("BuildFetchRequests() returned %d requests, want 1", len(requests))
	}

	expected := &models.FetchRequest{
		ResourceID:    "/subscriptions/sub-123/resourceGroups/my-rg",
		ResourceType:  "Microsoft.Resources/resourceGroups",
		ResourceGroup: "my-rg",
		Subscription:  "sub-123",
	}

	if !reflect.DeepEqual(requests[0], expected) {
		t.Errorf("BuildFetchRequests() = %+v, want %+v", requests[0], expected)
	}
}

func TestNothingListed(t *testing.T) {
	req := []*models.FetchRequest{{ResourceID: "id-1", ResourceType: "Microsoft.Graph/groups"}}
	empty := []string{"Microsoft.Graph/organizationalBranding"}

	tests := []struct {
		name       string
		requests   []*models.FetchRequest
		emptyTypes []string
		want       bool
	}{
		{name: "requests only", requests: req, want: false},
		{name: "empty types only", emptyTypes: empty, want: false},
		{name: "requests and empty types", requests: req, emptyTypes: empty, want: false},
		{name: "neither", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nothingListed(tt.requests, tt.emptyTypes); got != tt.want {
				t.Errorf("nothingListed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExecuteDownloadEmptyOnly(t *testing.T) {
	t.Run("real run", func(t *testing.T) {
		summary, err := executeDownload(context.Background(), &runprep.Prepared{DryRun: false}, nil, nil)
		if err != nil {
			t.Fatalf("executeDownload() error = %v", err)
		}
		if summary == nil {
			t.Fatal("executeDownload() returned a nil summary")
		}
		if summary.TotalResources != 0 || len(summary.Results) != 0 {
			t.Errorf("TotalResources=%d Results=%d, want 0 and 0", summary.TotalResources, len(summary.Results))
		}
		summary.MarkCompleteness()
		if !summary.Complete {
			t.Errorf("Complete = false (%s), want true", summary.IncompleteReason)
		}
	})
	t.Run("dry run", func(t *testing.T) {
		summary, err := executeDownload(context.Background(), &runprep.Prepared{DryRun: true}, nil, nil)
		if err != nil {
			t.Fatalf("executeDownload() error = %v", err)
		}
		if summary == nil || !summary.DryRun {
			t.Errorf("summary = %+v, want DryRun true", summary)
		}
	})
}
