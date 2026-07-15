package graph

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestDeviceManagementConfigurationPolicyHandler_GetType(t *testing.T) {
	handler, err := NewDeviceManagementConfigurationPolicyHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewDeviceManagementConfigurationPolicyHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/deviceManagementConfigurationPolicies"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

// TestDeviceManagementConfigurationPolicyHandler_Transform verifies that
// Settings Catalog policies are named via their `name` field (these policies
// have no `displayName`).
func TestDeviceManagementConfigurationPolicyHandler_Transform(t *testing.T) {
	handler, err := NewDeviceManagementConfigurationPolicyHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewDeviceManagementConfigurationPolicyHandler() unexpected error: %v", err)
	}

	id := "policy-1"
	name := "Defender Baseline"
	policy := betamodels.NewDeviceManagementConfigurationPolicy()
	policy.SetId(&id)
	policy.SetName(&name)

	result, err := handler.Transform(policy)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if result.DisplayName != name {
		t.Errorf("Transform() DisplayName = %q, want %q", result.DisplayName, name)
	}
	if result.ID != id {
		t.Errorf("Transform() ID = %q, want %q", result.ID, id)
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
			result := extractGraphItemID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractGraphItemID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}
