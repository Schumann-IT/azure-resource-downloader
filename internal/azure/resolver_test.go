package azure

import (
	"reflect"
	"testing"
)

func TestParseResourceID(t *testing.T) {
	tests := []struct {
		name        string
		resourceID  string
		expected    *ResourceIDInfo
		expectError bool
	}{
		{
			name:       "storage account",
			resourceID: "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected: &ResourceIDInfo{
				SubscriptionID: "sub-123",
				ResourceGroup:  "my-rg",
				Provider:       "Microsoft.Storage",
				ResourceType:   "storageAccounts",
				ResourceName:   "mystorageaccount",
				FullType:       "Microsoft.Storage/storageAccounts",
			},
			expectError: false,
		},
		{
			name:       "virtual machine",
			resourceID: "/subscriptions/sub-456/resourceGroups/vm-rg/providers/Microsoft.Compute/virtualMachines/myvm",
			expected: &ResourceIDInfo{
				SubscriptionID: "sub-456",
				ResourceGroup:  "vm-rg",
				Provider:       "Microsoft.Compute",
				ResourceType:   "virtualMachines",
				ResourceName:   "myvm",
				FullType:       "Microsoft.Compute/virtualMachines",
			},
			expectError: false,
		},
		{
			name:       "resource group",
			resourceID: "/subscriptions/sub-789/resourceGroups/my-rg",
			expected: &ResourceIDInfo{
				SubscriptionID: "sub-789",
				ResourceGroup:  "my-rg",
			},
			expectError: false,
		},
		{
			name:       "trailing slash",
			resourceID: "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount/",
			expected: &ResourceIDInfo{
				SubscriptionID: "sub-123",
				ResourceGroup:  "my-rg",
				Provider:       "Microsoft.Storage",
				ResourceType:   "storageAccounts",
				ResourceName:   "mystorageaccount",
				FullType:       "Microsoft.Storage/storageAccounts",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseResourceID(tt.resourceID)

			if tt.expectError && err == nil {
				t.Errorf("ParseResourceID() expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("ParseResourceID() unexpected error: %v", err)
			}

			if !tt.expectError && !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseResourceID() = %+v, want %+v", result, tt.expected)
			}
		})
	}
}

func TestResolveResourceIDToName(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "storage account",
			resourceID: "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected:   "mystorageaccount",
		},
		{
			name:       "virtual machine",
			resourceID: "/subscriptions/sub-456/resourceGroups/vm-rg/providers/Microsoft.Compute/virtualMachines/myvm",
			expected:   "myvm",
		},
		{
			name:       "resource group",
			resourceID: "/subscriptions/sub-789/resourceGroups/my-rg",
			expected:   "my-rg",
		},
		{
			name:       "simple path",
			resourceID: "/some/path/to/resource-name",
			expected:   "resource-name",
		},
		{
			name:       "empty string",
			resourceID: "",
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveResourceIDToName(tt.resourceID)
			if result != tt.expected {
				t.Errorf("ResolveResourceIDToName() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestIsAzureResourceID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid resource ID",
			input:    "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected: true,
		},
		{
			name:     "valid resource ID with trailing slash",
			input:    "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachines/myvm/",
			expected: true,
		},
		{
			name:     "invalid - missing providers",
			input:    "/subscriptions/sub-123/resourceGroups/my-rg/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected: false,
		},
		{
			name:     "invalid - missing resourceGroups",
			input:    "/subscriptions/sub-123/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expected: false,
		},
		{
			name:     "not a resource ID",
			input:    "just-a-name",
			expected: false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAzureResourceID(tt.input)
			if result != tt.expected {
				t.Errorf("isAzureResourceID(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveIDsInProperties(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "resolve single resource ID",
			input: map[string]interface{}{
				"name":      "test-resource",
				"networkId": "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet",
			},
			expected: map[string]interface{}{
				"name":           "test-resource",
				"networkId":      "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet",
				"networkId_name": "my-vnet",
			},
		},
		{
			name: "resolve nested resource ID",
			input: map[string]interface{}{
				"name": "test-resource",
				"network": map[string]interface{}{
					"vnetId": "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet",
				},
			},
			expected: map[string]interface{}{
				"name": "test-resource",
				"network": map[string]interface{}{
					"vnetId":      "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/my-vnet",
					"vnetId_name": "my-vnet",
				},
			},
		},
		{
			name: "resolve resource IDs in array",
			input: map[string]interface{}{
				"name": "test-resource",
				"subnets": []interface{}{
					"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet1",
					"/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet2",
				},
			},
			expected: map[string]interface{}{
				"name": "test-resource",
				"subnets": []interface{}{
					map[string]interface{}{
						"id":   "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet1",
						"name": "vnet",
					},
					map[string]interface{}{
						"id":   "/subscriptions/sub-123/resourceGroups/my-rg/providers/Microsoft.Network/virtualNetworks/vnet/subnets/subnet2",
						"name": "vnet",
					},
				},
			},
		},
		{
			name: "non-resource ID strings preserved",
			input: map[string]interface{}{
				"name":     "test-resource",
				"location": "eastus",
				"tags":     "production",
			},
			expected: map[string]interface{}{
				"name":     "test-resource",
				"location": "eastus",
				"tags":     "production",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolveIDsInProperties(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ResolveIDsInProperties() = %v, want %v", result, tt.expected)
			}
		})
	}
}
