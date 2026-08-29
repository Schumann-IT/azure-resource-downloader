package transform

import (
	"reflect"
	"testing"
)

func TestSortScalarSlices(t *testing.T) {
	data := map[string]interface{}{
		// Unordered scalar list (like Graph proxyAddresses) must be sorted.
		"proxyAddresses": []interface{}{"smtp:z@x.com", "SMTP:a@x.com", "smtp:m@x.com"},
		// Nested map with its own scalar list.
		"nested": map[string]interface{}{
			"groupTypes": []interface{}{"Unified", "DynamicMembership"},
		},
		// List of objects: order must be preserved (structured elements).
		"assignments": []interface{}{
			map[string]interface{}{"id": "b"},
			map[string]interface{}{"id": "a"},
		},
		// Mixed-type list: left untouched.
		"mixed": []interface{}{"b", 1, "a"},
		// Scalar list nested inside a list of objects.
		"settings": []interface{}{
			map[string]interface{}{"values": []interface{}{"y", "x"}},
		},
	}

	SortScalarSlices(data)

	if got := data["proxyAddresses"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"SMTP:a@x.com", "smtp:m@x.com", "smtp:z@x.com"}) {
		t.Errorf("proxyAddresses not sorted: %v", got)
	}
	if got := data["nested"].(map[string]interface{})["groupTypes"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"DynamicMembership", "Unified"}) {
		t.Errorf("nested groupTypes not sorted: %v", got)
	}
	if got := data["assignments"].([]interface{}); got[0].(map[string]interface{})["id"] != "b" {
		t.Errorf("list of objects must keep order, got %v", got)
	}
	if got := data["mixed"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"b", 1, "a"}) {
		t.Errorf("mixed-type list must be untouched, got %v", got)
	}
	if got := data["settings"].([]interface{})[0].(map[string]interface{})["values"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"x", "y"}) {
		t.Errorf("scalar list nested in object list not sorted: %v", got)
	}
}

func TestSortScalarSlicesEmpty(t *testing.T) {
	// Must not panic on empty/nil.
	SortScalarSlices(map[string]interface{}{})
	SortScalarSlices(nil)
}
