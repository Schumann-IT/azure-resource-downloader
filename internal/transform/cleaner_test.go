package transform

import (
	"reflect"
	"testing"
)

func TestCleanProperties(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "nil map",
			input:    nil,
			expected: map[string]interface{}{},
		},
		{
			name: "remove provisioning state",
			input: map[string]interface{}{
				"name":              "test-resource",
				"provisioningState": "Succeeded",
				"location":          "eastus",
			},
			expected: map[string]interface{}{
				"name":     "test-resource",
				"location": "eastus",
			},
		},
		{
			name: "remove empty values",
			input: map[string]interface{}{
				"name":        "test-resource",
				"emptyString": "",
				"location":    "eastus",
				"emptyMap":    map[string]interface{}{},
				"emptySlice":  []interface{}{},
			},
			expected: map[string]interface{}{
				"name":     "test-resource",
				"location": "eastus",
			},
		},
		{
			name: "remove metadata keys",
			input: map[string]interface{}{
				"name":          "test-resource",
				"creationTime":  "2024-01-01",
				"changedTime":   "2024-01-02",
				"correlationId": "abc123",
				"etag":          "xyz789",
				"managedBy":     "system",
				"location":      "eastus",
			},
			expected: map[string]interface{}{
				"name":     "test-resource",
				"location": "eastus",
			},
		},
		{
			name: "nested map with metadata",
			input: map[string]interface{}{
				"name": "test-resource",
				"properties": map[string]interface{}{
					"value":             "keep-this",
					"provisioningState": "Succeeded",
					"etag":              "xyz789",
				},
			},
			expected: map[string]interface{}{
				"name": "test-resource",
				"properties": map[string]interface{}{
					"value": "keep-this",
				},
			},
		},
		{
			name: "array with nested maps",
			input: map[string]interface{}{
				"name": "test-resource",
				"items": []interface{}{
					map[string]interface{}{
						"value":             "item1",
						"provisioningState": "Succeeded",
					},
					map[string]interface{}{
						"value": "item2",
						"etag":  "abc123",
					},
				},
			},
			expected: map[string]interface{}{
				"name": "test-resource",
				"items": []interface{}{
					map[string]interface{}{
						"value": "item1",
					},
					map[string]interface{}{
						"value": "item2",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanProperties(tt.input, nil, nil) // nil uses default keys
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("CleanProperties() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCleanPropertiesCustomKeys(t *testing.T) {
	input := map[string]interface{}{
		"id":       "/subscriptions/.../resourceGroups/my-rg",
		"name":     "my-rg",
		"location": "eastus",
		"etag":     "xyz123",
	}

	// Test with custom global exclude keys
	globalKeys := []string{"id", "etag"}
	result := CleanProperties(input, globalKeys, nil)

	expected := map[string]interface{}{
		"name":     "my-rg",
		"location": "eastus",
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("CleanProperties() with custom keys = %v, want %v", result, expected)
	}
}

func TestCleanPropertiesTypeSpecificKeys(t *testing.T) {
	input := map[string]interface{}{
		"id":                "/subscriptions/.../resourceGroups/my-rg",
		"name":              "my-rg",
		"location":          "eastus",
		"etag":              "xyz123",
		"provisioningState": "Succeeded",
		"managedBy":         "system",
	}

	// Test with both global and type-specific keys
	globalKeys := []string{"etag", "provisioningState"} // Exclude these globally
	typeSpecificKeys := []string{"id", "managedBy"}     // Exclude these for this type
	result := CleanProperties(input, globalKeys, typeSpecificKeys)

	expected := map[string]interface{}{
		"name":     "my-rg",
		"location": "eastus",
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("CleanProperties() with type-specific keys = %v, want %v", result, expected)
	}
}

func TestDeepCopy(t *testing.T) {
	original := map[string]interface{}{
		"name": "test",
		"nested": map[string]interface{}{
			"value": "nested-value",
		},
		"array": []interface{}{1, 2, 3},
	}

	copied := deepCopy(original)

	// Verify it's a copy
	if !reflect.DeepEqual(original, copied) {
		t.Errorf("deepCopy() did not create equal copy")
	}

	// Modify copy and verify original is unchanged
	copied["name"] = "modified"
	if original["name"] == "modified" {
		t.Errorf("deepCopy() did not create independent copy")
	}

	// Modify nested map
	if nestedCopy, ok := copied["nested"].(map[string]interface{}); ok {
		nestedCopy["value"] = "modified-nested"
		if nestedOriginal, ok := original["nested"].(map[string]interface{}); ok {
			if nestedOriginal["value"] == "modified-nested" {
				t.Errorf("deepCopy() did not deep copy nested maps")
			}
		}
	}
}

func TestDeepCopySlice(t *testing.T) {
	original := []interface{}{
		"string",
		123,
		map[string]interface{}{"key": "value"},
		[]interface{}{1, 2, 3},
	}

	copied := deepCopySlice(original)

	if !reflect.DeepEqual(original, copied) {
		t.Errorf("deepCopySlice() did not create equal copy")
	}

	// Modify copy and verify original is unchanged
	copied[0] = "modified"
	if original[0] == "modified" {
		t.Errorf("deepCopySlice() did not create independent copy")
	}
}

func TestRemoveEmptyValues(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name: "remove nil values",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": nil,
				"key3": "value3",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key3": "value3",
			},
		},
		{
			name: "remove empty strings",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": "",
				"key3": "value3",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key3": "value3",
			},
		},
		{
			name: "remove empty maps",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": map[string]interface{}{},
				"key3": "value3",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key3": "value3",
			},
		},
		{
			name: "remove empty slices",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": []interface{}{},
				"key3": "value3",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key3": "value3",
			},
		},
		{
			name: "keep non-empty nested structures",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": map[string]interface{}{"nested": "value"},
				"key3": []interface{}{1, 2, 3},
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": map[string]interface{}{"nested": "value"},
				"key3": []interface{}{1, 2, 3},
			},
		},
		{
			name: "remove empty maps from arrays",
			input: map[string]interface{}{
				"key1": "value1",
				"items": []interface{}{
					map[string]interface{}{"name": "item1"},
					map[string]interface{}{}, // empty map
					map[string]interface{}{"name": "item2"},
				},
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"items": []interface{}{
					map[string]interface{}{"name": "item1"},
					map[string]interface{}{"name": "item2"},
				},
			},
		},
		{
			name: "remove nil and empty strings from arrays",
			input: map[string]interface{}{
				"key1": "value1",
				"items": []interface{}{
					"valid",
					nil,
					"",
					"another",
				},
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"items": []interface{}{
					"valid",
					"another",
				},
			},
		},
		{
			name: "remove arrays that become empty after cleaning",
			input: map[string]interface{}{
				"key1": "value1",
				"items": []interface{}{
					nil,
					"",
					map[string]interface{}{},
				},
			},
			expected: map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			name: "deeply nested empty structures",
			input: map[string]interface{}{
				"key1": "value1",
				"nested": map[string]interface{}{
					"level1": map[string]interface{}{
						"level2": map[string]interface{}{
							"emptyArray": []interface{}{},
							"emptyMap":   map[string]interface{}{},
							"emptyStr":   "",
						},
					},
				},
			},
			expected: map[string]interface{}{
				"key1": "value1",
			},
		},
		{
			name: "complex nested structure with mixed empty values",
			input: map[string]interface{}{
				"name": "resource",
				"properties": map[string]interface{}{
					"validProp": "value",
					"emptyProp": "",
					"items": []interface{}{
						map[string]interface{}{
							"name":  "item1",
							"empty": "",
							"nested": map[string]interface{}{
								"validNested": "keep",
								"emptyNested": nil,
							},
						},
						map[string]interface{}{
							"empty": "",
							"null":  nil,
						},
					},
				},
			},
			expected: map[string]interface{}{
				"name": "resource",
				"properties": map[string]interface{}{
					"validProp": "value",
					"items": []interface{}{
						map[string]interface{}{
							"name": "item1",
							"nested": map[string]interface{}{
								"validNested": "keep",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deepCopy(tt.input)
			removeEmptyValues(result)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("removeEmptyValues() = %v, want %v", result, tt.expected)
			}
		})
	}
}
