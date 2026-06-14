package models

import (
	"testing"
)

func TestParseResourceFilters(t *testing.T) {
	tests := []struct {
		name        string
		raw         map[string]interface{}
		wantFilters int
		wantProps   int
		wantErr     bool
	}{
		{
			name: "valid single filter",
			raw: map[string]interface{}{
				"Microsoft.Graph/deviceConfigurations": map[string]interface{}{
					"displayName": "GBL_.*",
				},
			},
			wantFilters: 1,
			wantProps:   1,
			wantErr:     false,
		},
		{
			name: "multiple properties on one type",
			raw: map[string]interface{}{
				"Microsoft.Graph/groups": map[string]interface{}{
					"displayName": "^IT-.*",
					"mailEnabled": "true",
				},
			},
			wantFilters: 1,
			wantProps:   2,
			wantErr:     false,
		},
		{
			name: "invalid regex is skipped with error",
			raw: map[string]interface{}{
				"Microsoft.Storage/storageAccounts": map[string]interface{}{
					"name": "[unterminated",
				},
			},
			wantFilters: 0,
			wantErr:     true,
		},
		{
			name: "non-string pattern is skipped with error",
			raw: map[string]interface{}{
				"Microsoft.Graph/groups": map[string]interface{}{
					"mailEnabled": true,
				},
			},
			wantFilters: 0,
			wantErr:     true,
		},
		{
			name: "non-map type entry is skipped with error",
			raw: map[string]interface{}{
				"Microsoft.Graph/groups": "displayName",
			},
			wantFilters: 0,
			wantErr:     true,
		},
		{
			name:        "empty config",
			raw:         map[string]interface{}{},
			wantFilters: 0,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := ParseResourceFilters(tt.raw)

			if tt.wantErr && err == nil {
				t.Fatalf("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(filters) != tt.wantFilters {
				t.Fatalf("expected %d filters, got %d", tt.wantFilters, len(filters))
			}
			if tt.wantProps > 0 && len(filters[0].Properties) != tt.wantProps {
				t.Fatalf("expected %d properties, got %d", tt.wantProps, len(filters[0].Properties))
			}
		})
	}
}

func TestGetResourceFilter(t *testing.T) {
	filters, err := ParseResourceFilters(map[string]interface{}{
		"Microsoft.Graph/deviceConfigurations": map[string]interface{}{
			"displayName": "GBL_.*",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := GetResourceFilter(filters, "microsoft.graph/deviceconfigurations"); got == nil {
		t.Fatalf("expected case-insensitive match, got nil")
	}
	if got := GetResourceFilter(filters, "Microsoft.Storage/storageAccounts"); got != nil {
		t.Fatalf("expected no match, got %+v", got)
	}
}

func TestResourceFilterMatches(t *testing.T) {
	tests := []struct {
		name       string
		raw        map[string]interface{}
		properties map[string]interface{}
		want       bool
	}{
		{
			name: "matching displayName",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"displayName": "GBL_.*"},
			},
			properties: map[string]interface{}{"displayName": "GBL_Baseline"},
			want:       true,
		},
		{
			name: "non-matching displayName",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"displayName": "GBL_.*"},
			},
			properties: map[string]interface{}{"displayName": "EU_Baseline"},
			want:       false,
		},
		{
			name: "missing property excludes",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"displayName": "GBL_.*"},
			},
			properties: map[string]interface{}{"name": "GBL_Baseline"},
			want:       false,
		},
		{
			name: "all properties must match",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"displayName": "^IT-", "mailEnabled": "true"},
			},
			properties: map[string]interface{}{"displayName": "IT-Admins", "mailEnabled": "false"},
			want:       false,
		},
		{
			name: "case-insensitive key lookup",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"DisplayName": "GBL_.*"},
			},
			properties: map[string]interface{}{"displayname": "GBL_Baseline"},
			want:       true,
		},
		{
			name: "nested dot path",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"properties.subnet.id": ".*/prod-subnet$"},
			},
			properties: map[string]interface{}{
				"properties": map[string]interface{}{
					"subnet": map[string]interface{}{"id": "/vnets/x/prod-subnet"},
				},
			},
			want: true,
		},
		{
			name: "non-string value coerced",
			raw: map[string]interface{}{
				"t": map[string]interface{}{"version": "^2$"},
			},
			properties: map[string]interface{}{"version": 2},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters, err := ParseResourceFilters(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			f := GetResourceFilter(filters, "t")
			if f == nil {
				t.Fatalf("expected a filter for type 't'")
			}
			if got := f.Matches(tt.properties); got != tt.want {
				t.Fatalf("Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ExampleResourceFilter_Matches demonstrates filtering a resource by displayName.
func ExampleResourceFilter_Matches() {
	filters, _ := ParseResourceFilters(map[string]interface{}{
		"Microsoft.Graph/deviceConfigurations": map[string]interface{}{
			"displayName": "GBL_.*",
		},
	})
	f := GetResourceFilter(filters, "Microsoft.Graph/deviceConfigurations")
	matched := f.Matches(map[string]interface{}{"displayName": "GBL_Baseline"})
	println(matched) // true
}
