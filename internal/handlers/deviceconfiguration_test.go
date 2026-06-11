package handlers

import (
	"testing"
)

func TestDeviceConfigurationHandler_GetType(t *testing.T) {
	handler := &DeviceConfigurationHandler{}

	expected := "Microsoft.Graph/deviceConfigurations"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestDeviceConfigurationHandler_GetTerraformResourceType(t *testing.T) {
	handler := &DeviceConfigurationHandler{}

	expected := "microsoft365_graph_beta_device_management_device_configuration"
	result := handler.GetTerraformResourceType()

	if result != expected {
		t.Errorf("GetTerraformResourceType() = %q, want %q", result, expected)
	}
}

func TestExtractDeviceConfigurationID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "full path format",
			resourceID: "/deviceManagement/deviceConfigurations/abc-123-def",
			expected:   "abc-123-def",
		},
		{
			name:       "direct profile ID",
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
			result := extractDeviceConfigurationID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractDeviceConfigurationID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}
