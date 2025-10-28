package transform

import (
	"strings"
)

// DefaultExcludeKeys returns the default list of keys to exclude from resources
func DefaultExcludeKeys() []string {
	return []string{
		"provisioningState",
		"creationTime",
		"changedTime",
		"correlationId",
		"etag",
		"managedBy",
		"sku.tier", // Often auto-derived
	}
}

// CleanProperties removes unnecessary Azure metadata from resource properties
// If globalKeys is empty, uses DefaultExcludeKeys()
// typeSpecificKeys are merged with globalKeys for the final exclusion list
func CleanProperties(props map[string]interface{}, globalKeys []string, typeSpecificKeys []string) map[string]interface{} {
	if props == nil {
		return make(map[string]interface{})
	}

	// Use default keys if no global keys provided
	if len(globalKeys) == 0 {
		globalKeys = DefaultExcludeKeys()
	}

	// Merge global and type-specific keys
	allKeys := make([]string, 0, len(globalKeys)+len(typeSpecificKeys))
	allKeys = append(allKeys, globalKeys...)
	allKeys = append(allKeys, typeSpecificKeys...)

	cleaned := deepCopy(props)
	removeKeys(cleaned, allKeys)
	removeEmptyValues(cleaned)

	return cleaned
}

// deepCopy creates a deep copy of a map
func deepCopy(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{})

	for key, value := range src {
		switch v := value.(type) {
		case map[string]interface{}:
			dst[key] = deepCopy(v)
		case []interface{}:
			dst[key] = deepCopySlice(v)
		default:
			dst[key] = v
		}
	}

	return dst
}

// deepCopySlice creates a deep copy of a slice
func deepCopySlice(src []interface{}) []interface{} {
	dst := make([]interface{}, len(src))

	for i, value := range src {
		switch v := value.(type) {
		case map[string]interface{}:
			dst[i] = deepCopy(v)
		case []interface{}:
			dst[i] = deepCopySlice(v)
		default:
			dst[i] = v
		}
	}

	return dst
}

// removeKeys removes specified keys from the map (supports nested paths with dots)
func removeKeys(data map[string]interface{}, keys []string) {
	for _, key := range keys {
		if strings.Contains(key, ".") {
			// Handle nested keys
			parts := strings.SplitN(key, ".", 2)
			if nested, ok := data[parts[0]].(map[string]interface{}); ok {
				removeKeys(nested, []string{parts[1]})
			}
		} else {
			delete(data, key)
		}
	}

	// Recursively clean nested maps
	for _, value := range data {
		if nested, ok := value.(map[string]interface{}); ok {
			removeKeys(nested, keys)
		} else if slice, ok := value.([]interface{}); ok {
			for _, item := range slice {
				if nestedMap, ok := item.(map[string]interface{}); ok {
					removeKeys(nestedMap, keys)
				}
			}
		}
	}
}

// removeEmptyValues removes null, empty strings, and empty maps/arrays
// Recursively cleans nested structures including maps inside arrays
func removeEmptyValues(data map[string]interface{}) {
	for key, value := range data {
		switch v := value.(type) {
		case nil:
			delete(data, key)
		case string:
			if v == "" {
				delete(data, key)
			}
		case map[string]interface{}:
			removeEmptyValues(v)
			if len(v) == 0 {
				delete(data, key)
			}
		case []interface{}:
			// Clean nested maps inside arrays
			cleanedSlice := cleanSlice(v)
			if len(cleanedSlice) == 0 {
				delete(data, key)
			} else {
				data[key] = cleanedSlice
			}
		}
	}
}

// cleanSlice recursively cleans maps inside arrays and removes empty elements
func cleanSlice(slice []interface{}) []interface{} {
	cleaned := make([]interface{}, 0, len(slice))

	for _, item := range slice {
		switch v := item.(type) {
		case nil:
			// Skip nil values
			continue
		case string:
			if v == "" {
				// Skip empty strings
				continue
			}
			cleaned = append(cleaned, v)
		case map[string]interface{}:
			// Recursively clean nested maps
			removeEmptyValues(v)
			if len(v) > 0 {
				cleaned = append(cleaned, v)
			}
		case []interface{}:
			// Recursively clean nested slices
			nestedCleaned := cleanSlice(v)
			if len(nestedCleaned) > 0 {
				cleaned = append(cleaned, nestedCleaned)
			}
		default:
			// Keep other types (numbers, bools, etc.)
			cleaned = append(cleaned, v)
		}
	}

	return cleaned
}
