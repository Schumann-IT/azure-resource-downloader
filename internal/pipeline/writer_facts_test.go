package pipeline

import (
	"testing"

	"azure-resource-downloader/internal/models"
)

func TestStringSliceFromData(t *testing.T) {
	cases := []struct {
		name string
		data map[string]interface{}
		want []string
	}{
		{"absent", map[string]interface{}{}, nil},
		{"nil map", nil, nil},
		{"wrong type", map[string]interface{}{"groupTypes": "Unified"}, nil},
		{"strings", map[string]interface{}{"groupTypes": []interface{}{"Unified", "DynamicMembership"}}, []string{"Unified", "DynamicMembership"}},
		{"mixed skips non-strings", map[string]interface{}{"groupTypes": []interface{}{"Unified", 3, nil}}, []string{"Unified"}},
		{"all non-strings", map[string]interface{}{"groupTypes": []interface{}{1, 2}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stringSliceFromData(tc.data, "groupTypes")
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestBoolPtrFromData(t *testing.T) {
	if boolPtrFromData(nil, "securityEnabled") != nil {
		t.Error("nil map must yield nil")
	}
	if boolPtrFromData(map[string]interface{}{}, "securityEnabled") != nil {
		t.Error("absent key must yield nil")
	}
	if boolPtrFromData(map[string]interface{}{"securityEnabled": "true"}, "securityEnabled") != nil {
		t.Error("non-bool must yield nil")
	}
	got := boolPtrFromData(map[string]interface{}{"securityEnabled": false}, "securityEnabled")
	if got == nil || *got != false {
		t.Errorf("expected pointer to false, got %v", got)
	}
}

func TestBuildResourceFactsGroupFields(t *testing.T) {
	tr := &models.TransformResult{
		ResourceID:  "gid",
		DisplayName: "Group One",
		CleanedData: map[string]interface{}{
			"groupTypes":      []interface{}{"Unified", "DynamicMembership"},
			"securityEnabled": true,
		},
	}
	facts := buildResourceFacts(tr, []byte("displayName: Group One\n"))
	if len(facts.GroupTypes) != 2 {
		t.Fatalf("GroupTypes = %v", facts.GroupTypes)
	}
	if facts.SecurityEnabled == nil || !*facts.SecurityEnabled {
		t.Errorf("SecurityEnabled = %v, want pointer to true", facts.SecurityEnabled)
	}

	// A non-group resource records neither.
	plain := buildResourceFacts(&models.TransformResult{CleanedData: map[string]interface{}{"platforms": "windows"}}, []byte("x: 1\n"))
	if plain.GroupTypes != nil || plain.SecurityEnabled != nil {
		t.Errorf("non-group must not record group facts: %+v", plain)
	}
}
