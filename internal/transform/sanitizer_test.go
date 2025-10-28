package transform

import (
	"testing"
)

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple lowercase",
			input:    "myresource",
			expected: "myresource",
		},
		{
			name:     "mixed case",
			input:    "MyResource",
			expected: "myresource",
		},
		{
			name:     "with spaces",
			input:    "my resource name",
			expected: "my_resource_name",
		},
		{
			name:     "with hyphens",
			input:    "my-resource-name",
			expected: "my_resource_name",
		},
		{
			name:     "with special characters",
			input:    "my@resource#name$",
			expected: "myresourcename",
		},
		{
			name:     "starts with number",
			input:    "123resource",
			expected: "resource_123resource",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "unnamed",
		},
		{
			name:     "only special characters",
			input:    "@#$%",
			expected: "unnamed",
		},
		{
			name:     "leading and trailing underscores",
			input:    "_resource_",
			expected: "resource",
		},
		{
			name:     "multiple spaces and hyphens",
			input:    "my  --  resource",
			expected: "my_resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFileName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFileName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeTerraformName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "myresource",
			expected: "myresource",
		},
		{
			name:     "long name over 64 chars",
			input:    "this_is_a_very_long_resource_name_that_exceeds_sixty_four_characters_limit",
			expected: "this_is_a_very_long_resource_name_that_exceeds_sixty_four_charac",
		},
		{
			name:     "long name ending with underscore after truncation",
			input:    "this_is_a_very_long_resource_name_that_exceeds_the_limit_____",
			expected: "this_is_a_very_long_resource_name_that_exceeds_the_limit",
		},
		{
			name:     "with spaces and special chars",
			input:    "My Resource Name!",
			expected: "my_resource_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeTerraformName(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeTerraformName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			// Verify length constraint
			if len(result) > 64 {
				t.Errorf("SanitizeTerraformName(%q) returned name longer than 64 chars: %d", tt.input, len(result))
			}
		})
	}
}
