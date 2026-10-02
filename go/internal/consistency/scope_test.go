package consistency

import (
	"testing"

	"azure-resource-downloader/internal/docs"
)

func TestResourcePlatforms(t *testing.T) {
	tests := []struct {
		name  string
		entry docs.ResourceMeta
		want  string
	}{
		{"metadata platforms", docs.ResourceMeta{Platforms: "macOS"}, "macos"},
		{"windows10 platform", docs.ResourceMeta{Platforms: "windows10"}, "windows"},
		{"odata prefix", docs.ResourceMeta{ODataType: "#microsoft.graph.iosCompliancePolicy"}, "ios"},
		{"android prefix", docs.ResourceMeta{ODataType: "#microsoft.graph.androidCompliancePolicy"}, "android"},
		{"unknown", docs.ResourceMeta{}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resourcePlatforms(tt.entry)
			if len(got) != 1 || got[0] != tt.want {
				t.Errorf("want [%s], got %v", tt.want, got)
			}
		})
	}
}

func TestGroupKind(t *testing.T) {
	equal(t, kindDevice, groupKind(`(device.deviceOSType -eq "MacMDM")`))
	equal(t, kindUser, groupKind(`(user.department -eq "IT")`))
	equal(t, kindUnknown, groupKind(""))
}

// TestOverlapTruthTable pins the overlap verdict per resource pair.
func TestOverlapTruthTable(t *testing.T) {
	kinds := map[string]string{
		grpDevices: kindDevice,
		grpUsers:   kindUser,
		grpAdmins:  kindUnknown,
		grpOther:   kindUnknown,
	}
	scope := func(platform string, ts ...interface{}) *Scope {
		return buildScope(docs.ResourceMeta{Platforms: platform, AssignmentTargets: ts}, kinds, nil)
	}

	tests := []struct {
		name string
		a, b *Scope
		want string
	}{
		{"different known platforms", scope("macOS", allDevices()), scope("windows10", allDevices()), OverlapNone},
		{"one side has no include", scope("macOS", exclude(grpOther)), scope("macOS", allDevices()), OverlapNone},
		{"no assignments at all", scope("macOS"), scope("macOS", allDevices()), OverlapNone},
		{"all devices on both", scope("macOS", allDevices()), scope("macOS", allDevices()), OverlapCertain},
		{"all users on both", scope("macOS", allUsers()), scope("macOS", allUsers()), OverlapCertain},
		{"same group unfiltered", scope("macOS", include(grpOther)), scope("macOS", include(grpOther)), OverlapCertain},
		{"same group excluded on one side", scope("macOS", include(grpOther), exclude(grpOther)), scope("macOS", include(grpOther)), OverlapNone},
		{"same group but unknown platform", scope("", include(grpOther)), scope("", include(grpOther)), OverlapPossible},
		{"different groups", scope("macOS", include(grpOther)), scope("macOS", include(grpAdmins)), OverlapPossible},
		{
			"include all sits in the other's exclusion (complement pair)",
			scope("macOS", include(grpAdmins)), scope("macOS", allUsers(), exclude(grpAdmins)), OverlapNone,
		},
		{
			"mixed-kind exclusion is ignored by Intune and stays possible",
			scope("macOS", include(grpDevices)), scope("macOS", allUsers(), exclude(grpDevices)), OverlapPossible,
		},
		{
			"user group excluded from a device include stays possible",
			scope("macOS", include(grpUsers)), scope("macOS", allDevices(), exclude(grpUsers)), OverlapPossible,
		},
		{
			"user-only vs device-only stays possible",
			scope("macOS", include(grpUsers)), scope("macOS", include(grpDevices)), OverlapPossible,
		},
		{
			"same filter include vs exclude",
			scope("macOS", includeFiltered(grpOther, fltMac, "include")),
			scope("macOS", includeFiltered(grpAdmins, fltMac, "exclude")), OverlapNone,
		},
		{
			"same filter, both include",
			scope("macOS", includeFiltered(grpOther, fltMac, "include")),
			scope("macOS", includeFiltered(grpOther, fltMac, "include")), OverlapPossible,
		},
		{
			"zero-GUID filter is no filter",
			scope("macOS", allDevices()),
			scope("macOS", includeFiltered(grpOther, zeroFilter, "none"), allDevices()), OverlapCertain,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := overlapOf(tt.a, tt.b).Verdict; got != tt.want {
				t.Errorf("a→b: want %s, got %s", tt.want, got)
			}
			if got := overlapOf(tt.b, tt.a).Verdict; got != tt.want {
				t.Errorf("b→a: want %s, got %s", tt.want, got)
			}
		})
	}
}

func TestOverlapRecordsKindsAndFilters(t *testing.T) {
	kinds := map[string]string{grpUsers: kindUser}
	a := buildScope(docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(includeFiltered(grpUsers, fltMac, "include"))}, kinds, nil)
	b := buildScope(docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(allDevices())}, kinds, nil)
	o := overlapOf(a, b)
	if len(o.KindsA) != 1 || o.KindsA[0] != kindUser || len(o.KindsB) != 1 || o.KindsB[0] != kindDevice {
		t.Errorf("kinds not recorded: %+v", o)
	}
	if len(o.FiltersA) != 1 || o.FiltersA[0] != fltMac+":include" || len(o.FiltersB) != 0 {
		t.Errorf("filters not recorded: %+v", o)
	}
}
