package handlers

import (
	"testing"
)

func TestConditionalAccessPolicyHandler_GetType(t *testing.T) {
	handler, err := NewConditionalAccessPolicyHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewConditionalAccessPolicyHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/conditionalAccessPolicies"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestConditionalAccessPolicyHandler_GetTerraformResourceType(t *testing.T) {
	handler, err := NewConditionalAccessPolicyHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewConditionalAccessPolicyHandler() unexpected error: %v", err)
	}

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
			result := extractGraphItemID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractGraphItemID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
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
			input:    stringPointer("test"),
			expected: "test",
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
