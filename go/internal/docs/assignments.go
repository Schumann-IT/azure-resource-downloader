package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Constants for resolving and hashing assignment blocks.
const (
	// assignmentFiltersType is the in-scope type whose entries resolve an
	// assignment target's filter id to a display name.
	assignmentFiltersType = "Microsoft.Graph/assignmentFilters"
	// noFilterSentinel is Intune's "no filter" filter id. It is not an
	// unresolvable reference and must never be treated as a dangling filter.
	noFilterSentinel = "00000000-0000-0000-0000-000000000000"
	// danglingName is the cell shown for a referenced group or filter that has
	// no matching entry in the export (deleted from the tenant while still
	// assigned).
	danglingName = "⚠️ not in export"

	// hashFieldSep and hashRecordSep separate fields and records in the
	// canonical form fed to the assignment hashes. They are control characters
	// that cannot occur in a display name or GUID, so no value can forge a
	// boundary and make two distinct blocks hash alike.
	hashFieldSep  = "\x1f"
	hashRecordSep = "\x1e"
)

// assignmentRow is the structured form of one raw assignment target, carrying
// exactly the facts the rendered assignments table (and its hash) depend on.
type assignmentRow struct {
	direction  string // Include or Exclude, from the target @odata.type
	targetKind string // the target @odata.type (distinguishes the built-in targets)
	groupID    string // group target id; empty for built-in targets
	filterID   string // assignment filter id; empty when there is no filter
	filterType string // include, exclude or none
	intent     string // apps only
	source     string
}

// parseAssignments extracts the structured assignment rows from a resource's
// raw assignment targets (metadata's ResourceMeta.AssignmentTargets). It is
// tolerant of missing or malformed fields: a bad entry contributes what it can
// and never panics.
func parseAssignments(targets []interface{}) []assignmentRow {
	var rows []assignmentRow
	for _, raw := range targets {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		target, ok := entry["target"].(map[string]interface{})
		if !ok {
			continue
		}
		kind := stringField(target, "@odata.type")
		row := assignmentRow{
			direction:  assignmentDirection(kind),
			targetKind: kind,
			groupID:    stringField(target, "groupId"),
			filterType: stringField(target, "deviceAndAppManagementAssignmentFilterType"),
			intent:     stringField(entry, "intent"),
			source:     stringField(entry, "source"),
		}
		// The "no filter" sentinel is normalised to empty so it neither hashes
		// as a filter nor is reported as a dangling reference.
		if fid := stringField(target, "deviceAndAppManagementAssignmentFilterId"); fid != "" && fid != noFilterSentinel {
			row.filterID = fid
		}
		rows = append(rows, row)
	}
	return rows
}

// assignmentDirection maps a target @odata.type to the Include/Exclude column.
func assignmentDirection(targetKind string) string {
	if strings.Contains(targetKind, "exclusionGroupAssignmentTarget") {
		return "Exclude"
	}
	return "Include"
}

func stringField(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// groupInfo carries a referenced group's resolved facts from its metadata
// entry, so both the reference map and the assignment hashes read one source.
type groupInfo struct {
	name            string
	groupTypes      []string
	securityEnabled *bool
	present         bool
}

// buildGroupInfo indexes every group entry by its resource id.
func buildGroupInfo(m *Metadata) map[string]groupInfo {
	out := map[string]groupInfo{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == groupsType && entry.ResourceId != "" {
			out[entry.ResourceId] = groupInfo{
				name:            entry.DisplayName,
				groupTypes:      entry.GroupTypes,
				securityEnabled: entry.SecurityEnabled,
				present:         true,
			}
		}
	}
	return out
}

// filterInfo carries an assignment filter's resolved facts.
type filterInfo struct {
	name    string
	present bool
}

// buildFilterInfo indexes every assignment-filter entry by its resource id.
func buildFilterInfo(m *Metadata) map[string]filterInfo {
	out := map[string]filterInfo{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == assignmentFiltersType && entry.ResourceId != "" {
			out[entry.ResourceId] = filterInfo{name: entry.DisplayName, present: true}
		}
	}
	return out
}

// resolveGroupName returns the display name a group id renders as: its name, a
// bare id when the entry has no name, the dangling marker when it is not in the
// export, or "" for a built-in target (no group id).
func resolveGroupName(id string, groups map[string]groupInfo) string {
	if id == "" {
		return ""
	}
	if gi, ok := groups[id]; ok {
		if gi.name != "" {
			return gi.name
		}
		return id
	}
	return danglingName
}

// resolveFilterName mirrors resolveGroupName for assignment filters.
func resolveFilterName(id string, filters map[string]filterInfo) string {
	if id == "" {
		return ""
	}
	if fi, ok := filters[id]; ok {
		if fi.name != "" {
			return fi.name
		}
		return id
	}
	return danglingName
}

// groupKindLabel renders a referenced group's kind as the inline annotation the
// tables use, e.g. "dynamic security group" or "assigned Microsoft 365 group".
func groupKindLabel(gi groupInfo) string {
	membership := "assigned"
	class := "security"
	for _, t := range gi.groupTypes {
		switch {
		case strings.EqualFold(t, "DynamicMembership"):
			membership = "dynamic"
		case strings.EqualFold(t, "Unified"):
			class = "Microsoft 365"
		}
	}
	return membership + " " + class + " group"
}

// assignmentsSha256 hashes the resolved inputs of a resource's assignments
// block. Anything that would change a rendered cell — a group or filter rename,
// a group-kind change, a target added or removed, a group or filter leaving the
// export — changes the hash, and nothing else does. Rows are canonicalised and
// sorted so the hash never depends on the order assignments arrived in.
//
// It hashes the resolved inputs, not the rendered markdown, so reformatting the
// table does not restale existing blocks. A resource with no assignments has a
// stable non-empty hash (the empty row set), so a freshly generated
// no-assignments block matches on the next run.
func assignmentsSha256(rows []assignmentRow, groups map[string]groupInfo, filters map[string]filterInfo) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		fields := []string{
			r.direction,
			r.targetKind,
			r.groupID,
			resolveGroupName(r.groupID, groups),
			groupTypesField(r.groupID, groups),
			securityEnabledField(r.groupID, groups),
			r.filterID,
			resolveFilterName(r.filterID, filters),
			r.filterType,
			r.intent,
			r.source,
		}
		lines = append(lines, strings.Join(fields, hashFieldSep))
	}
	sort.Strings(lines)
	return hashLines(lines)
}

// groupTypesField renders a group's groupTypes as a sorted, comma-joined string
// for hashing, or "" for a built-in target or unknown group.
func groupTypesField(id string, groups map[string]groupInfo) string {
	if id == "" {
		return ""
	}
	gi, ok := groups[id]
	if !ok {
		return ""
	}
	sorted := append([]string(nil), gi.groupTypes...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// securityEnabledField renders a group's securityEnabled tri-state (true/false/
// unknown) for hashing.
func securityEnabledField(id string, groups map[string]groupInfo) string {
	if id == "" {
		return ""
	}
	gi, ok := groups[id]
	if !ok || gi.securityEnabled == nil {
		return ""
	}
	if *gi.securityEnabled {
		return "true"
	}
	return "false"
}

// reverseRow is one entry in a group's "Targeted by" block: a resource that
// assigns the group. Its fields are exactly the rendered columns plus the link
// source, so the reverse hash restales when any of them changes.
type reverseRow struct {
	resourceType string
	sourceKey    string // metadata key, so the document path and link are derivable
	resourceName string
	direction    string
	filterID     string
	filterType   string
}

// buildTargetedBy indexes each group id to the resources that assign it, so a
// group's targetedBySha256 and its rendered "Targeted by" table come from one
// place and cannot disagree. Only resources present in the tenant are counted:
// a resource that has left the tenant no longer targets the group. Autopilot
// identities and groups themselves are never sources of assignments.
func buildTargetedBy(m *Metadata) map[string][]reverseRow {
	out := map[string][]reverseRow{}
	for _, key := range sortedResourceKeys(m.Resources) {
		rtype := typeOfKey(key)
		if rtype == autopilotIdentitiesType || rtype == groupsType {
			continue
		}
		entry := m.Resources[key]
		if !entry.PresentInTenant {
			continue
		}
		for _, r := range parseAssignments(entry.AssignmentTargets) {
			if r.groupID == "" {
				continue
			}
			out[r.groupID] = append(out[r.groupID], reverseRow{
				resourceType: rtype,
				sourceKey:    key,
				resourceName: entry.DisplayName,
				direction:    r.direction,
				filterID:     r.filterID,
				filterType:   r.filterType,
			})
		}
	}
	return out
}

// targetedBySha256 hashes the resolved inputs of a group's "Targeted by" block:
// the set of resources assigning it, with each one's rendered columns. A
// resource added, removed, renamed, or changing its filter or direction toward
// this group moves the hash, and nothing else does.
func targetedBySha256(rows []reverseRow, filters map[string]filterInfo) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		fields := []string{
			r.resourceType,
			r.sourceKey,
			r.resourceName,
			r.direction,
			r.filterID,
			resolveFilterName(r.filterID, filters),
			r.filterType,
		}
		lines = append(lines, strings.Join(fields, hashFieldSep))
	}
	sort.Strings(lines)
	return hashLines(lines)
}

// hashLines returns the hex SHA-256 of the record-separated canonical lines.
func hashLines(lines []string) string {
	sum := sha256.Sum256([]byte(strings.Join(lines, hashRecordSep)))
	return hex.EncodeToString(sum[:])
}

// danglingFilterIDs returns the sorted assignment filter ids referenced by a
// present resource's assignments that have no filter entry in the export — the
// filter equivalent of a dangling group. The "no filter" sentinel is already
// normalised away by parseAssignments, so it never appears here.
func danglingFilterIDs(m *Metadata, filters map[string]filterInfo) []string {
	seen := map[string]bool{}
	for key, entry := range m.Resources {
		rtype := typeOfKey(key)
		if rtype == autopilotIdentitiesType || !entry.PresentInTenant {
			continue
		}
		for _, r := range parseAssignments(entry.AssignmentTargets) {
			if r.filterID != "" && !filters[r.filterID].present {
				seen[r.filterID] = true
			}
		}
	}
	return sortedKeys(seen)
}
