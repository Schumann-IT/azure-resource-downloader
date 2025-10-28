package handlers

import (
	"testing"
)

func TestConditionalAccessPolicyHandler_GetType(t *testing.T) {
	// We can test GetType without a real credential
	handler := &ConditionalAccessPolicyHandler{}

	expected := "Microsoft.Graph/conditionalAccessPolicies"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestConditionalAccessPolicyHandler_GetTerraformResourceType(t *testing.T) {
	// We can test GetTerraformResourceType without a real credential
	handler := &ConditionalAccessPolicyHandler{}

	expected := "azuread_conditional_access_policy"
	result := handler.GetTerraformResourceType()

	if result != expected {
		t.Errorf("GetTerraformResourceType() = %q, want %q", result, expected)
	}
}

func TestExtractPolicyID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "full path format",
			resourceID: "/identity/conditionalAccess/policies/abc-123-def",
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
			result := extractPolicyID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractPolicyID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}

func TestSafeStringPtr(t *testing.T) {
	tests := []struct {
		name     string
		input    *string
		expected string
	}{
		{
			name:     "nil pointer",
			input:    nil,
			expected: "",
		},
		{
			name:     "valid string",
			input:    stringPtr("test value"),
			expected: "test value",
		},
		{
			name:     "empty string",
			input:    stringPtr(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeStringPtr(tt.input)
			if result != tt.expected {
				t.Errorf("safeStringPtr() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Helper function to create string pointers for testing
func stringPtr(s string) *string {
	return &s
}
