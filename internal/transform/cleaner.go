package transform

import (
	"reflect"
	"strings"

	"azure-resource-downloader/internal/logger"
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
// If cleanEmpty is true, removes keys with empty values (null, [], "", {})
func CleanProperties(props map[string]interface{}, globalKeys []string, typeSpecificKeys []string, cleanEmpty bool) map[string]interface{} {
	log := logger.Default

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

	// Track what keys were removed
	if len(allKeys) > 0 {
		removedKeys := findAndRemoveKeys(cleaned, allKeys)
		if len(removedKeys) > 0 {
			log.Debug("Removed excluded keys",
				"keys_removed", removedKeys,
				"count", len(removedKeys))
		}
	}

	// Conditionally remove empty values
	if cleanEmpty {
		removedEmpty := removeEmptyValuesWithTracking(cleaned)
		if len(removedEmpty) > 0 {
			log.Debug("Removed empty values",
				"keys_removed", removedEmpty,
				"count", len(removedEmpty))
		}
	}

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

// findAndRemoveKeys removes specified keys and returns which keys were actually found and removed
func findAndRemoveKeys(data map[string]interface{}, keys []string) []string {
	removed := []string{}

	for _, key := range keys {
		if strings.Contains(key, ".") {
			// Handle nested keys
			parts := strings.SplitN(key, ".", 2)
			if nested, ok := data[parts[0]].(map[string]interface{}); ok {
				nestedRemoved := findAndRemoveKeys(nested, []string{parts[1]})
				for _, r := range nestedRemoved {
					removed = append(removed, parts[0]+"."+r)
				}
			}
		} else {
			if _, exists := data[key]; exists {
				delete(data, key)
				removed = append(removed, key)
			}
		}
	}

	// Recursively clean nested maps
	for _, value := range data {
		if nested, ok := value.(map[string]interface{}); ok {
			findAndRemoveKeys(nested, keys)
		} else if slice, ok := value.([]interface{}); ok {
			for _, item := range slice {
				if nestedMap, ok := item.(map[string]interface{}); ok {
					findAndRemoveKeys(nestedMap, keys)
				}
			}
		}
	}

	return removed
}

// removeKeys removes specified keys from the map (supports nested paths with dots)
// This is kept for backward compatibility and non-logging use cases
func removeKeys(data map[string]interface{}, keys []string) {
	findAndRemoveKeys(data, keys)
}

// removeEmptyValuesWithTracking removes empty values and returns which keys were removed
func removeEmptyValuesWithTracking(data map[string]interface{}) []string {
	removed := []string{}
	removeEmptyValuesWithPath(data, "", &removed)
	return removed
}

// removeEmptyValuesWithPath recursively removes empty values and tracks the path
func removeEmptyValuesWithPath(data map[string]interface{}, path string, removed *[]string) {
	for key, value := range data {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		if value == nil {
			delete(data, key)
			*removed = append(*removed, currentPath)
			continue
		}

		// Use reflection to check if it's a slice of any type
		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.String:
			if v.String() == "" {
				delete(data, key)
				*removed = append(*removed, currentPath)
			}
		case reflect.Map:
			if mapVal, ok := value.(map[string]interface{}); ok {
				removeEmptyValuesWithPath(mapVal, currentPath, removed)
				if len(mapVal) == 0 {
					delete(data, key)
					*removed = append(*removed, currentPath)
				}
			} else if v.Len() == 0 {
				delete(data, key)
				*removed = append(*removed, currentPath)
			}
		case reflect.Slice, reflect.Array:
			if v.Len() == 0 {
				// Empty slice/array of any type
				delete(data, key)
				*removed = append(*removed, currentPath)
			} else if slice, ok := value.([]interface{}); ok {
				// If it's []interface{}, clean nested elements
				cleanedSlice := cleanSlice(slice)
				if len(cleanedSlice) == 0 {
					delete(data, key)
					*removed = append(*removed, currentPath)
				} else {
					data[key] = cleanedSlice
				}
			}
			// For typed slices ([]string, etc.), just check if empty (already done above)
		}
	}
}

// removeEmptyValues removes null, empty strings, and empty maps/arrays
// Recursively cleans nested structures including maps inside arrays
// Handles both []interface{} and typed slices ([]string, []int, etc.)
func removeEmptyValues(data map[string]interface{}) {
	removed := []string{}
	removeEmptyValuesWithPath(data, "", &removed)
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
