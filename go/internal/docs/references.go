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
}

// NewReferenceIndex builds the index from a loaded export metadata. All three
// lists are sorted by ID and include dangling references (Present false), so
// rendering them is deterministic and never hides a broken reference.
func NewReferenceIndex(m *Metadata) *ReferenceIndex {
	return &ReferenceIndex{
		groups:    referencedGroupEntries(m),
		filters:   filterEntries(m),
		templates: templateEntries(m),
	}
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

// referencedGroupEntries resolves every group GUID referenced by an assignment
// target, in the same way renderRefmap does for the documentation prompt.
func referencedGroupEntries(m *Metadata) []ReferenceEntry {
	referenced, _ := referencedGroups(m)
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
