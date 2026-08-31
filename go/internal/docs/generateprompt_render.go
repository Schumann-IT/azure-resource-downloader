package docs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// requiredMarkers are the marked blocks GeneratePrompt fills. Every one must be
// present, matched and non-nested in the template before anything is written.
var requiredMarkers = []string{"export", "worklist", "refmap", "resplice", "migrate", "summary-facts"}

// validateMarkers checks that each named block appears exactly once as a
// matched, correctly ordered start/end pair. It fails naming the offending
// marker so a broken --prompt template never splices into nothing.
func validateMarkers(template []byte, names []string) error {
	s := string(template)
	for _, name := range names {
		start := fmt.Sprintf("<!-- %s:start -->", name)
		end := fmt.Sprintf("<!-- %s:end -->", name)

		if n := strings.Count(s, start); n != 1 {
			return fmt.Errorf("template marker %q: expected exactly one %q, found %d", name, start, n)
		}
		if n := strings.Count(s, end); n != 1 {
			return fmt.Errorf("template marker %q: expected exactly one %q, found %d", name, end, n)
		}
		if strings.Index(s, start) > strings.Index(s, end) {
			return fmt.Errorf("template marker %q: start appears after end", name)
		}
	}
	return nil
}

// spliceMarker replaces the content between a block's start and end markers
// (the markers themselves stay), returning the rewritten template. The block is
// assumed validated by validateMarkers.
func spliceMarker(template []byte, name, content string) ([]byte, error) {
	s := string(template)
	start := fmt.Sprintf("<!-- %s:start -->", name)
	end := fmt.Sprintf("<!-- %s:end -->", name)

	i := strings.Index(s, start)
	j := strings.Index(s, end)
	if i < 0 || j < 0 || i > j {
		return nil, fmt.Errorf("template marker %q: not found or misordered", name)
	}
	after := i + len(start)

	var b strings.Builder
	b.WriteString(s[:after])
	b.WriteString("\n")
	b.WriteString(content)
	b.WriteString("\n")
	b.WriteString(s[j:])
	return []byte(b.String()), nil
}

// renderExport describes the export being documented.
func renderExport(tenantDir string, m *Metadata) string {
	complete := "`true`"
	if !m.Run.Complete {
		reason := m.Run.IncompleteReason
		if reason == "" {
			reason = "no reason recorded"
		}
		complete = fmt.Sprintf("`false — %s`", reason)
	}
	generatedAt := m.GeneratedAt
	if generatedAt == "" {
		generatedAt = "unknown"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "- Tenant folder: `%s`\n", tenantDir)
	fmt.Fprintf(&b, "- Resources (read-only source of truth): `%s`\n", filepath.Join(tenantDir, "resources"))
	fmt.Fprintf(&b, "- Documents (your output): `%s`\n", filepath.Join(tenantDir, DocsDirName))
	fmt.Fprintf(&b, "- Export generated at: `%s`\n", generatedAt)
	fmt.Fprintf(&b, "- Export complete: %s", complete)
	return b.String()
}

// renderWorklist groups the documents to generate by type and renders one
// section per type followed by a tally. Output is deterministic: types and rows
// are sorted.
func renderWorklist(items []WorkItem) string {
	if len(items) == 0 {
		return "_No documents need generating — every in-scope document is current._"
	}

	byType := map[string][]WorkItem{}
	for _, it := range items {
		byType[it.ResourceType] = append(byType[it.ResourceType], it)
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)

	var b strings.Builder
	for _, t := range types {
		rows := byType[t]
		sort.Slice(rows, func(i, j int) bool { return rows[i].SourcePath < rows[j].SourcePath })

		spec := fmt.Sprintf("resources/%s/%s", t, docPromptFileName)
		fmt.Fprintf(&b, "### %s — spec: `%s`\n\n", t, spec)
		b.WriteString("| Source | Document | Reason | sourceSha256 | promptSha256 | assignmentsSha256 |\n")
		b.WriteString("|---|---|---|---|---|---|\n")
		for _, r := range rows {
			fmt.Fprintf(&b, "| `%s` | `%s` | %s | `%s` | `%s` | %s |\n",
				r.SourcePath, r.DocPath, r.Reason, r.SourceSha256, r.PromptSha256, hashCell(r.AssignmentsSha256))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "_%d document(s) to write across %d type(s)._", len(items), len(types))
	return b.String()
}

// renderRefmap lists every referenced assignment target group as GUID → name,
// document path and kind (assigned/dynamic, security/Microsoft 365), so the
// agent renders the assignment tables from the same facts the forward hash was
// computed from. GUIDs with no group in the export are flagged dangling.
func renderRefmap(m *Metadata, referenced map[string]bool, groups map[string]groupInfo) string {
	if len(referenced) == 0 {
		return "_No assignment target groups are referenced in this export._"
	}

	keyByID := groupKeyByID(m)
	ids := make([]string, 0, len(referenced))
	for id := range referenced {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	for _, id := range ids {
		key, ok := keyByID[id]
		if !ok {
			fmt.Fprintf(&b, "- `%s` → ⚠️ not in export (dangling)\n", id)
			continue
		}
		entry := m.Resources[key]
		name := entry.DisplayName
		if name == "" {
			name = id
		}
		fmt.Fprintf(&b, "- `%s` → [%s](%s) · %s\n", id, name, docRel(key), groupKindLabel(groups[id]))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderResplice renders the two re-splice groups: documents whose own
// assignments block must be re-rendered (forward), and group documents whose
// "Targeted by" block must be re-rendered (reverse). Each row carries the new
// hash the agent must write into that document's frontmatter after splicing. It
// is "none" only when both sets are empty.
func renderResplice(forward, reverse []RespliceItem) string {
	if len(forward) == 0 && len(reverse) == 0 {
		return "_No documents need re-splicing — every current document's assignment and targeting blocks match the export._"
	}

	var b strings.Builder

	b.WriteString("**Forward — re-render the document's own assignments block (write the new `assignmentsSha256`):**\n\n")
	if len(forward) == 0 {
		b.WriteString("_None._\n")
	} else {
		b.WriteString("| Document | Type | Reason | assignmentsSha256 |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, it := range sortedResplice(forward) {
			fmt.Fprintf(&b, "| `%s` | %s | %s | `%s` |\n", it.DocPath, it.ResourceType, it.Reason, it.Hash)
		}
	}

	b.WriteString("\n**Reverse — re-render the group document's `Targeted by` block (write the new `targetedBySha256`):**\n\n")
	if len(reverse) == 0 {
		b.WriteString("_None._")
	} else {
		b.WriteString("| Document | Type | Reason | targetedBySha256 |\n")
		b.WriteString("|---|---|---|---|\n")
		for _, it := range sortedResplice(reverse) {
			fmt.Fprintf(&b, "| `%s` | %s | %s | `%s` |\n", it.DocPath, it.ResourceType, it.Reason, it.Hash)
		}
		return strings.TrimRight(b.String(), "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderMigrate lists documents predating the assignment markers that must have
// the markers inserted before their block can be spliced. It is "none" when
// there are none.
func renderMigrate(items []WorkItem) string {
	if len(items) == 0 {
		return "_No documents need migrating — every assignment-capable document already carries the markers._"
	}

	sorted := append([]WorkItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DocPath < sorted[j].DocPath })

	var b strings.Builder
	b.WriteString("| Document | Type | assignmentsSha256 |\n")
	b.WriteString("|---|---|---|\n")
	for _, it := range sorted {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", it.DocPath, it.ResourceType, hashCell(it.AssignmentsSha256))
	}
	return strings.TrimRight(b.String(), "\n")
}

// sortedResplice returns a copy of items sorted by document path for
// deterministic output.
func sortedResplice(items []RespliceItem) []RespliceItem {
	sorted := append([]RespliceItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].DocPath < sorted[j].DocPath })
	return sorted
}

// hashCell renders a hash as a backticked table cell, or an empty cell when the
// hash is absent (a type with no assignments concept).
func hashCell(hash string) string {
	if hash == "" {
		return ""
	}
	return "`" + hash + "`"
}

// renderSummaryFacts renders the tenant-wide facts the agent narrates into
// docs/summary.md: the export freshness, a per-type count of every resource
// present in the tenant (all types, groups and Autopilot included) with the
// platforms it covers and whether it has an assignments concept, the assignment
// posture across assignment-capable types, and the coverage caveats. It is
// computed from metadata.yaml, which is complete every run, so the summary is
// correct even when the work list was empty. Facts only — grouping the types
// into management areas is left to the agent's prose.
func renderSummaryFacts(m *Metadata, groups map[string]groupInfo) string {
	type typeAgg struct {
		count     int
		platforms map[string]bool
	}
	agg := map[string]*typeAgg{}

	// Assignment posture, across assignment-capable present resources.
	var assignable, assigned int
	var allUsers, allDevices, groupTargets, dynamicGroups, assignedGroups, danglingGroups int
	var gone int

	for _, key := range sortedResourceKeys(m.Resources) {
		entry := m.Resources[key]
		rtype := typeOfKey(key)

		if !entry.PresentInTenant {
			gone++
			continue
		}

		a := agg[rtype]
		if a == nil {
			a = &typeAgg{platforms: map[string]bool{}}
			agg[rtype] = a
		}
		a.count++
		for _, p := range splitCommaList(entry.Platforms) {
			a.platforms[p] = true
		}

		if !m.Types[rtype].HasAssignments {
			continue
		}
		assignable++
		rows := parseAssignments(entry.AssignmentTargets)
		if len(rows) > 0 {
			assigned++
		}
		for _, r := range rows {
			switch {
			case r.groupID != "":
				groupTargets++
				switch gi, ok := groups[r.groupID]; {
				case !ok || !gi.present:
					danglingGroups++
				case isDynamicGroup(gi):
					dynamicGroups++
				default:
					assignedGroups++
				}
			case strings.Contains(r.targetKind, allUsersTargetKind):
				allUsers++
			case strings.Contains(r.targetKind, allDevicesTargetKind):
				allDevices++
			}
		}
	}

	var b strings.Builder

	// Export freshness, repeated here so the block is a self-contained source
	// for the summary's frontmatter (generatedAt, exportComplete).
	generatedAt := m.GeneratedAt
	if generatedAt == "" {
		generatedAt = "unknown"
	}
	fmt.Fprintf(&b, "Export generated at: `%s`\n", generatedAt)
	if m.Run.Complete {
		b.WriteString("Export complete: `true`\n\n")
	} else {
		reason := m.Run.IncompleteReason
		if reason == "" {
			reason = "no reason recorded"
		}
		fmt.Fprintf(&b, "Export complete: `false — %s`\n\n", reason)
	}

	b.WriteString("Resources present in tenant, by type (all types, groups and Autopilot included):\n\n")
	b.WriteString("| Type | Count | Platforms | Has assignments |\n")
	b.WriteString("|---|---|---|---|\n")
	types := make([]string, 0, len(agg))
	for t := range agg {
		types = append(types, t)
	}
	sort.Strings(types)
	total := 0
	for _, t := range types {
		a := agg[t]
		total += a.count
		has := "no"
		if m.Types[t].HasAssignments {
			has = "yes"
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %s |\n", t, a.count, platformsCell(a.platforms), has)
	}
	fmt.Fprintf(&b, "\n_%d resource(s) present across %d type(s)._\n\n", total, len(types))

	b.WriteString("Assignment posture (across assignment-capable types):\n")
	fmt.Fprintf(&b, "- Assigned: %d of %d resources\n", assigned, assignable)
	fmt.Fprintf(&b, "- Configured but unassigned: %d\n", assignable-assigned)
	fmt.Fprintf(&b, "- Targets: All users ×%d · All devices ×%d · group targets ×%d (dynamic ×%d · assigned ×%d · dangling ×%d)\n\n",
		allUsers, allDevices, groupTargets, dynamicGroups, assignedGroups, danglingGroups)

	fmt.Fprintf(&b, "Retained but no longer in tenant: %d\n", gone)
	fmt.Fprintf(&b, "Types not listed (permissions): %s\n", listOrNone(m.NotListed.Types))
	fmt.Fprintf(&b, "Types that listed to zero: %s", listOrNone(m.NotListed.Empty))

	return b.String()
}

// splitCommaList splits a comma-separated metadata field (e.g. platforms) into
// its distinct, trimmed, non-empty values, in first-seen order.
func splitCommaList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// platformsCell renders a type's collected platforms as a sorted, comma-joined
// cell, or an em dash when none were recorded.
func platformsCell(set map[string]bool) string {
	if len(set) == 0 {
		return "—"
	}
	ps := make([]string, 0, len(set))
	for p := range set {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return strings.Join(ps, ", ")
}

// isDynamicGroup reports whether a group's membership is rule-based, so the
// posture can split group targets into dynamic and assigned.
func isDynamicGroup(gi groupInfo) bool {
	for _, t := range gi.groupTypes {
		if strings.EqualFold(t, "DynamicMembership") {
			return true
		}
	}
	return false
}

// listOrNone renders a slice as a sorted, comma-joined cell, or "none" when it
// is empty, for the coverage caveats.
func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}
