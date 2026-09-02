package docs

import (
	"sort"
	"strings"
)

// notificationMessageTemplatesType is the in-scope type whose documents carry a
// reverse "Used by" block listing the resources that reference the template in a
// noncompliance action — the template analogue of a group's "Targeted by" block.
const notificationMessageTemplatesType = "Microsoft.Graph/notificationMessageTemplates"

// usedByRow is one entry in a notification template's "Used by" block: a
// resource that references the template through its noncompliance actions. Its
// fields are exactly the rendered columns plus the link source, so the used-by
// hash restales when any of them changes.
type usedByRow struct {
	resourceType string
	sourceKey    string // metadata key, so the document path and link are derivable
	resourceName string
}

// buildUsedByTemplate indexes each notification-template id to the resources
// that reference it, so a template's usedBySha256 and its rendered "Used by"
// table come from one place and cannot disagree. Only resources present in the
// tenant are counted: a resource that has left the tenant no longer references
// the template. Autopilot identities, groups and the templates themselves are
// never sources of references.
func buildUsedByTemplate(m *Metadata) map[string][]usedByRow {
	out := map[string][]usedByRow{}
	for _, key := range sortedResourceKeys(m.Resources) {
		rtype := typeOfKey(key)
		if rtype == autopilotIdentitiesType || rtype == groupsType || rtype == notificationMessageTemplatesType {
			continue
		}
		entry := m.Resources[key]
		if !entry.PresentInTenant {
			continue
		}
		for _, id := range entry.NotificationTemplateRefs {
			if id == "" {
				continue
			}
			out[id] = append(out[id], usedByRow{
				resourceType: rtype,
				sourceKey:    key,
				resourceName: entry.DisplayName,
			})
		}
	}
	return out
}

// usedBySha256 hashes the resolved inputs of a template's "Used by" block: the
// set of resources referencing it, with each one's rendered columns. A resource
// added, removed or renamed relative to this template moves the hash, and
// nothing else does. A template with no references has a stable non-empty hash
// (the empty row set), so a freshly generated "not referenced" block matches on
// the next run.
func usedBySha256(rows []usedByRow) string {
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, strings.Join([]string{r.resourceType, r.sourceKey, r.resourceName}, hashFieldSep))
	}
	sort.Strings(lines)
	return hashLines(lines)
}

// templateKeyByID indexes each notification template's resource id to its
// metadata key, so a used-by map can link to the template's document.
func templateKeyByID(m *Metadata) map[string]string {
	out := map[string]string{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == notificationMessageTemplatesType && entry.ResourceId != "" {
			out[entry.ResourceId] = key
		}
	}
	return out
}

// templateNames indexes each present notification template's resource id to its
// display name, so the forward hash and the reference map can resolve a
// referenced template's current name without re-opening its file. A template
// absent from the map is dangling (referenced but not in the export).
func templateNames(m *Metadata) map[string]string {
	out := map[string]string{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == notificationMessageTemplatesType && entry.ResourceId != "" && entry.PresentInTenant {
			out[entry.ResourceId] = entry.DisplayName
		}
	}
	return out
}

// notificationRefsSha256 hashes the resolved forward references a resource makes
// to notification templates: for each referenced id, its resolved display name
// and whether it is present in the export. Renaming a referenced template, or a
// referenced template appearing or disappearing, moves the hash — the forward
// counterpart of assignmentsSha256, so a policy's noncompliance-notification
// block re-splices when the template it names is renamed, without the policy's
// own YAML changing.
func notificationRefsSha256(names map[string]string, refs []string) string {
	lines := make([]string, 0, len(refs))
	for _, id := range refs {
		if id == "" {
			continue
		}
		name, present := names[id], "0"
		if _, ok := names[id]; ok {
			present = "1"
		}
		lines = append(lines, strings.Join([]string{id, name, present}, hashFieldSep))
	}
	sort.Strings(lines)
	return hashLines(lines)
}

// danglingTemplateIDs returns the sorted notification-template ids referenced by
// a present resource's noncompliance actions that have no template entry in the
// export — the template equivalent of a dangling group (usually deleted from the
// tenant while still referenced).
func danglingTemplateIDs(m *Metadata) []string {
	known := templateKeyByID(m)
	seen := map[string]bool{}
	for key, entry := range m.Resources {
		rtype := typeOfKey(key)
		if rtype == autopilotIdentitiesType || !entry.PresentInTenant {
			continue
		}
		for _, id := range entry.NotificationTemplateRefs {
			if id == "" {
				continue
			}
			if _, ok := known[id]; !ok {
				seen[id] = true
			}
		}
	}
	return sortedKeys(seen)
}

// sortedUsedBy returns a copy of rows sorted by resource type, then name, then
// source key for deterministic rendering and hashing inputs.
func sortedUsedBy(rows []usedByRow) []usedByRow {
	sorted := append([]usedByRow(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].resourceType != sorted[j].resourceType {
			return sorted[i].resourceType < sorted[j].resourceType
		}
		if sorted[i].resourceName != sorted[j].resourceName {
			return sorted[i].resourceName < sorted[j].resourceName
		}
		return sorted[i].sourceKey < sorted[j].sourceKey
	})
	return sorted
}
