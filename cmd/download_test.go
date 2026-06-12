package cmd

import (
	"context"
	"reflect"
	"testing"

	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"
)

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
			registry:       handlers.NewRegistry(),
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

			result, _, _, err := buildFetchRequests(context.Background(), tt.registry, tt.resourceIDs, tt.resourceGroup, tt.resourceTypes, tt.subscriptionID)

			if tt.expectError && err == nil {
				t.Errorf("buildFetchRequests() expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("buildFetchRequests() unexpected error: %v", err)
			}

			if len(result) != tt.expectedCount {
				t.Errorf("buildFetchRequests() returned %d requests, want %d", len(result), tt.expectedCount)
			}

			// Verify resource IDs case
			if len(tt.resourceIDs) > 0 {
				for i, req := range result {
					if req.ResourceID != tt.resourceIDs[i] {
						t.Errorf("buildFetchRequests() request %d has ResourceID %q, want %q", i, req.ResourceID, tt.resourceIDs[i])
					}
					if req.Subscription != tt.subscriptionID {
						t.Errorf("buildFetchRequests() request %d has Subscription %q, want %q", i, req.Subscription, tt.subscriptionID)
					}
				}
			}

			// Verify resource group case
			if tt.resourceGroup != "" && len(result) > 0 {
				req := result[0]
				expectedID := "/subscriptions/" + tt.subscriptionID + "/resourceGroups/" + tt.resourceGroup
				if req.ResourceID != expectedID {
					t.Errorf("buildFetchRequests() ResourceID = %q, want %q", req.ResourceID, expectedID)
				}
				if req.ResourceGroup != tt.resourceGroup {
					t.Errorf("buildFetchRequests() ResourceGroup = %q, want %q", req.ResourceGroup, tt.resourceGroup)
				}
				if req.ResourceType != "Microsoft.Resources/resourceGroups" {
					t.Errorf("buildFetchRequests() ResourceType = %q, want %q", req.ResourceType, "Microsoft.Resources/resourceGroups")
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

	requests, _, _, err := buildFetchRequests(context.Background(), nil, resourceIDs, "", nil, subscriptionID)
	if err != nil {
		t.Fatalf("buildFetchRequests() unexpected error: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("buildFetchRequests() returned %d requests, want 1", len(requests))
	}

	req := requests[0]
	if req.Subscription != subscriptionID {
		t.Errorf("buildFetchRequests() Subscription = %q, want %q", req.Subscription, subscriptionID)
	}
}

func TestBuildFetchRequestsResourceGroup(t *testing.T) {
	subscriptionID := "sub-123"
	resourceGroup := "my-rg"

	requests, _, _, err := buildFetchRequests(context.Background(), nil, []string{}, resourceGroup, nil, subscriptionID)
	if err != nil {
		t.Fatalf("buildFetchRequests() unexpected error: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("buildFetchRequests() returned %d requests, want 1", len(requests))
	}

	expected := &models.FetchRequest{
		ResourceID:    "/subscriptions/sub-123/resourceGroups/my-rg",
		ResourceType:  "Microsoft.Resources/resourceGroups",
		ResourceGroup: "my-rg",
		Subscription:  "sub-123",
	}

	if !reflect.DeepEqual(requests[0], expected) {
		t.Errorf("buildFetchRequests() = %+v, want %+v", requests[0], expected)
	}
}
