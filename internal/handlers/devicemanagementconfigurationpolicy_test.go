package handlers

import (
	"testing"
)

func TestDeviceManagementConfigurationPolicyHandler_GetType(t *testing.T) {
	handler := &DeviceManagementConfigurationPolicyHandler{}

	expected := "Microsoft.Graph/deviceManagementConfigurationPolicies"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestDeviceManagementConfigurationPolicyHandler_GetTerraformResourceType(t *testing.T) {
	handler := &DeviceManagementConfigurationPolicyHandler{}

	expected := "microsoft365_graph_beta_device_management_settings_catalog_configuration_policy"
	result := handler.GetTerraformResourceType()

	if result != expected {
		t.Errorf("GetTerraformResourceType() = %q, want %q", result, expected)
	}
}

func TestExtractConfigurationPolicyID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "full path format",
			resourceID: "/deviceManagement/configurationPolicies/abc-123-def",
			expected:   "abc-123-def",
		},
		{
			name:       "direct policy ID",
			resourceID: "abc-123-def",
			expected:   "abc-123-def",
		},
		{
			name:       "UUID format",
			resourceID: "12345678-1234-1234-1234-123456789abc",
			expected:   "12345678-1234-1234-1234-123456789abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractConfigurationPolicyID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractConfigurationPolicyID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}
