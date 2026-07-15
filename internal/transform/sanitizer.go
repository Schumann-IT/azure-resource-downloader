package transform

import (
	"regexp"
	"strings"

	"azure-resource-downloader/internal/logger"
)

// SanitizeFileName converts a display name into a valid filename
func SanitizeFileName(displayName string) string {
	log := logger.Default

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

	// Ensure it doesn't start with a number (for file/system compatibility)
	if len(sanitized) > 0 && sanitized[0] >= '0' && sanitized[0] <= '9' {
		sanitized = "resource_" + sanitized
	}

	// If empty after sanitization, use default
	if sanitized == "" {
		sanitized = "unnamed"
	}

	// Log if name changed significantly
	if sanitized != displayName {
		log.Debug("Sanitized name",
			"original", displayName,
			"sanitized", sanitized)
	}

	return sanitized
}
