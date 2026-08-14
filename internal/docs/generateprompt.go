package docs

import (
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
	// ReferencedGroups is the count of groups referenced by an assignment.
	ReferencedGroups int
}

// GeneratePrompt compares resources/metadata.yaml against the documents under
// docs/ and renders the incremental documentation prompt. It writes exactly one
// file (OutPath) unless DryRun is set, never touches resources/ and never
// deletes anything.
//
// This is Phase 1: it decides list 1 (documents to generate) from source and
// prompt hashes. The re-splice and migrate blocks are rendered as "none"; the
// forward/reverse splice hashes are a later phase.
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

	res := &GeneratePromptResult{
		ExportGeneratedAt: m.GeneratedAt,
		ExportComplete:    m.Run.Complete,
		IncompleteReason:  m.Run.IncompleteReason,
		DanglingGroupIDs:  dangling,
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

		reason := staleReason(opts.TenantDir, key, entry, m.Types[rtype])
		if reason == "" {
			continue // current
		}
		res.ToGenerate = append(res.ToGenerate, WorkItem{
			ResourceType: rtype,
			SourcePath:   srcRel(key),
			DocPath:      docRel(key),
			Reason:       reason,
			SourceSha256: entry.SourceSha256,
			PromptSha256: m.Types[rtype].PromptSha256,
		})
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
		{"refmap", renderRefmap(&m, referenced)},
		{"resplice", renderNotImplemented("re-splice detection")},
		{"migrate", renderNotImplemented("marker migration")},
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

// staleReason returns why a document must be (re)generated, or "" when it is
// current. It never skips on a signal it could not read.
func staleReason(tenantDir, key string, entry ResourceMeta, tm TypeMeta) string {
	docPath := filepath.Join(tenantDir, filepath.FromSlash(docRel(key)))
	data, err := os.ReadFile(docPath)
	if err != nil {
		return "no document"
	}
	fm, ok := parseFrontmatter(data)
	if !ok {
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

// docFrontmatter is the self-describing state a generated document records.
type docFrontmatter struct {
	Source            string `yaml:"source"`
	SourceSha256      string `yaml:"sourceSha256"`
	PromptSha256      string `yaml:"promptSha256"`
	AssignmentsSha256 string `yaml:"assignmentsSha256"`
	TargetedBySha256  string `yaml:"targetedBySha256"`
	GeneratedAt       string `yaml:"generatedAt"`
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
