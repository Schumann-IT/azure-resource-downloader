package transform

import (
	"strings"
	"testing"
)

func TestGenerateTerraformImport(t *testing.T) {
	tests := []struct {
		name                  string
		terraformResourceType string
		resourceName          string
		azureResourceID       string
		expectedContains      []string
	}{
		{
			name:                  "simple storage account",
			terraformResourceType: "azurerm_storage_account",
			resourceName:          "mystorageaccount",
			azureResourceID:       "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expectedContains: []string{
				"terraform import",
				"azurerm_storage_account.mystorageaccount",
				"/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			},
		},
		{
			name:                  "resource group",
			terraformResourceType: "azurerm_resource_group",
			resourceName:          "my-resource-group",
			azureResourceID:       "/subscriptions/sub-id/resourceGroups/my-resource-group",
			expectedContains: []string{
				"terraform import",
				"azurerm_resource_group.my_resource_group",
			},
		},
		{
			name:                  "name with special characters",
			terraformResourceType: "azurerm_virtual_machine",
			resourceName:          "My VM @123!",
			azureResourceID:       "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm-123",
			expectedContains: []string{
				"terraform import",
				"azurerm_virtual_machine.my_vm_123",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTerraformImport(tt.terraformResourceType, tt.resourceName, tt.azureResourceID)

			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("GenerateTerraformImport() = %q, want to contain %q", result, expected)
				}
			}
		})
	}
}

func TestGenerateTerraformImportBlock(t *testing.T) {
	tests := []struct {
		name                  string
		terraformResourceType string
		resourceName          string
		azureResourceID       string
		expectedContains      []string
	}{
		{
			name:                  "import block format",
			terraformResourceType: "azurerm_storage_account",
			resourceName:          "mystorageaccount",
			azureResourceID:       "/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			expectedContains: []string{
				"import {",
				"to = azurerm_storage_account.mystorageaccount",
				"id = \"/subscriptions/sub-id/resourceGroups/rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount\"",
				"}",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTerraformImportBlock(tt.terraformResourceType, tt.resourceName, tt.azureResourceID)

			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("GenerateTerraformImportBlock() does not contain %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestGenerateTerraformResourceStub(t *testing.T) {
	tests := []struct {
		name                  string
		terraformResourceType string
		resourceName          string
		properties            map[string]interface{}
		expectedContains      []string
	}{
		{
			name:                  "basic resource with name and location",
			terraformResourceType: "azurerm_storage_account",
			resourceName:          "mystorageaccount",
			properties: map[string]interface{}{
				"name":     "mystorageaccount",
				"location": "eastus",
			},
			expectedContains: []string{
				"resource \"azurerm_storage_account\" \"mystorageaccount\"",
				"name = \"mystorageaccount\"",
				"location = \"eastus\"",
			},
		},
		{
			name:                  "resource with resource group extracted from id",
			terraformResourceType: "azurerm_storage_account",
			resourceName:          "mystorageaccount",
			properties: map[string]interface{}{
				"name":     "mystorageaccount",
				"location": "eastus",
				"id":       "/subscriptions/sub-id/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			},
			expectedContains: []string{
				"resource \"azurerm_storage_account\" \"mystorageaccount\"",
				"name = \"mystorageaccount\"",
				"location = \"eastus\"",
				"resource_group_name = \"my-rg\"",
			},
		},
		{
			name:                  "resource with explicit resource group property",
			terraformResourceType: "azurerm_storage_account",
			resourceName:          "mystorageaccount",
			properties: map[string]interface{}{
				"name":          "mystorageaccount",
				"location":      "eastus",
				"resourceGroup": "my-rg",
			},
			expectedContains: []string{
				"resource \"azurerm_storage_account\" \"mystorageaccount\"",
				"resource_group_name = \"my-rg\"",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateTerraformResourceStub(tt.terraformResourceType, tt.resourceName, tt.properties)

			for _, expected := range tt.expectedContains {
				if !strings.Contains(result, expected) {
					t.Errorf("GenerateTerraformResourceStub() does not contain %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestExtractResourceGroup(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]interface{}
		expected   string
	}{
		{
			name: "extract from id",
			properties: map[string]interface{}{
				"id": "/subscriptions/sub-id/resourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			},
			expected: "my-rg",
		},
		{
			name: "extract from resourceGroup property",
			properties: map[string]interface{}{
				"resourceGroup": "my-rg",
			},
			expected: "my-rg",
		},
		{
			name:       "no resource group info",
			properties: map[string]interface{}{},
			expected:   "",
		},
		{
			name: "case insensitive resourceGroups in id",
			properties: map[string]interface{}{
				"id": "/subscriptions/sub-id/ResourceGroups/my-rg/providers/Microsoft.Storage/storageAccounts/mystorageaccount",
			},
			expected: "my-rg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractResourceGroup(tt.properties)
			if result != tt.expected {
				t.Errorf("extractResourceGroup() = %q, want %q", result, tt.expected)
			}
		})
	}
}
