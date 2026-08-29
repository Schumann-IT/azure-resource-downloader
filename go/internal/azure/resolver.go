package azure

import (
	"regexp"
	"strings"

	"azure-resource-downloader/internal/logger"
)

// ResourceIDInfo contains parsed information from an Azure resource ID
type ResourceIDInfo struct {
	SubscriptionID string
	ResourceGroup  string
	Provider       string
	ResourceType   string
	ResourceName   string
	FullType       string // e.g., "Microsoft.Storage/storageAccounts"
}

// ParseResourceID parses an Azure resource ID into its components
func ParseResourceID(resourceID string) (*ResourceIDInfo, error) {
	// Azure Resource ID format:
	// /subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/{resourceProviderNamespace}/{resourceType}/{resourceName}

	parts := strings.Split(strings.Trim(resourceID, "/"), "/")

	info := &ResourceIDInfo{}

	for i := 0; i < len(parts)-1; i += 2 {
		key := strings.ToLower(parts[i])
		value := parts[i+1]

		switch key {
		case "subscriptions":
			info.SubscriptionID = value
		case "resourcegroups":
			info.ResourceGroup = value
		case "providers":
			info.Provider = value
			// Resource type follows provider
			if i+3 < len(parts) {
				info.ResourceType = parts[i+2]
				info.ResourceName = parts[i+3]
				info.FullType = info.Provider + "/" + info.ResourceType
			}
		}
	}

	return info, nil
}

// ResolveResourceIDToName extracts the resource name from a resource ID
func ResolveResourceIDToName(resourceID string) string {
	info, err := ParseResourceID(resourceID)
	if err != nil || info.ResourceName == "" {
		// Fallback: get the last segment
		parts := strings.Split(strings.Trim(resourceID, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return "unknown"
	}

	return info.ResourceName
}

// ResolveIDsInProperties recursively finds and resolves Azure resource IDs in properties
func ResolveIDsInProperties(properties map[string]interface{}) map[string]interface{} {
	log := logger.Default
	resolvedIDs := []string{}

	result := resolveIDsWithTracking(properties, "", &resolvedIDs)

	if len(resolvedIDs) > 0 {
		log.Debug("Resolved resource IDs to names",
			"ids_resolved", resolvedIDs,
			"count", len(resolvedIDs))
	}

	return result
}

// resolveIDsWithTracking recursively resolves IDs and tracks which ones were resolved
func resolveIDsWithTracking(properties map[string]interface{}, path string, resolvedIDs *[]string) map[string]interface{} {
	result := make(map[string]interface{})

	for key, value := range properties {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		switch v := value.(type) {
		case string:
			// Check if this looks like an Azure resource ID
			if isAzureResourceID(v) {
				result[key] = v
				// Add a resolved name field
				resolvedName := ResolveResourceIDToName(v)
				result[key+"_name"] = resolvedName
				*resolvedIDs = append(*resolvedIDs, currentPath)
			} else {
				result[key] = v
			}
		case map[string]interface{}:
			result[key] = resolveIDsWithTracking(v, currentPath, resolvedIDs)
		case []interface{}:
			result[key] = resolveIDsInSliceWithTracking(v, currentPath, resolvedIDs)
		default:
			result[key] = v
		}
	}

	return result
}

// resolveIDsInSliceWithTracking handles arrays with tracking
func resolveIDsInSliceWithTracking(slice []interface{}, path string, resolvedIDs *[]string) []interface{} {
	result := make([]interface{}, len(slice))

	for i, item := range slice {
		currentPath := path + "[" + string(rune('0'+i)) + "]"

		switch v := item.(type) {
		case string:
			if isAzureResourceID(v) {
				result[i] = map[string]interface{}{
					"id":   v,
					"name": ResolveResourceIDToName(v),
				}
				*resolvedIDs = append(*resolvedIDs, currentPath)
			} else {
				result[i] = v
			}
		case map[string]interface{}:
			result[i] = resolveIDsWithTracking(v, currentPath, resolvedIDs)
		case []interface{}:
			result[i] = resolveIDsInSliceWithTracking(v, currentPath, resolvedIDs)
		default:
			result[i] = v
		}
	}

	return result
}

// isAzureResourceID checks if a string looks like an Azure resource ID
func isAzureResourceID(s string) bool {
	// Azure resource IDs follow a specific pattern
	pattern := `^/subscriptions/[^/]+/resourceGroups/[^/]+/providers/[^/]+/`
	matched, _ := regexp.MatchString(pattern, s)
	return matched
}
