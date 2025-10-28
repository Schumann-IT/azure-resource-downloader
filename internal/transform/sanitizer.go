package transform

import (
	"regexp"
	"strings"
)

// SanitizeFileName converts a display name into a valid filename
func SanitizeFileName(displayName string) string {
	if displayName == "" {
		return "unnamed"
	}

	// Convert to lowercase
	sanitized := strings.ToLower(displayName)

	// Replace spaces and multiple hyphens/underscores with single underscore
	sanitized = regexp.MustCompile(`[\s\-]+`).ReplaceAllString(sanitized, "_")

	// Remove special characters, keep only alphanumeric and underscores
	sanitized = regexp.MustCompile(`[^a-z0-9_]`).ReplaceAllString(sanitized, "")

	// Remove leading/trailing underscores
	sanitized = strings.Trim(sanitized, "_")

	// Ensure it doesn't start with a number (for Terraform compatibility)
	if len(sanitized) > 0 && sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "resource_" + sanitized
	}

	// If empty after sanitization, use default
	if sanitized == "" {
		sanitized = "unnamed"
	}

	return sanitized
}

// SanitizeTerraformName creates a valid Terraform resource name
func SanitizeTerraformName(name string) string {
	// Similar to filename but enforce stricter rules for Terraform
	sanitized := SanitizeFileName(name)

	// Terraform names can't be too long (limit to 64 chars)
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}

	// Remove trailing underscore if truncated
	sanitized = strings.TrimRight(sanitized, "_")

	return sanitized
}
