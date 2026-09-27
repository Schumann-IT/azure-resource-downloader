package docs

import "testing"

// referencesMeta builds a metadata with one policy assigning G1, groups G1
// (referenced) and G2 (unreferenced), and one autopilot identity.
func referencesMeta() *Metadata {
	return &Metadata{
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId:        "pol",
				PresentInTenant:   true,
				AssignmentTargets: []interface{}{groupTarget("G1")},
			},
			groupsType + "/g1.yaml":              {ResourceId: "G1", DisplayName: "Group One", PresentInTenant: true},
			groupsType + "/g2.yaml":              {ResourceId: "G2", DisplayName: "Unreferenced", PresentInTenant: true},
			autopilotIdentitiesType + "/d1.yaml": {ResourceId: "D1", DisplayName: "Device", PresentInTenant: true},
		},
	}
}

func TestReferenceIndexInScope(t *testing.T) {
	ri := NewReferenceIndex(referencesMeta())

	tests := []struct {
		name        string
		rtype, id   string
		wantInScope bool
	}{
		{"ordinary type always", compType, "pol", true},
		{"autopilot never", autopilotIdentitiesType, "D1", false},
		{"referenced group", groupsType, "G1", true},
		{"unreferenced group", groupsType, "G2", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ri.InScope(tc.rtype, tc.id); got != tc.wantInScope {
				t.Errorf("InScope(%s, %s) = %v, want %v", tc.rtype, tc.id, got, tc.wantInScope)
			}
		})
	}

	// The facade must decide exactly like the docs engines' inScope.
	m := referencesMeta()
	referenced, _ := referencedGroups(m)
	for key, entry := range m.Resources {
		rtype := typeOfKey(key)
		if want := inScope(rtype, entry, referenced); ri.InScope(rtype, entry.ResourceId) != want {
			t.Errorf("InScope(%s, %s) diverges from the docs engines' inScope (%v)", rtype, entry.ResourceId, want)
		}
	}
}

func TestReferenceIndexWithExtraGroups(t *testing.T) {
	ri := NewReferenceIndexWithExtraGroups(referencesMeta(), []string{"G2", "G-new", ""})

	// An extra ID promotes an unreferenced baseline group into scope.
	if !ri.InScope(groupsType, "G2") {
		t.Error("a group referenced via extras must be in scope")
	}
	// An extra ID with no baseline entry is in scope but marked ExtraOnly.
	if !ri.InScope(groupsType, "G-new") {
		t.Error("an extra-only group must be in scope")
	}

	byID := map[string]ReferenceEntry{}
	for _, e := range ri.Groups() {
		byID[e.ID] = e
	}
	if e := byID["G2"]; !e.Present || e.ExtraOnly {
		t.Errorf("G2 must resolve to its baseline entry, got %+v", e)
	}
	if e := byID["G-new"]; e.Present || !e.ExtraOnly {
		t.Errorf("G-new must be marked ExtraOnly (not dangling), got %+v", e)
	}
	if _, ok := byID[""]; ok {
		t.Error("empty extra IDs must be dropped")
	}
	// Autopilot stays excluded no matter what.
	if ri.InScope(autopilotIdentitiesType, "D1") {
		t.Error("autopilot identities are never in scope")
	}
}
