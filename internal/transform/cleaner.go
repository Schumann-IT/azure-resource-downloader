package transform

import (
	"reflect"
	"strings"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
)

// KeyReplacement is an alias for models.KeyReplacement
type KeyReplacement = models.KeyReplacement

// CleanProperties removes unnecessary Azure metadata from resource properties
// globalKeys and typeSpecificKeys specify which keys to remove
// If cleanEmpty is true, removes keys with empty values (null, [], "", {})
func CleanProperties(props map[string]interface{}, globalKeys []string, typeSpecificKeys []string, cleanEmpty bool) map[string]interface{} {
	return CleanPropertiesWithPreserve(props, globalKeys, typeSpecificKeys, []string{}, cleanEmpty)
}

// CleanPropertiesWithPreserve is like CleanProperties but supports preserve-keys and replacements
// preserveKeys takes precedence - keys in this list won't be removed even if in remove lists
// Example: remove-keys: ["id"], preserve-keys: ["foo.id"] -> removes all "id" except "foo.id"
// replacements allow replacing nested values with specific fields
func CleanPropertiesWithPreserve(props map[string]interface{}, globalKeys []string, typeSpecificKeys []string, preserveKeys []string, cleanEmpty bool) map[string]interface{} {
	return CleanPropertiesWithReplace(props, globalKeys, typeSpecificKeys, preserveKeys, nil, cleanEmpty)
}

// CleanPropertiesWithReplace extends CleanPropertiesWithPreserve to support value replacements
func CleanPropertiesWithReplace(props map[string]interface{}, globalKeys []string, typeSpecificKeys []string, preserveKeys []string, replacements []KeyReplacement, cleanEmpty bool) map[string]interface{} {
	log := logger.Default

	if props == nil {
		return make(map[string]interface{})
	}

	// If no global keys provided, use empty list (don't apply defaults)
	if globalKeys == nil {
		globalKeys = []string{}
	}

	// Merge global and type-specific keys
	allKeys := make([]string, 0, len(globalKeys)+len(typeSpecificKeys))
	allKeys = append(allKeys, globalKeys...)
	allKeys = append(allKeys, typeSpecificKeys...)

	cleaned := deepCopy(props)

	// Apply replacements first (before removing keys)
	if len(replacements) > 0 {
		replacedCount := applyReplacements(cleaned, replacements)
		if replacedCount > 0 {
			log.Debug("Applied key replacements",
				"replacements", replacedCount)
		}
	}

	// Track what keys were removed and preserved
	if len(allKeys) > 0 {
		removedKeys, preservedKeys := findAndRemoveKeysWithPreserve(cleaned, allKeys, preserveKeys, "")
		if len(removedKeys) > 0 {
			log.Debug("Removed excluded keys",
				"keys_removed", removedKeys,
				"count", len(removedKeys))
		}
		if len(preservedKeys) > 0 {
			log.Debug("Preserved keys (excluded from removal)",
				"keys_preserved", preservedKeys,
				"count", len(preservedKeys))
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

// applyReplacements applies all replacement operations
func applyReplacements(data map[string]interface{}, replacements []KeyReplacement) int {
	log := logger.Default
	count := 0

	for _, repl := range replacements {
		// Get value from source path
		value := getValueAtPath(data, repl.From)
		if value != nil {
			// Set value at destination path
			if setValueAtPath(data, repl.To, value) {
				count++
				log.Debug("Replaced key value",
					"from", repl.From,
					"to", repl.To,
					"value_type", reflect.TypeOf(value).String())
			}
		} else {
			log.Debug("Replacement source not found",
				"from", repl.From,
				"to", repl.To)
		}
	}

	return count
}

// getValueAtPath retrieves a value at a nested path (e.g., "foo.bar.baz")
func getValueAtPath(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	parts := strings.Split(path, ".")
	current := interface{}(data)

	for _, part := range parts {
		if currentMap, ok := current.(map[string]interface{}); ok {
			if value, exists := currentMap[part]; exists {
				current = value
			} else {
				return nil
			}
		} else {
			return nil
		}
	}

	return current
}

// setValueAtPath sets a value at a nested path, creating intermediate maps as needed
func setValueAtPath(data map[string]interface{}, path string, value interface{}) bool {
	if path == "" {
		return false
	}

	parts := strings.Split(path, ".")
	current := data

	// Navigate to parent, creating intermediate maps
	for i := 0; i < len(parts)-1; i++ {
		if next, ok := current[parts[i]].(map[string]interface{}); ok {
			current = next
		} else {
			// Create intermediate map
			newMap := make(map[string]interface{})
			current[parts[i]] = newMap
			current = newMap
		}
	}

	// Set the final value
	current[parts[len(parts)-1]] = value
	return true
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

// findAndRemoveKeysWithPreserve removes keys with preserve-key support
// Returns: (removed keys, preserved keys)
func findAndRemoveKeysWithPreserve(data map[string]interface{}, removeKeys []string, preserveKeys []string, currentPath string) ([]string, []string) {
	log := logger.Default
	removed := []string{}
	preserved := []string{}

	for _, key := range removeKeys {
		if strings.Contains(key, ".") {
			// Handle nested keys like "properties.subnet.id"
			parts := strings.SplitN(key, ".", 2)
			if nested, ok := data[parts[0]].(map[string]interface{}); ok {
				nestedPath := parts[0]
				if currentPath != "" {
					nestedPath = currentPath + "." + parts[0]
				}
				nestedRemoved, nestedPreserved := findAndRemoveKeysWithPreserve(nested, []string{parts[1]}, preserveKeys, nestedPath)
				for _, r := range nestedRemoved {
					removed = append(removed, parts[0]+"."+r)
				}
				for _, p := range nestedPreserved {
					preserved = append(preserved, parts[0]+"."+p)
				}
			}
		} else {
			// Simple key removal
			fullPath := key
			if currentPath != "" {
				fullPath = currentPath + "." + key
			}

			// Check if this specific path should be preserved
			shouldPreserve := false
			for _, preserveKey := range preserveKeys {
				if preserveKey == fullPath {
					shouldPreserve = true
					preserved = append(preserved, fullPath)
					log.Debug("Preserving key (in preserve-keys list)",
						"key", key,
						"path", fullPath)
					break
				}
			}

			if !shouldPreserve {
				if _, exists := data[key]; exists {
					delete(data, key)
					removed = append(removed, key)
					log.Debug("Removed key",
						"key", key,
						"path", fullPath)
				}
			}
		}
	}

	// Recursively process nested structures
	for key, value := range data {
		fullPath := key
		if currentPath != "" {
			fullPath = currentPath + "." + key
		}

		if nested, ok := value.(map[string]interface{}); ok {
			findAndRemoveKeysWithPreserve(nested, removeKeys, preserveKeys, fullPath)
		} else if slice, ok := value.([]interface{}); ok {
			for _, item := range slice {
				if nestedMap, ok := item.(map[string]interface{}); ok {
					findAndRemoveKeysWithPreserve(nestedMap, removeKeys, preserveKeys, fullPath)
				}
			}
		}
	}

	return removed, preserved
}

// findAndRemoveKeys removes specified keys and returns which keys were actually found and removed
func findAndRemoveKeys(data map[string]interface{}, keys []string) []string {
	removed, _ := findAndRemoveKeysWithPreserve(data, keys, []string{}, "")
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
	log := logger.Default

	for key, value := range data {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		if value == nil {
			delete(data, key)
			*removed = append(*removed, currentPath)
			log.Debug("Removed null value",
				"key", key,
				"path", currentPath)
			continue
		}

		// Use reflection to check if it's a slice of any type
		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.String:
			if v.String() == "" {
				delete(data, key)
				*removed = append(*removed, currentPath)
				log.Debug("Removed empty string",
					"key", key,
					"path", currentPath)
			}
		case reflect.Map:
			if mapVal, ok := value.(map[string]interface{}); ok {
				removeEmptyValuesWithPath(mapVal, currentPath, removed)
				if len(mapVal) == 0 {
					delete(data, key)
					*removed = append(*removed, currentPath)
					log.Debug("Removed empty map",
						"key", key,
						"path", currentPath)
				}
			} else if v.Len() == 0 {
				delete(data, key)
				*removed = append(*removed, currentPath)
				log.Debug("Removed empty map (typed)",
					"key", key,
					"path", currentPath)
			}
		case reflect.Slice, reflect.Array:
			if v.Len() == 0 {
				// Empty slice/array of any type
				delete(data, key)
				*removed = append(*removed, currentPath)
				log.Debug("Removed empty array",
					"key", key,
					"path", currentPath,
					"type", v.Type().String())
			} else if slice, ok := value.([]interface{}); ok {
				// If it's []interface{}, clean nested elements
				cleanedSlice := cleanSlice(slice)
				if len(cleanedSlice) == 0 {
					delete(data, key)
					*removed = append(*removed, currentPath)
					log.Debug("Removed array (became empty after cleaning)",
						"key", key,
						"path", currentPath)
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
