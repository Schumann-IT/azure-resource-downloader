package azure

import (
	"reflect"
	"testing"
)

func TestPermissionCoverage(t *testing.T) {
	tests := []struct {
		name        string
		declared    []string
		scopes      []string
		wantCovered []string
		wantMissing []string
	}{
		{
			name:        "exact match",
			declared:    []string{"Policy.Read.All"},
			scopes:      []string{"Policy.Read.All"},
			wantCovered: []string{"Policy.Read.All"},
		},
		{
			name:        "case-insensitive",
			declared:    []string{"Policy.Read.All"},
			scopes:      []string{"policy.read.all"},
			wantCovered: []string{"Policy.Read.All"},
		},
		{
			name:        "ReadWrite covers Read",
			declared:    []string{"DeviceManagementConfiguration.Read.All", "User.Read"},
			scopes:      []string{"DeviceManagementConfiguration.ReadWrite.All", "User.ReadWrite"},
			wantCovered: []string{"DeviceManagementConfiguration.Read.All", "User.Read"},
		},
		{
			name:        "Read does not cover ReadWrite",
			declared:    []string{"DeviceManagementConfiguration.ReadWrite.All"},
			scopes:      []string{"DeviceManagementConfiguration.Read.All"},
			wantMissing: []string{"DeviceManagementConfiguration.ReadWrite.All"},
		},
		{
			name:        "mixed, declared order kept, duplicates dropped",
			declared:    []string{"Policy.Read.All", "Group.Read.All", "policy.read.all"},
			scopes:      []string{"Group.Read.All"},
			wantCovered: []string{"Group.Read.All"},
			wantMissing: []string{"Policy.Read.All"},
		},
		{
			name:        "empty scope never covers a permission without a Read segment",
			declared:    []string{"Directory.AccessAsUser.All"},
			scopes:      []string{"", " "},
			wantMissing: []string{"Directory.AccessAsUser.All"},
		},
		{
			name:     "nothing declared",
			declared: nil,
			scopes:   []string{"User.Read"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			covered, missing := PermissionCoverage(tt.declared, tt.scopes)
			if !reflect.DeepEqual(covered, tt.wantCovered) {
				t.Errorf("covered = %v, want %v", covered, tt.wantCovered)
			}
			if !reflect.DeepEqual(missing, tt.wantMissing) {
				t.Errorf("missing = %v, want %v", missing, tt.wantMissing)
			}
		})
	}
}
