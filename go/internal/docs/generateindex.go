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
// incompatible change to the shape the frontend consumes.
const indexVersion = 1

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
	// DryRun withholds the write: the index is still assembled and reported.
	DryRun bool
}

// IndexFile is the on-disk shape of docs/index.yaml. It is fully derived from
// resources/metadata.yaml (facts) enriched with each document's frontmatter
// (the LLM's summary and grouping), so it is safe to delete and regenerate at
// any time. It carries no wall-clock time — generatedAt mirrors the export — so
// re-running over an unchanged export produces byte-identical output.
type IndexFile struct {
	Version          int             `yaml:"version"`
	Tenant           string          `yaml:"tenant"`
	GeneratedAt      string          `yaml:"generatedAt"`
	Complete         bool            `yaml:"complete"`
	IncompleteReason string          `yaml:"incompleteReason,omitempty"`
	Counts           IndexCounts     `yaml:"counts"`
	Resources        []IndexResource `yaml:"resources"`
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
	Type          string            `yaml:"type"`
	Doc           string            `yaml:"doc"`
	DisplayName   string            `yaml:"displayName"`
	Summary       string            `yaml:"summary,omitempty"`
	Documented    bool              `yaml:"documented"`
	Scope         string            `yaml:"scope,omitempty"`
	PlatformGroup string            `yaml:"platformGroup,omitempty"`
	FunctionGroup string            `yaml:"functionGroup,omitempty"`
	ODataType     string            `yaml:"odataType,omitempty"`
	Platforms     string            `yaml:"platforms,omitempty"`
	Assignments   *IndexAssignments `yaml:"assignments,omitempty"`
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
	Excluded    map[string]int
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

	referenced, _ := referencedGroups(&m)
	targetedBy := buildTargetedBy(&m)

	idx := IndexFile{
		Version:          indexVersion,
		Tenant:           m.Tenant,
		GeneratedAt:      m.GeneratedAt,
		Complete:         m.Run.Complete,
		IncompleteReason: m.Run.IncompleteReason,
		Counts:           IndexCounts{Excluded: map[string]int{}},
	}
	res := &GenerateIndexResult{
		Tenant:      m.Tenant,
		GeneratedAt: m.GeneratedAt,
		Complete:    m.Run.Complete,
		Excluded:    idx.Counts.Excluded,
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

		idx.Resources = append(idx.Resources, ir)
		if ir.Documented {
			idx.Counts.Documented++
		} else {
			idx.Counts.Pending++
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
