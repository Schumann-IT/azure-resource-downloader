// Package consistency implements `docs analyze-consistency`: it indexes every
// configured setting of an export, models every resource's assignment scope,
// and reports where two resources configure the same setting differently — or
// redundantly — for scopes that can reach the same device or user. It reads
// only resources/ (the YAML and metadata.yaml) and writes only its own
// consistency/ tree; the analysis is a docs command's decision, never a fact in
// resources/metadata.yaml.
package consistency

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// DirName is the tree the consistency analysis owns: a sibling of resources/,
// docs/ and drift/ under the tenant directory. It describes one export, so a
// re-baselining resource download clears it.
const DirName = "consistency"

// File names at the consistency/ tree root.
const (
	MechanicalFileName = "mechanical.yaml"
	MetadataFileName   = "metadata.yaml"
)

// Schema versions; bump on an incompatible shape change.
const (
	mechanicalVersion = 1
	metadataVersion   = 1
)

// Types read for the scope model, never indexed.
const (
	typeGroups  = "Microsoft.Graph/groups"
	typeFilters = "Microsoft.Graph/assignmentFilters"
)

// ErrWrite marks a failure writing the consistency/ tree.
var ErrWrite = errors.New("cannot write the consistency tree")

// Options bundles everything Analyze needs.
type Options struct {
	// TenantDir is the per-tenant export directory (the parent of resources/).
	TenantDir string
	// ExpectDomain, when non-empty, must equal metadata.yaml's tenant.
	ExpectDomain string
	// ToolVersion is recorded in consistency/metadata.yaml.
	ToolVersion string
	// Equivalences join canonical keys across policy types; empty means exact
	// keys only.
	Equivalences []Equivalence
	// DryRun computes everything and writes nothing.
	DryRun bool
}

// MechanicalFile is the on-disk shape of consistency/mechanical.yaml.
type MechanicalFile struct {
	Version       int            `yaml:"version"`
	Findings      []Finding      `yaml:"findings"`
	UnknownValues []UnknownValue `yaml:"unknownValues"`
}

// Finding is one mechanical finding for a resource pair.
type Finding struct {
	Kind       string      `yaml:"kind"`
	Key        string      `yaml:"key"`
	Overlap    string      `yaml:"overlap"`
	Confidence string      `yaml:"confidence"`
	Operator   string      `yaml:"operator,omitempty"`
	A          FindingSide `yaml:"a"`
	B          FindingSide `yaml:"b"`
}

// FindingSide is one resource of a finding with the value it configures.
type FindingSide struct {
	Resource  string `yaml:"resource"`
	SourceKey string `yaml:"sourceKey"`
	Value     string `yaml:"value"`
}

// UnknownValue is a pair on one key whose overlap is not none but one of whose
// values is unknown (a secret, a non-scalar across the bridge). It never
// carries a value.
type UnknownValue struct {
	Key     string      `yaml:"key"`
	Overlap string      `yaml:"overlap"`
	A       UnknownSide `yaml:"a"`
	B       UnknownSide `yaml:"b"`
}

// UnknownSide is one resource of an unknown-value pair.
type UnknownSide struct {
	Resource  string `yaml:"resource"`
	SourceKey string `yaml:"sourceKey"`
}

// MetadataFile is the on-disk shape of consistency/metadata.yaml. It carries no
// wall-clock time, so a rerun over an unchanged export is byte-equal.
type MetadataFile struct {
	Version           int    `yaml:"version"`
	Tenant            string `yaml:"tenant"`
	ExportGeneratedAt string `yaml:"exportGeneratedAt"`
	ExportComplete    bool   `yaml:"exportComplete"`
	ToolVersion       string `yaml:"toolVersion"`
	Counts            Counts `yaml:"counts"`
}

// Counts summarises one analysis.
type Counts struct {
	Indexed       map[string]SourceCount    `yaml:"indexed"`
	Unreadable    int                       `yaml:"unreadable"`
	Findings      map[string]map[string]int `yaml:"findings"`
	UnknownValues int                       `yaml:"unknownValues"`
}

// SourceCount is the number of indexed resources and settings of a source type.
type SourceCount struct {
	Resources int `yaml:"resources"`
	Settings  int `yaml:"settings"`
}

// Result is what Analyze computed (and, unless dry-run, wrote).
type Result struct {
	Dir        string
	Metadata   MetadataFile
	Mechanical MechanicalFile
	// Unreadable lists the metadata keys whose file was missing or did not
	// parse.
	Unreadable []string
}

// Analyze indexes the export's settings, models scopes, detects same-setting
// conflicts and duplicates, and writes consistency/mechanical.yaml then
// consistency/metadata.yaml. Other files in consistency/ are left alone.
func Analyze(opts Options) (*Result, error) {
	m, err := docs.LoadExportMetadata(opts.TenantDir)
	if err != nil {
		return nil, err
	}
	if opts.ExpectDomain != "" && m.Tenant != "" && !strings.EqualFold(m.Tenant, opts.ExpectDomain) {
		return nil, fmt.Errorf("%w (metadata tenant %q, resolved %q)", docs.ErrTenantMismatch, m.Tenant, opts.ExpectDomain)
	}

	resourcesDir := filepath.Join(opts.TenantDir, models.ResourcesDirName)
	settings, indexed, unreadable := buildIndex(&m, resourcesDir)

	groupKinds, filters := scopeInputs(&m, resourcesDir, indexed)
	scopes := map[string]*Scope{}
	for key := range indexed {
		scopes[key] = buildScope(m.Resources[key], groupKinds, filters)
	}

	findings, unknowns := detect(settings, scopes, opts.Equivalences)

	res := &Result{
		Dir:        filepath.Join(opts.TenantDir, DirName),
		Mechanical: MechanicalFile{Version: mechanicalVersion, Findings: findings, UnknownValues: unknowns},
		Unreadable: unreadable,
	}
	res.Metadata = MetadataFile{
		Version:           metadataVersion,
		Tenant:            m.Tenant,
		ExportGeneratedAt: m.GeneratedAt,
		ExportComplete:    m.Run.Complete,
		ToolVersion:       opts.ToolVersion,
		Counts:            countAll(settings, indexed, unreadable, findings, unknowns),
	}

	if opts.DryRun {
		return res, nil
	}
	if err := write(res); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWrite, err)
	}
	return res, nil
}

// buildIndex reads every in-scope resource of the indexed types and returns
// the settings, the indexed resource keys with their type, and the unreadable
// keys.
func buildIndex(m *docs.Metadata, resourcesDir string) ([]Setting, map[string]string, []string) {
	log := logger.Default
	isIndexed := map[string]bool{}
	for _, t := range indexedTypes {
		isIndexed[t] = true
	}

	var settings []Setting
	indexed := map[string]string{}
	var unreadable []string
	for _, key := range sortedKeys(m.Resources) {
		entry := m.Resources[key]
		rtype := path.Dir(filepath.ToSlash(key))
		if !isIndexed[rtype] || !entry.PresentInTenant || entry.Skipped || entry.Filtered {
			continue
		}
		doc, reason := readDoc(resourcesDir, key)
		if doc == nil {
			// The key only: never the file's content, which may hold secrets.
			log.Warn("Resource not readable; skipped by the consistency analysis", "resource", key, "reason", reason)
			unreadable = append(unreadable, key)
			continue
		}
		indexed[key] = rtype
		typeDir := filepath.Join(resourcesDir, filepath.FromSlash(rtype))
		settings = append(settings, indexResource(key, rtype, doc, entry.Artifacts, typeDir)...)
	}
	return settings, indexed, unreadable
}

// readDoc reads and parses a resource YAML; on failure it returns nil and a
// reason that never quotes the file.
func readDoc(resourcesDir, key string) (map[string]interface{}, string) {
	data, err := os.ReadFile(filepath.Join(resourcesDir, filepath.FromSlash(key)))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "missing"
		}
		return nil, "cannot be read"
	}
	var doc map[string]interface{}
	if err := yaml.Unmarshal(data, &doc); err != nil || doc == nil {
		return nil, "does not parse"
	}
	return doc, ""
}

// scopeInputs resolves the target kind of every group and the platform and
// rule of every filter that an indexed resource's assignments reference.
func scopeInputs(m *docs.Metadata, resourcesDir string, indexed map[string]string) (map[string]string, map[string]filterInfo) {
	groupRefs, filterRefs := referencedTargets(m, indexed)

	groupKinds := map[string]string{}
	filters := map[string]filterInfo{}
	for _, key := range sortedKeys(m.Resources) {
		entry := m.Resources[key]
		switch path.Dir(filepath.ToSlash(key)) {
		case typeGroups:
			if !groupRefs[entry.ResourceId] {
				continue
			}
			groupKinds[entry.ResourceId] = resolveGroupKind(resourcesDir, key, entry.GroupTypes)
		case typeFilters:
			if !filterRefs[entry.ResourceId] {
				continue
			}
			if doc, _ := readDoc(resourcesDir, key); doc != nil {
				platform, _ := doc["platform"].(string)
				rule, _ := doc["rule"].(string)
				filters[entry.ResourceId] = filterInfo{Platform: platform, Rule: rule}
			}
		}
	}
	return groupKinds, filters
}

// referencedTargets collects the group and filter ids the indexed resources'
// assignments reference.
func referencedTargets(m *docs.Metadata, indexed map[string]string) (groupRefs, filterRefs map[string]bool) {
	groupRefs = map[string]bool{}
	filterRefs = map[string]bool{}
	for key := range indexed {
		for _, t := range docs.ParseAssignmentTargets(m.Resources[key].AssignmentTargets) {
			if t.GroupID != "" {
				groupRefs[t.GroupID] = true
			}
			if t.FilterID != "" {
				filterRefs[t.FilterID] = true
			}
		}
	}
	return groupRefs, filterRefs
}

// resolveGroupKind is unknown unless the group is dynamic and its document
// readable, then the kind its membership rule names.
func resolveGroupKind(resourcesDir, key string, groupTypes []string) string {
	if !isDynamic(groupTypes) {
		return kindUnknown
	}
	doc, _ := readDoc(resourcesDir, key)
	if doc == nil {
		return kindUnknown
	}
	rule, _ := doc["membershipRule"].(string)
	return groupKind(rule)
}

func isDynamic(groupTypes []string) bool {
	for _, t := range groupTypes {
		if strings.EqualFold(t, "DynamicMembership") {
			return true
		}
	}
	return false
}

// countAll builds the metadata counts; every source type, kind and overlap is
// listed, zero included, so the shape never depends on the tenant.
func countAll(settings []Setting, indexed map[string]string, unreadable []string, findings []Finding, unknowns []UnknownValue) Counts {
	c := Counts{
		Indexed:       map[string]SourceCount{},
		Unreadable:    len(unreadable),
		Findings:      map[string]map[string]int{},
		UnknownValues: len(unknowns),
	}
	for _, t := range indexedTypes {
		c.Indexed[t] = SourceCount{}
	}
	for _, t := range indexed {
		sc := c.Indexed[t]
		sc.Resources++
		c.Indexed[t] = sc
	}
	for _, s := range settings {
		sc := c.Indexed[s.SourceType]
		sc.Settings++
		c.Indexed[s.SourceType] = sc
	}
	for _, k := range []string{KindConflict, KindContradiction, KindDuplicate} {
		c.Findings[k] = map[string]int{OverlapCertain: 0, OverlapPossible: 0}
	}
	for _, f := range findings {
		c.Findings[f.Kind][f.Overlap]++
	}
	return c
}

// write writes mechanical.yaml first and metadata.yaml last, both atomically,
// so a consumer that sees the metadata can trust the findings beside it.
func write(res *Result) error {
	if err := os.MkdirAll(res.Dir, 0755); err != nil {
		return fmt.Errorf("failed to create %s: %w", DirName, err)
	}
	mech, err := yaml.Marshal(&res.Mechanical)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", MechanicalFileName, err)
	}
	if err := docs.WriteFileAtomic(filepath.Join(res.Dir, MechanicalFileName), mech); err != nil {
		return err
	}
	meta, err := yaml.Marshal(&res.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal %s: %w", MetadataFileName, err)
	}
	return docs.WriteFileAtomic(filepath.Join(res.Dir, MetadataFileName), meta)
}

// ClearTree removes a tenant's consistency/ tree entirely, reporting whether it
// existed. A re-baselining resource download calls it after updating
// resources/metadata.yaml: the analysis describes one export, so a new baseline
// supersedes it. The path is constructed here — never derived from any input —
// so this cannot reach into resources/, docs/ or drift/.
func ClearTree(tenantDir string) (bool, error) {
	dir := filepath.Join(tenantDir, DirName)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat consistency tree: %w", err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return false, fmt.Errorf("failed to clear consistency tree: %w", err)
	}
	return true, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
