package docs

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// IndexFileName is the machine-readable navigation index azure-rd emits at the
// docs/ tree root. The documentation frontend reads this single file to build a
// tenant's index, so no index.md is generated. It is the only file besides
// generate.md that azure-rd writes under docs/; both sit at the tree root where
// no document can be (documents are always <APIType>/<endpoint>/<name>.md).
const IndexFileName = "index.yaml"

// indexVersion is the schema version of index.yaml. Bump it only on an
// incompatible change to the shape the frontend consumes. Version 2 added the
// grouping contract: the header `vocabularies` and `programmes`, and the
// per-resource `groups`. Version 3 added the multi-axis facet contract: the
// header `facets` and the per-resource `facets` map (the transitional
// `programmes`/`groups` fields are still emitted alongside them). The frontend
// accepts any version >= 1 and ignores fields it does not know, so the bump is
// additive.
const indexVersion = 3

// Built-in assignment targets carry no group id; they are recognised by their
// @odata.type so the index can report "all users" / "all devices" as facts.
const (
	allUsersTargetKind   = "allLicensedUsersAssignmentTarget"
	allDevicesTargetKind = "allDevicesAssignmentTarget"
)

// GenerateIndexOptions bundles everything GenerateIndex needs. Like
// generate-prompt it never fetches a resource, never authenticates and writes
// only OutPath.
type GenerateIndexOptions struct {
	// TenantDir is the per-tenant export directory (the parent of resources/).
	TenantDir string
	// ExpectDomain, when non-empty, must equal metadata.yaml's tenant field or
	// GenerateIndex refuses. Empty skips the cross-check.
	ExpectDomain string
	// OutPath is where index.yaml is written. Empty defaults to
	// TenantDir/docs/index.yaml.
	OutPath string
	// Taxonomy, when non-nil, is the curated `taxonomy:` config section whose
	// programme rules classify each resource into the per-resource `groups` and
	// the header registry. Nil leaves both empty and the index falls back to
	// per-type grouping.
	Taxonomy *TaxonomyConfig
	// DryRun withholds the write: the index is still assembled and reported.
	DryRun bool
}

// IndexFile is the on-disk shape of docs/index.yaml. It is fully derived from
// resources/metadata.yaml (facts) enriched with each document's frontmatter
// (the LLM's summary and grouping), so it is safe to delete and regenerate at
// any time. It carries no wall-clock time — generatedAt mirrors the export — so
// re-running over an unchanged export produces byte-identical output.
type IndexFile struct {
	Version          int               `yaml:"version"`
	Tenant           string            `yaml:"tenant"`
	GeneratedAt      string            `yaml:"generatedAt"`
	Complete         bool              `yaml:"complete"`
	IncompleteReason string            `yaml:"incompleteReason,omitempty"`
	Vocabularies     IndexVocabularies `yaml:"vocabularies"`
	Programmes       []IndexProgramme  `yaml:"programmes,omitempty"`
	Facets           []IndexFacet      `yaml:"facets,omitempty"`
	Counts           IndexCounts       `yaml:"counts"`
	Resources        []IndexResource   `yaml:"resources"`
}

// IndexVocabularies carries the closed grouping vocabularies in display order,
// one list per axis, so a consumer orders navigation from the data rather than
// from a copy of the vocabulary that would drift. Both mirror the source-of-
// truth constants in internal/models.
type IndexVocabularies struct {
	Platform []string `yaml:"platform"`
	Function []string `yaml:"function"`
}

// IndexProgramme is one entry in the header programme registry: a stable id, a
// display label, and the number of resources matched in this tenant. The full
// registry is emitted in display order including programmes with a zero count,
// since "this programme is empty here" is itself information a consumer cannot
// recover from per-resource membership alone.
type IndexProgramme struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Count int    `yaml:"count"`
}

// IndexFacet is one filter axis in the header: a stable id, a display label, and
// its values in the taxonomy's display order, each with the number of resources
// matched in this tenant. Like the programme registry, zero-count values are
// kept — "this value matched nothing here" is information a consumer cannot
// recover from per-resource membership alone. Per-resource membership carries
// value ids only; the label is resolved from here.
type IndexFacet struct {
	ID     string            `yaml:"id"`
	Label  string            `yaml:"label"`
	Values []IndexFacetValue `yaml:"values"`
}

// IndexFacetValue is one value of a facet axis: its stable id, display label and
// tenant-wide match count. The count is used to enumerate values (a zero-count
// value is still a selectable chip); a consumer that shows selection-aware
// counts recomputes them from the resource list.
type IndexFacetValue struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
	Count int    `yaml:"count"`
}

// IndexGroup is one programme membership on a resource: a stable id (the value
// a consumer puts in a URL, which must survive a label rename) paired with a
// display label.
type IndexGroup struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}

// IndexCounts summarises the export for the picker and the "not documented"
// footnote. documented + pending is the in-scope total; excluded is the
// deliberately-undocumented bulk types (unreferenced groups, autopilot device
// identities), keyed by resource type.
type IndexCounts struct {
	Documented int            `yaml:"documented"`
	Pending    int            `yaml:"pending"`
	Excluded   map[string]int `yaml:"excluded,omitempty"`
}

// IndexResource is one in-scope resource's index entry: facts from
// metadata.yaml plus, when its document exists, the LLM's summary and grouping
// read from the document frontmatter. A resource with no document yet is listed
// with documented:false and a blank summary/grouping so the count stays honest
// and the frontend can show it as pending.
type IndexResource struct {
	Type          string              `yaml:"type"`
	Doc           string              `yaml:"doc"`
	DisplayName   string              `yaml:"displayName"`
	Summary       string              `yaml:"summary,omitempty"`
	Documented    bool                `yaml:"documented"`
	Scope         string              `yaml:"scope,omitempty"`
	PlatformGroup string              `yaml:"platformGroup,omitempty"`
	FunctionGroup string              `yaml:"functionGroup,omitempty"`
	ODataType     string              `yaml:"odataType,omitempty"`
	Platforms     string              `yaml:"platforms,omitempty"`
	Groups        []IndexGroup        `yaml:"groups,omitempty"`
	Facets        map[string][]string `yaml:"facets,omitempty"`
	Assignments   *IndexAssignments   `yaml:"assignments,omitempty"`
}

// IndexAssignments is the count-only assignment summary; resolved target names
// stay in the document itself. groups/allUsers/allDevices describe what a
// resource targets (an empty struct means assignment-capable but unassigned);
// targetedBy is set only on group entries and counts the resources assigning
// that group. The block is omitted entirely for types with no assignments
// concept.
type IndexAssignments struct {
	Groups     int  `yaml:"groups,omitempty"`
	AllUsers   bool `yaml:"allUsers,omitempty"`
	AllDevices bool `yaml:"allDevices,omitempty"`
	TargetedBy int  `yaml:"targetedBy,omitempty"`
}

// GenerateIndexResult reports what the index run found, for the command to
// print. Excluded mirrors IndexCounts.Excluded; Orphans counts in-scope
// resources gone from the tenant (documents left in place, not listed).
type GenerateIndexResult struct {
	OutPath     string
	Written     bool
	Tenant      string
	GeneratedAt string
	Complete    bool
	Documented  int
	Pending     int
	Orphans     int
	// Uncategorised counts, per axis id, the listed resources that matched no
	// value on that axis. It is populated only when a taxonomy was supplied;
	// without one it is empty. These per-axis counts are independent, so they
	// must NOT be summed into a single figure: a resource missing two axes would
	// be counted twice, overstating the problem.
	Uncategorised map[string]int
	// FullyUncategorised counts the listed resources that matched no value on
	// ANY axis — the resources that genuinely fell through the whole taxonomy,
	// as opposed to being merely absent from one facet. It is the honest headline
	// figure; the per-axis Uncategorised map carries the partial detail. Only
	// populated when a taxonomy was supplied.
	FullyUncategorised int
	Excluded           map[string]int
}

// GenerateIndex builds docs/index.yaml from resources/metadata.yaml, enriched
// with each in-scope document's frontmatter. It writes exactly one file
// (OutPath) unless DryRun is set, never touches resources/ and never deletes
// anything.
//
// It shares metadata.yaml, the in-scope rule and the assignment resolution with
// generate-prompt, so the index can never describe a different set of resources
// than the prompt documents.
func GenerateIndex(opts GenerateIndexOptions) (*GenerateIndexResult, error) {
	log := logger.Default

	resourcesDir := filepath.Join(opts.TenantDir, models.ResourcesDirName)
	metaPath := filepath.Join(resourcesDir, MetadataFileName)
	if _, err := os.Stat(metaPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNoMetadata, metaPath)
		}
		return nil, fmt.Errorf("failed to stat metadata: %w", err)
	}

	m, err := loadMetadata(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	if opts.ExpectDomain != "" && m.Tenant != "" && !strings.EqualFold(m.Tenant, opts.ExpectDomain) {
		return nil, fmt.Errorf("%w (metadata tenant %q, resolved %q)", ErrTenantMismatch, m.Tenant, opts.ExpectDomain)
	}

	var tax *taxonomy
	if opts.Taxonomy != nil {
		tax, err = compileTaxonomy(*opts.Taxonomy)
		if err != nil {
			return nil, err
		}
	}

	referenced, _ := referencedGroups(&m)
	targetedBy := buildTargetedBy(&m)

	idx := IndexFile{
		Version:          indexVersion,
		Tenant:           m.Tenant,
		GeneratedAt:      m.GeneratedAt,
		Complete:         m.Run.Complete,
		IncompleteReason: m.Run.IncompleteReason,
		Vocabularies: IndexVocabularies{
			Platform: models.PlatformGroups,
			Function: models.FunctionGroups,
		},
		Counts: IndexCounts{Excluded: map[string]int{}},
	}

	// programmeCounts accumulates matches on the "programme" axis so the
	// transitional programmes registry can report a per-programme count, zero
	// included. facetCounts does the same for every axis (axis id -> value id ->
	// count) for the multi-axis facets header.
	programmeCounts := map[string]int{}
	facetCounts := map[string]map[string]int{}
	res := &GenerateIndexResult{
		Tenant:        m.Tenant,
		GeneratedAt:   m.GeneratedAt,
		Complete:      m.Run.Complete,
		Excluded:      idx.Counts.Excluded,
		Uncategorised: map[string]int{},
	}

	// Keys are already sorted (type then name), so appended resources are
	// deterministic without a second sort.
	for _, key := range sortedResourceKeys(m.Resources) {
		entry := m.Resources[key]
		rtype := typeOfKey(key)

		// Excluded bulk types (unreferenced groups, autopilot) are not
		// documented: count present entries and skip.
		if !inScope(rtype, entry, referenced) {
			if entry.PresentInTenant {
				idx.Counts.Excluded[rtype]++
			}
			continue
		}

		// An orphan (resource gone from the tenant) is not part of the current
		// index; count it for the report but leave its document in place.
		if !entry.PresentInTenant {
			res.Orphans++
			continue
		}

		ir := IndexResource{
			Type:        rtype,
			Doc:         docTreePath(key),
			DisplayName: displayNameOr(entry, key),
			Scope:       scopeFromKey(key),
			ODataType:   entry.ODataType,
			Platforms:   entry.Platforms,
			Assignments: indexAssignments(rtype, entry, m.Types[rtype], targetedBy),
		}

		// Enrich with the LLM's summary and grouping when the document exists.
		// A missing document leaves the entry pending (documented:false) rather
		// than dropping it, so the count reflects the in-scope total.
		if docBytes, fm := readDocument(opts.TenantDir, key); docBytes != nil {
			ir.Documented = true
			if fm != nil {
				ir.Summary = fm.Summary
				ir.PlatformGroup = fm.PlatformGroup
				ir.FunctionGroup = fm.FunctionGroup
			}
		}

		// Classify on every axis when a taxonomy was supplied. classifyAxes
		// returns axes and values in display order, so the per-resource facets
		// and legacy groups are deterministic. A resource matching no value on
		// an axis is counted as uncategorised for that axis; the "programme"
		// axis is additionally mirrored into the transitional groups field.
		if tax != nil {
			facts := taxonomyFacts{
				name:      ir.DisplayName,
				rtype:     rtype,
				odataType: entry.ODataType,
				platforms: entry.Platforms,
				scope:     ir.Scope,
			}
			for _, ac := range tax.classifyAxes(facts) {
				if len(ac.values) == 0 {
					res.Uncategorised[ac.axisID]++
					continue
				}
				if facetCounts[ac.axisID] == nil {
					facetCounts[ac.axisID] = map[string]int{}
				}
				for _, v := range ac.values {
					if ir.Facets == nil {
						ir.Facets = map[string][]string{}
					}
					ir.Facets[ac.axisID] = append(ir.Facets[ac.axisID], v.id)
					facetCounts[ac.axisID][v.id]++
					if ac.axisID == programmeAxisID {
						ir.Groups = append(ir.Groups, IndexGroup{ID: v.id, Label: v.label})
						programmeCounts[v.id]++
					}
				}
			}
			// An empty facets map means the resource matched no value on any
			// axis — it fell through the whole taxonomy, not just one facet.
			if len(ir.Facets) == 0 {
				res.FullyUncategorised++
			}
		}

		idx.Resources = append(idx.Resources, ir)
		if ir.Documented {
			idx.Counts.Documented++
		} else {
			idx.Counts.Pending++
		}
	}

	// Emit the header registries in display order, including values with a zero
	// count, so a consumer can render the facet chooser and see that a value
	// matched nothing here. The facets registry covers every axis; the programmes
	// registry is the transitional single-axis view of the "programme" axis.
	if tax != nil {
		for _, a := range tax.axes {
			facet := IndexFacet{ID: a.id, Label: a.label}
			for _, v := range a.values {
				facet.Values = append(facet.Values, IndexFacetValue{ID: v.id, Label: v.label, Count: facetCounts[a.id][v.id]})
			}
			idx.Facets = append(idx.Facets, facet)
		}
		for _, p := range tax.registry() {
			idx.Programmes = append(idx.Programmes, IndexProgramme{ID: p.id, Label: p.label, Count: programmeCounts[p.id]})
		}
	}

	res.Documented = idx.Counts.Documented
	res.Pending = idx.Counts.Pending

	res.OutPath = opts.OutPath
	if res.OutPath == "" {
		res.OutPath = filepath.Join(opts.TenantDir, DocsDirName, IndexFileName)
	}

	if opts.DryRun {
		log.Info("Dry-run: not writing index file", "path", res.OutPath,
			"documented", res.Documented, "pending", res.Pending)
		return res, nil
	}

	data, err := yaml.Marshal(idx)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal index: %w", err)
	}
	if err := writeIndexFile(res.OutPath, data); err != nil {
		return nil, err
	}
	res.Written = true
	return res, nil
}

// indexAssignments builds the count-only assignment summary for one resource,
// or nil when the resource has no assignments concept and targets nothing. A
// group's targeted-by count is set only for group entries, keyed by the group's
// resource id.
func indexAssignments(rtype string, entry ResourceMeta, tm TypeMeta, targetedBy map[string][]reverseRow) *IndexAssignments {
	var a IndexAssignments
	present := false

	if tm.HasAssignments {
		present = true
		for _, r := range parseAssignments(entry.AssignmentTargets) {
			switch {
			case r.groupID != "":
				a.Groups++
			case strings.Contains(r.targetKind, allUsersTargetKind):
				a.AllUsers = true
			case strings.Contains(r.targetKind, allDevicesTargetKind):
				a.AllDevices = true
			}
		}
	}

	if rtype == groupsType {
		if n := len(targetedBy[entry.ResourceId]); n > 0 {
			a.TargetedBy = n
			present = true
		}
	}

	if !present {
		return nil
	}
	return &a
}

// docTreePath returns a resource's document path relative to the docs/ tree
// root (where index.yaml lives), e.g. "Microsoft.Graph/type/name.md". The
// frontend routes on this path.
func docTreePath(key string) string {
	return strings.TrimPrefix(docRel(key), DocsDirName+"/")
}

// displayNameOr returns the recorded display name, falling back to the source
// file's base name (without extension) so an entry always has a label.
func displayNameOr(entry ResourceMeta, key string) string {
	if entry.DisplayName != "" {
		return entry.DisplayName
	}
	return strings.TrimSuffix(path.Base(key), ".yaml")
}

// scopeFromKey derives the device/user scope from the resource file name's
// convention token (_d_ / _u_), or "" when the name carries no such token.
func scopeFromKey(key string) string {
	base := path.Base(key)
	switch {
	case strings.Contains(base, "_d_"):
		return "device"
	case strings.Contains(base, "_u_"):
		return "user"
	default:
		return ""
	}
}

// writeIndexFile creates the docs/ directory if needed and writes index.yaml.
// Writing index.yaml is, with generate.md, one of the two exceptions to
// azure-rd never writing under docs/.
func writeIndexFile(outPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("failed to create docs directory: %w", err)
	}
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}
	return nil
}
