package drift

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/models"
)

// renderAnalyzeObservation describes the observation under analysis: the trees
// involved, the baseline it was decided against, its completeness and the
// verdict counts — everything the report's frontmatter and caveats section are
// copied from.
func renderAnalyzeObservation(tenantDir string, obs *Observation) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Tenant folder: `%s`\n", tenantDir)
	fmt.Fprintf(&b, "- Baseline (read-only): `%s/`\n", path.Join(tenantDir, models.ResourcesDirName))
	fmt.Fprintf(&b, "- Observed payloads (read-only): `%s/`\n", path.Join(tenantDir, DriftDirName))
	fmt.Fprintf(&b, "- Your drift documents (one per finding; each worklist entry names its destination): `%s/<APIType>/<endpoint>/<name>.md`\n", path.Join(tenantDir, DriftDirName))
	fmt.Fprintf(&b, "- Your index (the summary, written last): `%s`\n", path.Join(tenantDir, DriftDirName, IndexFileName))
	fmt.Fprintf(&b, "- Observed at: `%s`\n", obs.ObservedAt)
	fmt.Fprintf(&b, "- Baseline generated at: `%s`\n", obs.Baseline.GeneratedAt)
	fmt.Fprintf(&b, "- Drift run complete: %s\n", completeCell(obs.Run.Complete, obs.Run.IncompleteReason))
	c := obs.Counts
	fmt.Fprintf(&b, "- Verdicts: changed %d · added %d · removed %d · renamed %d (unchanged %d of %d compared)\n",
		c.Changed, c.Added, c.Removed, c.Renamed, c.Unchanged, c.Compared)
	fmt.Fprintf(&b, "- Types whose drift is unknown (listing failed): %s\n", listOrNone(obs.UnknownTypes))
	fmt.Fprintf(&b, "- Removals suppressed (incomplete run could not assert absence): %v\n", obs.RemovalsSuppressed)
	b.WriteString(renderNotComparable(obs.NotComparable))
	return strings.TrimRight(b.String(), "\n")
}

// completeCell renders a completeness flag with its reason.
func completeCell(complete bool, reason string) string {
	if complete {
		return "`true`"
	}
	if reason == "" {
		reason = "no reason recorded"
	}
	return fmt.Sprintf("`false — %s`", reason)
}

// listOrNone renders a string list inline, or "none".
func listOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	quoted := make([]string, 0, len(items))
	for _, it := range items {
		quoted = append(quoted, "`"+it+"`")
	}
	return strings.Join(quoted, ", ")
}

// renderNotComparable lists the baseline entries whose drift could not be
// decided (config attestation), so the report's "Not analyzed" section can name
// them instead of implying they did not drift.
func renderNotComparable(entries []NotComparableEntry) string {
	if len(entries) == 0 {
		return "- Entries not comparable (config attestation): none\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- Entries not comparable (config attestation): %d\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(&b, "  - `%s` — %s\n", e.Key, e.Reason)
	}
	return b.String()
}

// renderAnalyzeWorklist renders the findings grouped by type, each with the
// files to read and the recorded field deltas. Output is deterministic: types
// and findings are sorted by key.
func renderAnalyzeWorklist(obs *Observation, specPresent map[string]bool) string {
	byType := map[string][]string{}
	for key := range obs.Findings {
		rtype := typeOfKey(key)
		byType[rtype] = append(byType[rtype], key)
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)

	var b strings.Builder
	total := 0
	for _, t := range types {
		b.WriteString(specHeading(t, specPresent[t]))
		keys := byType[t]
		sort.Strings(keys)
		for _, key := range keys {
			f := obs.Findings[key]
			b.WriteString(renderFinding(key, f))
			total++
		}
	}
	fmt.Fprintf(&b, "_%d finding(s) across %d type(s)._", total, len(types))
	return b.String()
}

// specHeading renders a type's section heading with its spec path, or the
// missing-spec caveat when the export carries none.
func specHeading(rtype string, present bool) string {
	if present {
		return fmt.Sprintf("### %s — spec: `%s/%s/doc-prompt.md`\n\n", rtype, models.ResourcesDirName, rtype)
	}
	return fmt.Sprintf("### %s — ⚠️ spec missing (no `doc-prompt.md` in the export; was the download run with --no-prompt?) — analyze from the YAML alone and flag the reduced confidence\n\n", rtype)
}

// renderFinding renders one finding: verdict, names, the three files that
// matter for it, and its deltas.
func renderFinding(key string, f Finding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "#### %s: %s\n\n", f.Verdict, findingTitle(key, f))
	if f.ResourceID != "" {
		fmt.Fprintf(&b, "- Resource id: `%s`\n", f.ResourceID)
	}
	if f.Verdict == VerdictRenamed {
		fmt.Fprintf(&b, "- Renamed from: `%s`\n", f.PreviousDisplayName)
	}
	b.WriteString(findingPaths(key, f))
	b.WriteString(findingDeltas(f))
	b.WriteString("\n")
	return b.String()
}

// findingTitle names a finding by display name, falling back to its key.
func findingTitle(key string, f Finding) string {
	if f.DisplayName != "" {
		return f.DisplayName
	}
	return key
}

// findingPaths renders the baseline, payload and document lines a finding's
// verdict makes applicable, and the drift-document destination the agent
// writes for it — derived from the same key, so the four files always join.
func findingPaths(key string, f Finding) string {
	var b strings.Builder
	if f.BaselineKey != "" {
		fmt.Fprintf(&b, "- Baseline (old bytes): `%s/%s`\n", models.ResourcesDirName, f.BaselineKey)
	} else {
		b.WriteString("- Baseline (old bytes): none — the resource is new\n")
	}
	if f.Verdict == VerdictRemoved {
		b.WriteString("- Current (new bytes): none — the resource is gone from the tenant\n")
	} else {
		fmt.Fprintf(&b, "- Current (new bytes): `%s/%s`\n", DriftDirName, key)
	}
	switch {
	case f.Verdict == VerdictAdded:
		fmt.Fprintf(&b, "- Document: none yet (`%s` after the next documentation pass)\n", f.DocPath)
	case f.DocPath != "":
		fmt.Fprintf(&b, "- Document (describes the pre-change state): `%s`\n", f.DocPath)
	}
	fmt.Fprintf(&b, "- Your drift document (write it): `%s/%s`\n", DriftDirName, ReportPathForKey(key))
	return b.String()
}

// findingDeltas renders a finding's recorded field deltas as a list — never a
// table, since delta values may contain any character.
func findingDeltas(f Finding) string {
	if f.Verdict == VerdictAdded || f.Verdict == VerdictRemoved {
		return ""
	}
	if f.DeltaNote != "" {
		return fmt.Sprintf("- Field deltas: not recorded (%s) — diff the baseline and payload files yourself\n", f.DeltaNote)
	}
	if len(f.Deltas) == 0 {
		return "- Field deltas: none recorded — diff the baseline and payload files yourself\n"
	}
	var b strings.Builder
	b.WriteString("- Field deltas (values truncated; open both files when context is needed):\n")
	for _, d := range f.Deltas {
		fmt.Fprintf(&b, "  - `%s`: `%s` → `%s`\n", d.Path, d.Old, d.New)
	}
	return b.String()
}

// renderAnalyzeRefmap renders the GUID → name/document maps for assignment
// target groups, assignment filters and notification message templates, so
// "who is affected" resolves from the same facts the docs engines hash.
func renderAnalyzeRefmap(ri *docs.ReferenceIndex) string {
	var b strings.Builder
	b.WriteString("**Assignment target groups (referenced by an assignment in the baseline):**\n\n")
	b.WriteString(refEntries(ri.Groups()))
	b.WriteString("\n**Assignment filters:**\n\n")
	b.WriteString(refEntries(ri.Filters()))
	b.WriteString("\n**Notification message templates:**\n\n")
	b.WriteString(refEntries(ri.Templates()))
	return strings.TrimRight(b.String(), "\n")
}

// refEntries renders one reference list, flagging dangling references.
func refEntries(entries []docs.ReferenceEntry) string {
	if len(entries) == 0 {
		return "_None._\n"
	}
	var b strings.Builder
	for _, e := range entries {
		if !e.Present {
			fmt.Fprintf(&b, "- `%s` → ⚠️ not in export (dangling)\n", e.ID)
			continue
		}
		if e.Kind != "" {
			fmt.Fprintf(&b, "- `%s` → [%s](%s) · %s\n", e.ID, e.Name, e.DocPath, e.Kind)
			continue
		}
		fmt.Fprintf(&b, "- `%s` → [%s](%s)\n", e.ID, e.Name, e.DocPath)
	}
	return b.String()
}
