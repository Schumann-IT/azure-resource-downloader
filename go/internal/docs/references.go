package docs

import "sort"

// ReferenceEntry is one resolved reference: a GUID the export knows about — an
// assignment target group, an assignment filter or a notification message
// template — with the facts a renderer needs to name it and link its document.
type ReferenceEntry struct {
	ID string
	// Name is the object's display name, the bare ID when the entry records no
	// name, and empty when the object is not in the export (dangling).
	Name string
	// Kind is the rendered group kind (e.g. "dynamic security group"); empty
	// for filters and templates, which have no kind concept.
	Kind string
	// DocPath is the object's tenant-relative document path; empty when the
	// object is not in the export.
	DocPath string
	// Present reports whether the export holds the object. A referenced GUID
	// with no entry is dangling: usually deleted from the tenant while still
	// referenced.
	Present bool
	// ExtraOnly reports a group referenced only by the caller-supplied extra
	// IDs (e.g. an observed drift payload) with no entry in the export. It is
	// not dangling — the baseline never referenced it — the object is simply
	// newer than the export.
	ExtraOnly bool
}

// ReferenceIndex is the exported facade over the reference-resolution indexes
// the docs engines build from resources/metadata.yaml. Consumers outside this
// package (the drift analysis prompt in internal/drift) render GUID → name /
// kind / document maps through it, so they can never re-derive the same facts
// differently from the engines that hash them.
type ReferenceIndex struct {
	groups    []ReferenceEntry
	filters   []ReferenceEntry
	templates []ReferenceEntry
	// referenced is the group IDs backing InScope: the baseline's
	// assignment-referenced set, unioned with any caller-supplied extras.
	referenced map[string]bool
}

// NewReferenceIndex builds the index from a loaded export metadata. All three
// lists are sorted by ID and include dangling references (Present false), so
// rendering them is deterministic and never hides a broken reference.
func NewReferenceIndex(m *Metadata) *ReferenceIndex {
	return NewReferenceIndexWithExtraGroups(m, nil)
}

// NewReferenceIndexWithExtraGroups builds the index with additional referenced
// group IDs beyond the baseline's assignment targets — the drift analysis
// passes group IDs harvested from observed payloads, so a group a drifted
// policy newly assigns counts as referenced. With no extras it is exactly
// NewReferenceIndex.
func NewReferenceIndexWithExtraGroups(m *Metadata, extraGroupIDs []string) *ReferenceIndex {
	baselineRef, _ := referencedGroups(m)
	referenced := make(map[string]bool, len(baselineRef)+len(extraGroupIDs))
	for id := range baselineRef {
		referenced[id] = true
	}
	for _, id := range extraGroupIDs {
		if id != "" {
			referenced[id] = true
		}
	}
	return &ReferenceIndex{
		groups:     referencedGroupEntries(m, referenced, baselineRef),
		filters:    filterEntries(m),
		templates:  templateEntries(m),
		referenced: referenced,
	}
}

// InScope reports whether a resource of the given type and ID falls inside the
// documentation scope shared by the docs engines and the drift analysis:
// autopilot identities never, groups only when referenced by an assignment
// (baseline or extras), everything else always.
func (ri *ReferenceIndex) InScope(rtype, resourceID string) bool {
	return scopeDecision(rtype, resourceID, ri.referenced)
}

// Groups returns the assignment target groups referenced by any assignment in
// the export. The returned slice is shared; callers must not modify it.
func (ri *ReferenceIndex) Groups() []ReferenceEntry { return ri.groups }

// Filters returns the assignment filters in the export plus any dangling filter
// references. The returned slice is shared; callers must not modify it.
func (ri *ReferenceIndex) Filters() []ReferenceEntry { return ri.filters }

// Templates returns the notification message templates in the export plus any
// dangling template references. The returned slice is shared; callers must not
// modify it.
func (ri *ReferenceIndex) Templates() []ReferenceEntry { return ri.templates }

// referencedGroupEntries resolves every group GUID in the referenced set, in
// the same way renderRefmap does for the documentation prompt. An unresolved
// GUID the baseline referenced is dangling; one only the extras referenced is
// marked ExtraOnly instead — the export never promised it.
func referencedGroupEntries(m *Metadata, referenced, baselineRef map[string]bool) []ReferenceEntry {
	infos := buildGroupInfo(m)
	keyByID := groupKeyByID(m)

	entries := make([]ReferenceEntry, 0, len(referenced))
	for _, id := range sortedKeys(referenced) {
		e := ReferenceEntry{ID: id}
		if key, ok := keyByID[id]; ok {
			gi := infos[id]
			e.Name = nameOrID(gi.name, id)
			e.Kind = groupKindLabel(gi)
			e.DocPath = docRel(key)
			e.Present = true
		} else if !baselineRef[id] {
			e.ExtraOnly = true
		}
		entries = append(entries, e)
	}
	return entries
}

// filterEntries lists every assignment filter in the export, plus the dangling
// filter GUIDs referenced by an assignment but absent from it.
func filterEntries(m *Metadata) []ReferenceEntry {
	keyByID := map[string]string{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == assignmentFiltersType && entry.ResourceId != "" {
			keyByID[entry.ResourceId] = key
		}
	}

	entries := make([]ReferenceEntry, 0, len(keyByID))
	for _, id := range sortedStringMapKeys(keyByID) {
		key := keyByID[id]
		entries = append(entries, ReferenceEntry{
			ID:      id,
			Name:    nameOrID(m.Resources[key].DisplayName, id),
			DocPath: docRel(key),
			Present: true,
		})
	}
	for _, id := range danglingFilterIDs(m, buildFilterInfo(m)) {
		entries = append(entries, ReferenceEntry{ID: id})
	}
	return entries
}

// templateEntries lists every notification message template in the export, plus
// the dangling template GUIDs referenced by a noncompliance action but absent
// from it.
func templateEntries(m *Metadata) []ReferenceEntry {
	keyByID := templateKeyByID(m)

	entries := make([]ReferenceEntry, 0, len(keyByID))
	for _, id := range sortedStringMapKeys(keyByID) {
		key := keyByID[id]
		entries = append(entries, ReferenceEntry{
			ID:      id,
			Name:    nameOrID(m.Resources[key].DisplayName, id),
			DocPath: docRel(key),
			Present: true,
		})
	}
	for _, id := range danglingTemplateIDs(m) {
		entries = append(entries, ReferenceEntry{ID: id})
	}
	return entries
}

// nameOrID falls back to the ID when an entry records no display name.
func nameOrID(name, id string) string {
	if name != "" {
		return name
	}
	return id
}

// sortedStringMapKeys returns the keys of a string-valued map, sorted.
func sortedStringMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
