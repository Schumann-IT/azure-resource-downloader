package docs

import (
	"testing"
)

func boolPtr(b bool) *bool { return &b }

// groupTargetWithFilter builds a raw group assignment target carrying a filter.
func groupTargetWithFilter(groupID, filterID, filterType string) map[string]interface{} {
	return map[string]interface{}{
		"target": map[string]interface{}{
			"@odata.type": "#microsoft.graph.groupAssignmentTarget",
			"groupId":     groupID,
			"deviceAndAppManagementAssignmentFilterId":   filterID,
			"deviceAndAppManagementAssignmentFilterType": filterType,
		},
	}
}

// exclusionTarget builds a raw exclusion group assignment target.
func exclusionTarget(groupID string) map[string]interface{} {
	return map[string]interface{}{
		"target": map[string]interface{}{
			"@odata.type": "#microsoft.graph.exclusionGroupAssignmentTarget",
			"groupId":     groupID,
		},
	}
}

func TestParseAssignments(t *testing.T) {
	targets := []interface{}{
		groupTarget("G1"),
		exclusionTarget("G2"),
		groupTargetWithFilter("G3", noFilterSentinel, "none"),
		groupTargetWithFilter("G4", "F1", "include"),
		"not a map",
		map[string]interface{}{"no": "target"},
	}
	rows := parseAssignments(targets)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].direction != "Include" || rows[0].groupID != "G1" {
		t.Errorf("row0 = %+v", rows[0])
	}
	if rows[1].direction != "Exclude" || rows[1].groupID != "G2" {
		t.Errorf("row1 (exclusion) = %+v", rows[1])
	}
	// The sentinel filter id is normalised to empty so it never hashes as a
	// filter nor reads as dangling.
	if rows[2].filterID != "" {
		t.Errorf("sentinel filter id must be normalised to empty, got %q", rows[2].filterID)
	}
	if rows[3].filterID != "F1" || rows[3].filterType != "include" {
		t.Errorf("row3 filter = %+v", rows[3])
	}
}

func TestAssignmentsSha256DetectsChanges(t *testing.T) {
	groups := map[string]groupInfo{
		"G1": {name: "Group One", groupTypes: []string{"DynamicMembership"}, securityEnabled: boolPtr(true), present: true},
	}
	filters := map[string]filterInfo{
		"F1": {name: "Filter One", present: true},
	}
	rows := parseAssignments([]interface{}{groupTargetWithFilter("G1", "F1", "include")})

	base := assignmentsSha256(rows, groups, filters)
	if base == "" {
		t.Fatal("hash must be non-empty")
	}

	// Order independence: a second target list in a different order hashes the
	// same once both rows are present.
	twoA := parseAssignments([]interface{}{groupTarget("Ga"), groupTarget("Gb")})
	twoB := parseAssignments([]interface{}{groupTarget("Gb"), groupTarget("Ga")})
	if assignmentsSha256(twoA, groups, filters) != assignmentsSha256(twoB, groups, filters) {
		t.Error("hash must be independent of assignment order")
	}

	// A group rename changes the hash even though the resource is identical.
	renamed := map[string]groupInfo{
		"G1": {name: "Group One RENAMED", groupTypes: []string{"DynamicMembership"}, securityEnabled: boolPtr(true), present: true},
	}
	if assignmentsSha256(rows, renamed, filters) == base {
		t.Error("a group rename must change the assignments hash")
	}

	// A filter rename changes the hash.
	renamedFilter := map[string]filterInfo{"F1": {name: "Filter RENAMED", present: true}}
	if assignmentsSha256(rows, groups, renamedFilter) == base {
		t.Error("a filter rename must change the assignments hash")
	}

	// A group leaving the export (dangling) changes the hash.
	if assignmentsSha256(rows, map[string]groupInfo{}, filters) == base {
		t.Error("a group leaving the export must change the assignments hash")
	}

	// A group-kind change (dynamic -> assigned) changes the hash.
	kindChanged := map[string]groupInfo{
		"G1": {name: "Group One", groupTypes: nil, securityEnabled: boolPtr(true), present: true},
	}
	if assignmentsSha256(rows, kindChanged, filters) == base {
		t.Error("a group-kind change must change the assignments hash")
	}

	// An empty assignment set hashes to a stable, non-empty value.
	empty := assignmentsSha256(nil, groups, filters)
	if empty == "" || empty == base {
		t.Errorf("empty assignments must have a stable distinct hash, got %q", empty)
	}
	if empty != assignmentsSha256([]assignmentRow{}, groups, filters) {
		t.Error("empty assignments hash must be stable")
	}
}

func TestTargetedBySha256DetectsChanges(t *testing.T) {
	filters := map[string]filterInfo{}
	rows := []reverseRow{
		{resourceType: compType, sourceKey: compType + "/a.yaml", resourceName: "Alpha", direction: "Include"},
	}
	base := targetedBySha256(rows, filters)

	// Adding a targeting resource changes the hash.
	more := append([]reverseRow{}, rows...)
	more = append(more, reverseRow{resourceType: compType, sourceKey: compType + "/b.yaml", resourceName: "Beta", direction: "Include"})
	if targetedBySha256(more, filters) == base {
		t.Error("adding a targeting resource must change the reverse hash")
	}

	// Renaming a targeting resource changes the hash.
	renamed := []reverseRow{{resourceType: compType, sourceKey: compType + "/a.yaml", resourceName: "Alpha RENAMED", direction: "Include"}}
	if targetedBySha256(renamed, filters) == base {
		t.Error("renaming a targeting resource must change the reverse hash")
	}

	// Order independence.
	orderA := targetedBySha256(more, filters)
	swapped := []reverseRow{more[1], more[0]}
	if targetedBySha256(swapped, filters) != orderA {
		t.Error("reverse hash must be independent of row order")
	}
}

func TestGroupKindLabel(t *testing.T) {
	cases := []struct {
		name string
		gi   groupInfo
		want string
	}{
		{"dynamic security", groupInfo{groupTypes: []string{"DynamicMembership"}, securityEnabled: boolPtr(true)}, "dynamic security group"},
		{"assigned security", groupInfo{securityEnabled: boolPtr(true)}, "assigned security group"},
		{"dynamic m365", groupInfo{groupTypes: []string{"DynamicMembership", "Unified"}}, "dynamic Microsoft 365 group"},
		{"assigned m365", groupInfo{groupTypes: []string{"Unified"}}, "assigned Microsoft 365 group"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := groupKindLabel(c.gi); got != c.want {
				t.Errorf("groupKindLabel = %q, want %q", got, c.want)
			}
		})
	}
}
