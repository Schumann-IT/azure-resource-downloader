package docs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// requiredMarkers are the marked blocks GeneratePrompt fills. Every one must be
// present, matched and non-nested in the template before anything is written.
var requiredMarkers = []string{"export", "worklist", "refmap", "resplice", "migrate"}

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
			fmt.Fprintf(&b, "| `%s` | `%s` | %s | `%s` | `%s` | |\n",
				r.SourcePath, r.DocPath, r.Reason, r.SourceSha256, r.PromptSha256)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "_%d document(s) to write across %d type(s)._", len(items), len(types))
	return b.String()
}

// renderRefmap lists every referenced assignment target group as GUID → name
// and document path, flagging GUIDs with no group in the export as dangling.
func renderRefmap(m *Metadata, referenced map[string]bool) string {
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
		fmt.Fprintf(&b, "- `%s` → [%s](%s)\n", id, name, docRel(key))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderNotImplemented renders a marked block whose detection this build does
// not yet perform. It states so plainly rather than claiming "none", so the
// block is never silently trusted as empty.
func renderNotImplemented(what string) string {
	return fmt.Sprintf("_none detected — %s is not yet implemented in this build, so a group rename or a re-pointed assignment will not be reflected here until it is._", what)
}
