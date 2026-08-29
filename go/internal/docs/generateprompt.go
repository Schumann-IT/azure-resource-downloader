package docs

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// generatePromptTemplate is the default template spliced by GeneratePrompt. This
// embedded file is the source of truth for the incremental documentation prompt;
// edit it directly. (It must live inside this package because go:embed cannot
// reach outside the package directory.)
//
//go:embed generate_prompt_template.md
var generatePromptTemplate []byte

// DefaultGeneratePromptTemplate returns the embedded default template bytes.
func DefaultGeneratePromptTemplate() []byte {
	out := make([]byte, len(generatePromptTemplate))
	copy(out, generatePromptTemplate)
	return out
}

// Structural facts about the Graph API that decide what is documented. These
// are not per-tenant preferences, so they live here as constants rather than as
// config surface (a one-off override would be a --include-type flag).
const (
	// groupsType is documented only for groups referenced by an assignment.
	groupsType = "Microsoft.Graph/groups"
	// autopilotIdentitiesType is never documented — a bulk directory record.
	autopilotIdentitiesType = "Microsoft.Graph/windowsAutopilotDeviceIdentities"
	// docPromptFileName is the per-type spec that gates generation of its type.
	docPromptFileName = "doc-prompt.md"
	// GenerateFileName is the single file azure-rd writes under docs/.
	GenerateFileName = "generate.md"
	// DocsDirName is the sibling tree that holds generated documentation. It is
	// not tool-owned except for GenerateFileName at its root.
	DocsDirName = "docs"
)

// Sentinel errors let the command map a failure to a distinct exit code.
var (
	// ErrNoMetadata is returned when the export directory has no
	// resources/metadata.yaml, so there is nothing to compare against.
	ErrNoMetadata = errors.New("no resources/metadata.yaml in export directory")
	// ErrTenantMismatch is returned when metadata.yaml's tenant does not match
	// the resolved tenant domain: documenting the wrong export is worse than
	// stopping.
	ErrTenantMismatch = errors.New("export tenant does not match resolved tenant domain")
)

// GeneratePromptOptions bundles everything GeneratePrompt needs. It never
// fetches a resource, never authenticates and never writes outside OutPath.
type GeneratePromptOptions struct {
	// TenantDir is the per-tenant export directory (the parent of resources/).
	TenantDir string
	// ExpectDomain, when non-empty, must equal metadata.yaml's tenant field or
	// GeneratePrompt refuses. Empty skips the cross-check.
	ExpectDomain string
	// Template is the prompt template to splice (default or --prompt override).
	Template []byte
	// OutPath is where the finished prompt is written. Empty defaults to
	// TenantDir/docs/generate.md.
	OutPath string
	// DryRun withholds the write: the comparison still runs.
	DryRun bool
}

// WorkItem is one resource whose document must be generated.
type WorkItem struct {
	ResourceType string
	SourcePath   string // tenant-relative, under resources/
	DocPath      string // tenant-relative, under docs/
	Reason       string
	SourceSha256 string
	PromptSha256 string
	// AssignmentsSha256 is the forward hash the generated document must record
	// in its frontmatter. It is set only for assignment-capable types; blank
	// for types with no assignments concept.
	AssignmentsSha256 string
}

// RespliceItem is one document whose marked block must be re-rendered even
// though its own resource is current, because the block is rendered from
// information outside that resource (see appendix A of the template). Hash is
// the new value to write into the document's frontmatter after splicing:
// assignmentsSha256 for a forward item, targetedBySha256 for a reverse one.
type RespliceItem struct {
	ResourceType string
	SourcePath   string // tenant-relative, under resources/
	DocPath      string // tenant-relative, under docs/
	Reason       string
	Hash         string
}

// GeneratePromptResult reports what the comparison found, for the command to
// print and to decide an exit code from.
type GeneratePromptResult struct {
	OutPath           string
	Written           bool
	ExportGeneratedAt string
	ExportComplete    bool
	IncompleteReason  string
	ToGenerate        []WorkItem
	// Orphans are in-scope entries marked presentInTenant:false — reported, not
	// generated and never deleted.
	Orphans []string
	// PromptMissingTypes are in-scope types whose doc-prompt.md is absent, so no
	// document of that type can be produced (e.g. an export run with --no-prompt).
	PromptMissingTypes []string
	// DanglingGroupIDs are assignment target GUIDs with no group in the export.
	DanglingGroupIDs []string
	// DanglingFilterIDs are assignment filter GUIDs referenced by an assignment
	// with no filter in the export (the sentinel "no filter" id is excluded).
	DanglingFilterIDs []string
	// ReferencedGroups is the count of groups referenced by an assignment.
	ReferencedGroups int
	// ForwardResplice lists current documents whose own assignments table must
	// be re-rendered because a referenced group or filter changed name, kind or
	// presence (the resource's own YAML did not move, so it is not in
	// ToGenerate). Excludes documents already in ToGenerate, which get a fresh
	// block anyway.
	ForwardResplice []RespliceItem
	// ReverseResplice lists current group documents whose "Targeted by" block
	// must be re-rendered because the set of resources targeting the group
	// changed.
	ReverseResplice []RespliceItem
	// Migrate lists current documents of an assignment-capable type that predate
	// the assignment markers, so the markers must be inserted before the block
	// can be spliced.
	Migrate []WorkItem
}

// HasPendingWork reports whether any document is out of date in any sense:
// missing/stale (ToGenerate), a marked block needing a re-splice (forward or
// reverse), or a document needing marker migration. --exit-code gates on this,
// so a clean CI run means every document on disk matches the export.
func (r *GeneratePromptResult) HasPendingWork() bool {
	return len(r.ToGenerate) > 0 ||
		len(r.ForwardResplice) > 0 ||
		len(r.ReverseResplice) > 0 ||
		len(r.Migrate) > 0
}

// GeneratePrompt compares resources/metadata.yaml against the documents under
// docs/ and renders the incremental documentation prompt. It writes exactly one
// file (OutPath) unless DryRun is set, never touches resources/ and never
// deletes anything.
//
// It decides two lists in one pass over the in-scope documents, reading each
// document's bytes once:
//   - list 1 (ToGenerate): documents missing or whose source/spec hash moved,
//     regenerated wholesale;
//   - list 2 (ForwardResplice / ReverseResplice / Migrate): current documents
//     whose marked assignment block was rendered from information outside their
//     own resource and has since gone stale, or that predate the markers.
func GeneratePrompt(opts GeneratePromptOptions) (*GeneratePromptResult, error) {
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

	if err := validateMarkers(opts.Template, requiredMarkers); err != nil {
		return nil, err
	}

	referenced, dangling := referencedGroups(&m)

	// The group and filter indexes and the reverse (targeted-by) index resolve
	// every hash and every rendered table from one source, so the command and
	// the agent can never render from different facts.
	groups := buildGroupInfo(&m)
	filters := buildFilterInfo(&m)
	targetedBy := buildTargetedBy(&m)

	res := &GeneratePromptResult{
		ExportGeneratedAt: m.GeneratedAt,
		ExportComplete:    m.Run.Complete,
		IncompleteReason:  m.Run.IncompleteReason,
		DanglingGroupIDs:  dangling,
		DanglingFilterIDs: danglingFilterIDs(&m, filters),
		ReferencedGroups:  len(referenced),
	}

	// Whether a type's doc-prompt.md is present, computed once per type.
	promptPresent := map[string]bool{}
	promptMissing := map[string]bool{}

	for _, key := range sortedResourceKeys(m.Resources) {
		entry := m.Resources[key]
		rtype := typeOfKey(key)

		if !inScope(rtype, entry, referenced) {
			continue
		}

		// An orphaned document (resource gone from the tenant) is reported and
		// left in place, never regenerated.
		if !entry.PresentInTenant {
			res.Orphans = append(res.Orphans, srcRel(key))
			continue
		}

		// The type's doc-prompt.md is the input to generation; without it no
		// document of that type can be produced.
		if _, known := promptPresent[rtype]; !known {
			present := fileExists(filepath.Join(resourcesDir, filepath.FromSlash(rtype), docPromptFileName))
			promptPresent[rtype] = present
			if !present {
				promptMissing[rtype] = true
			}
		}
		if !promptPresent[rtype] {
			continue
		}

		tm := m.Types[rtype]

		// Read the document once; this pass decides list 1 and, for a current
		// document, list 2 (re-splice / migrate) without a second walk.
		docBytes, fm := readDocument(opts.TenantDir, key)

		// List 1: the document is missing, unreadable, or its source/spec hash
		// moved. It is regenerated wholesale (with a fresh assignments block),
		// so it is never also a list-2 candidate.
		if reason := list1Reason(entry, tm, docBytes, fm); reason != "" {
			item := WorkItem{
				ResourceType: rtype,
				SourcePath:   srcRel(key),
				DocPath:      docRel(key),
				Reason:       reason,
				SourceSha256: entry.SourceSha256,
				PromptSha256: tm.PromptSha256,
			}
			// The generated document must record the forward hash the tool
			// computed; the agent never computes a hash.
			if tm.HasAssignments {
				item.AssignmentsSha256 = assignmentsSha256(parseAssignments(entry.AssignmentTargets), groups, filters)
			}
			res.ToGenerate = append(res.ToGenerate, item)
			continue
		}

		// List 2, forward: an assignment-capable type's document is current but
		// its own assignments block was rendered from inputs that have since
		// moved. A document predating the markers is migrated first (5e) so the
		// splice has something to replace.
		if tm.HasAssignments {
			if !hasAssignmentsMarker(docBytes) {
				res.Migrate = append(res.Migrate, WorkItem{
					ResourceType:      rtype,
					SourcePath:        srcRel(key),
					DocPath:           docRel(key),
					Reason:            "document predates the assignment markers",
					SourceSha256:      entry.SourceSha256,
					PromptSha256:      tm.PromptSha256,
					AssignmentsSha256: assignmentsSha256(parseAssignments(entry.AssignmentTargets), groups, filters),
				})
			} else if want := assignmentsSha256(parseAssignments(entry.AssignmentTargets), groups, filters); want != fm.AssignmentsSha256 {
				res.ForwardResplice = append(res.ForwardResplice, RespliceItem{
					ResourceType: rtype,
					SourcePath:   srcRel(key),
					DocPath:      docRel(key),
					Reason:       forwardRespliceReason(fm.AssignmentsSha256),
					Hash:         want,
				})
			}
		}

		// List 2, reverse: a group document is current but the set of resources
		// targeting it changed. Groups have no assignments concept of their own,
		// so this is independent of HasAssignments.
		if rtype == groupsType {
			if want := targetedBySha256(targetedBy[entry.ResourceId], filters); want != fm.TargetedBySha256 {
				res.ReverseResplice = append(res.ReverseResplice, RespliceItem{
					ResourceType: rtype,
					SourcePath:   srcRel(key),
					DocPath:      docRel(key),
					Reason:       reverseRespliceReason(fm.TargetedBySha256),
					Hash:         want,
				})
			}
		}
	}

	res.PromptMissingTypes = sortedKeys(promptMissing)

	// Splice the computed blocks into the template.
	out := opts.Template
	blocks := []struct {
		name    string
		content string
	}{
		{"export", renderExport(opts.TenantDir, &m)},
		{"worklist", renderWorklist(res.ToGenerate)},
		{"refmap", renderRefmap(&m, referenced, groups)},
		{"resplice", renderResplice(res.ForwardResplice, res.ReverseResplice)},
		{"migrate", renderMigrate(res.Migrate)},
	}
	for _, b := range blocks {
		out, err = spliceMarker(out, b.name, b.content)
		if err != nil {
			return nil, err
		}
	}

	res.OutPath = opts.OutPath
	if res.OutPath == "" {
		res.OutPath = filepath.Join(opts.TenantDir, DocsDirName, GenerateFileName)
	}

	if opts.DryRun {
		log.Info("Dry-run: not writing prompt file", "path", res.OutPath, "to_generate", len(res.ToGenerate))
		return res, nil
	}

	if err := writeGeneratePrompt(res.OutPath, out); err != nil {
		return nil, err
	}
	res.Written = true
	return res, nil
}

// inScope reports whether a resource entry should be considered for
// documentation: excluded types are dropped, groups only when referenced.
func inScope(rtype string, entry ResourceMeta, referenced map[string]bool) bool {
	switch rtype {
	case autopilotIdentitiesType:
		return false
	case groupsType:
		return referenced[entry.ResourceId]
	default:
		return true
	}
}

// readDocument reads a document's bytes and parses its frontmatter, returning
// nil bytes when the file cannot be read and nil frontmatter when there is no
// parseable block. Reading once serves both list-1 staleness and the list-2
// marker/hash checks.
func readDocument(tenantDir, key string) ([]byte, *docFrontmatter) {
	docPath := filepath.Join(tenantDir, filepath.FromSlash(docRel(key)))
	data, err := os.ReadFile(docPath)
	if err != nil {
		return nil, nil
	}
	fm, ok := parseFrontmatter(data)
	if !ok {
		return data, nil
	}
	return data, fm
}

// list1Reason returns why a document must be regenerated wholesale, or "" when
// it is current. It never skips on a signal it could not read: a missing file
// or unreadable frontmatter regenerates rather than being trusted.
func list1Reason(entry ResourceMeta, tm TypeMeta, docBytes []byte, fm *docFrontmatter) string {
	if docBytes == nil {
		return "no document"
	}
	if fm == nil {
		return "frontmatter missing or unreadable"
	}
	if fm.SourceSha256 != entry.SourceSha256 {
		return "resource changed since document was written"
	}
	if fm.PromptSha256 != tm.PromptSha256 {
		return "type spec (doc-prompt.md) changed"
	}
	return ""
}

// hasAssignmentsMarker reports whether a document already carries the assignment
// splice markers. A current assignment-capable document without them predates
// the markers and must be migrated (5e) before its block can be spliced.
func hasAssignmentsMarker(docBytes []byte) bool {
	return bytes.Contains(docBytes, []byte("<!-- assignments:start -->"))
}

// forwardRespliceReason describes why a document's assignments block is stale,
// distinguishing a block that was never spliced from one whose inputs moved.
func forwardRespliceReason(existing string) string {
	if existing == "" {
		return "assignments block never spliced (no assignmentsSha256 recorded)"
	}
	return "a referenced group or filter changed name, kind or presence"
}

// reverseRespliceReason mirrors forwardRespliceReason for a group's Targeted by
// block.
func reverseRespliceReason(existing string) string {
	if existing == "" {
		return "Targeted by block never spliced (no targetedBySha256 recorded)"
	}
	return "the set of resources targeting this group changed"
}

// docFrontmatter is the self-describing state a generated document records.
type docFrontmatter struct {
	Source            string `yaml:"source"`
	SourceSha256      string `yaml:"sourceSha256"`
	PromptSha256      string `yaml:"promptSha256"`
	AssignmentsSha256 string `yaml:"assignmentsSha256"`
	TargetedBySha256  string `yaml:"targetedBySha256"`
	GeneratedAt       string `yaml:"generatedAt"`
	// Summary, PlatformGroup and FunctionGroup are the LLM-authored index
	// signals: a one-line purpose and the navigation grouping the model chose
	// for this resource. generate-index reads them to build docs/index.yaml;
	// generate-prompt ignores them (they never affect staleness).
	Summary       string `yaml:"summary"`
	PlatformGroup string `yaml:"platformGroup"`
	FunctionGroup string `yaml:"functionGroup"`
}

// parseFrontmatter extracts the leading YAML frontmatter block delimited by
// "---" lines. It returns ok=false when there is no parseable block, which the
// caller treats as "regenerate" rather than trusting a signal it could not read.
func parseFrontmatter(b []byte) (*docFrontmatter, bool) {
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return nil, false
	}
	rest := s[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, false
	}
	var fm docFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return nil, false
	}
	return &fm, true
}

// referencedGroups returns the set of group IDs referenced by any assignment
// target in the export, and the sorted list of referenced GUIDs with no group
// entry (dangling references).
func referencedGroups(m *Metadata) (map[string]bool, []string) {
	groupIDs := map[string]bool{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == autopilotIdentitiesType {
			continue
		}
		for _, id := range assignmentGroupIDs(entry.AssignmentTargets) {
			groupIDs[id] = true
		}
	}

	// A group ID is resolvable when some group entry carries it as resourceId.
	known := map[string]bool{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == groupsType && entry.ResourceId != "" {
			known[entry.ResourceId] = true
		}
	}

	var danglingSet []string
	for id := range groupIDs {
		if !known[id] {
			danglingSet = append(danglingSet, id)
		}
	}
	sort.Strings(danglingSet)
	return groupIDs, danglingSet
}

// assignmentGroupIDs extracts the group target GUIDs from a resource's raw
// assignment targets, ignoring built-in targets (all users / all devices) that
// carry no groupId.
func assignmentGroupIDs(targets []interface{}) []string {
	var ids []string
	for _, raw := range targets {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		target, ok := entry["target"].(map[string]interface{})
		if !ok {
			continue
		}
		if id, ok := target["groupId"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// srcRel returns the tenant-relative source path for a metadata key.
func srcRel(key string) string {
	return path.Join(models.ResourcesDirName, key)
}

// docRel returns the tenant-relative document path mirroring a metadata key:
// the resources/ tree root and .yaml extension are swapped for docs/ and .md.
func docRel(key string) string {
	noExt := strings.TrimSuffix(key, ".yaml")
	return path.Join(DocsDirName, noExt+".md")
}

// groupKeyByID indexes referenced group IDs to their metadata key.
func groupKeyByID(m *Metadata) map[string]string {
	byKey := map[string]string{}
	for key, entry := range m.Resources {
		if typeOfKey(key) == groupsType && entry.ResourceId != "" {
			byKey[entry.ResourceId] = key
		}
	}
	return byKey
}

func sortedResourceKeys(res map[string]ResourceMeta) []string {
	keys := make([]string, 0, len(res))
	for k := range res {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// writeGeneratePrompt creates the docs/ directory if needed and writes the
// finished prompt. Writing generate.md is the one exception to azure-rd never
// writing under docs/.
func writeGeneratePrompt(outPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("failed to create docs directory: %w", err)
	}
	if err := os.WriteFile(outPath, content, 0644); err != nil {
		return fmt.Errorf("failed to write prompt file: %w", err)
	}
	return nil
}
