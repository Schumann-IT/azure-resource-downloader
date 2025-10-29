package handlers

import (
	"testing"
)

func TestAuthenticationStrengthPolicyHandler_GetType(t *testing.T) {
	// We can test GetType without a real credential
	handler := &AuthenticationStrengthPolicyHandler{}

	expected := "Microsoft.Graph/authenticationStrengthPolicies"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestAuthenticationStrengthPolicyHandler_GetTerraformResourceType(t *testing.T) {
	// We can test GetTerraformResourceType without a real credential
	handler := &AuthenticationStrengthPolicyHandler{}

	expected := "azuread_authentication_strength_policy"
	result := handler.GetTerraformResourceType()

	if result != expected {
		t.Errorf("GetTerraformResourceType() = %q, want %q", result, expected)
	}
}

func TestExtractAuthStrengthPolicyID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "full path format",
			resourceID: "/policies/authenticationStrengthPolicies/abc-123-def",
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
			result := extractAuthStrengthPolicyID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractAuthStrengthPolicyID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}

func TestSafeStringValue(t *testing.T) {
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
			input:    stringPointer("test value"),
			expected: "test value",
		},
		{
			name:     "empty string",
			input:    stringPointer(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := safeStringValue(tt.input)
			if result != tt.expected {
				t.Errorf("safeStringValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// Helper function to create string pointers for testing
func stringPointer(s string) *string {
	return &s
}
