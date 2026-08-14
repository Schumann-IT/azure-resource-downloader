// Package docs owns the export metadata file (resources/metadata.yaml): its
// format, the merge that folds a run's results into any previous export, the
// prune set derived from it, and marshalling. It records facts only — what a
// resource is, never how it should be classified — so revising a classification
// never requires re-downloading a tenant.
package docs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"

	"gopkg.in/yaml.v3"
)

// MetadataFileName is the name of the export metadata file, written at the root
// of the resources/ tree it describes.
const MetadataFileName = "metadata.yaml"

// Coverage source labels recorded per type as lastCoveredBy.
const (
	coveredByFull          = "full"
	coveredByType          = "--type"
	coveredByResourceID    = "--resource-id"
	coveredByResourceGroup = "--resource-group"
)

// RunScope records what a run was asked to download. An empty scope is a full
// export.
type RunScope struct {
	Types         []string
	ResourceIDs   []string
	ResourceGroup string
}

// ExportRun bundles everything WriteExportMetadata needs from a single download
// run. All of it is available in cmd.runDownload at write time.
type ExportRun struct {
	// Output is the per-tenant output directory (the parent of resources/).
	Output      string
	Tenant      string
	ToolVersion string
	GeneratedAt time.Time
	Scope       RunScope
	// TransformConfigSha256 explains mass hash movement: flip the transform
	// config and every resource hash moves, which a later diff can attribute.
	TransformConfigSha256 string
	ResolveSecrets        bool
	WritePrompts          bool
	DryRun                bool
	// Prune requests deletion of files this run establishes are gone from the
	// tenant. It is honoured only for a complete run with no failures.
	Prune   bool
	Summary *pipeline.ExecutionSummary
}

// Metadata is the on-disk shape of resources/metadata.yaml.
type Metadata struct {
	GeneratedAt string                  `yaml:"generatedAt"`
	Tenant      string                  `yaml:"tenant"`
	ToolVersion string                  `yaml:"toolVersion"`
	Run         RunMeta                 `yaml:"run"`
	Types       map[string]TypeMeta     `yaml:"types"`
	Resources   map[string]ResourceMeta `yaml:"resources"`
	NotListed   NotListedMeta           `yaml:"notListed"`
}

// RunMeta records run-level facts.
type RunMeta struct {
	Complete              bool      `yaml:"complete"`
	IncompleteReason      string    `yaml:"incompleteReason"`
	Scope                 ScopeMeta `yaml:"scope"`
	TransformConfigSha256 string    `yaml:"transformConfigSha256"`
	ResolveSecrets        bool      `yaml:"resolveSecrets"`
	WritePrompts          bool      `yaml:"writePrompts"`
	Pruned                bool      `yaml:"pruned"`
}

// ScopeMeta mirrors RunScope for serialisation.
type ScopeMeta struct {
	Types         []string `yaml:"types"`
	ResourceIds   []string `yaml:"resourceIds"`
	ResourceGroup string   `yaml:"resourceGroup"`
}

// TypeMeta records per-type facts.
type TypeMeta struct {
	PromptSha256      string `yaml:"promptSha256,omitempty"`
	PromptFileWritten bool   `yaml:"promptFileWritten"`
	LastCoveredAt     string `yaml:"lastCoveredAt,omitempty"`
	LastCoveredBy     string `yaml:"lastCoveredBy,omitempty"`
}

// ResourceMeta records per-resource facts. The map key is the resource path
// relative to metadata.yaml's own directory, so the key is the source path.
type ResourceMeta struct {
	ResourceId        string        `yaml:"resourceId,omitempty"`
	DisplayName       string        `yaml:"displayName,omitempty"`
	SourceSha256      string        `yaml:"sourceSha256,omitempty"`
	ODataType         string        `yaml:"odataType,omitempty"`
	Platforms         string        `yaml:"platforms,omitempty"`
	Technologies      string        `yaml:"technologies,omitempty"`
	Artifacts         []string      `yaml:"artifacts"`
	PresentInTenant   bool          `yaml:"presentInTenant"`
	Filtered          bool          `yaml:"filtered,omitempty"`
	Skipped           bool          `yaml:"skipped,omitempty"`
	LastSeenAt        string        `yaml:"lastSeenAt,omitempty"`
	AssignmentTargets []interface{} `yaml:"assignmentTargets,omitempty"`
	// GroupTypes and SecurityEnabled are group-only facts, retained so a later
	// step (docs generate-prompt) can resolve a referenced group's kind from
	// metadata.yaml alone. They are recorded raw; the rendered
	// "dynamic security group" phrasing is left to the renderer.
	GroupTypes      []string `yaml:"groupTypes,omitempty"`
	SecurityEnabled *bool    `yaml:"securityEnabled,omitempty"`
}

// NotListedMeta records this run's types that could not be listed (permissions)
// and types that listed to zero resources.
type NotListedMeta struct {
	Types []string `yaml:"types"`
	Empty []string `yaml:"empty"`
}

// HashTransformConfig returns a stable SHA-256 over the effective transform
// configuration and the resolve-secrets switch. Both change the YAML bytes of
// every resource, so recording their hash lets a later diff attribute a mass
// hash movement to a config change rather than a mass edit.
func HashTransformConfig(configs []models.TransformerConfig, resolveSecrets bool) string {
	payload := struct {
		Transformers   []models.TransformerConfig `yaml:"transformers"`
		ResolveSecrets bool                       `yaml:"resolveSecrets"`
	}{
		Transformers:   configs,
		ResolveSecrets: resolveSecrets,
	}
	data, err := yaml.Marshal(payload)
	if err != nil {
		// Marshalling a config of plain maps and strings cannot realistically
		// fail; return empty rather than panicking.
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// WriteExportMetadata merges this run's results into any existing
// resources/metadata.yaml and writes the union back. It never truncates a
// previous export: types outside this run's scope, and files still on disk, are
// preserved. When run.Prune is set and the run is complete with no failures, it
// also deletes the files this run establishes are gone from the tenant and
// drops their entries — one write, not two.
//
// It never returns an error that should fail the download: the YAML resources
// are the valuable output, so callers should only warn on failure.
func WriteExportMetadata(run ExportRun) error {
	log := logger.Default
	resourcesDir := filepath.Join(run.Output, models.ResourcesDirName)
	metaPath := filepath.Join(resourcesDir, MetadataFileName)

	existing, err := loadMetadata(metaPath)
	if err != nil {
		return fmt.Errorf("failed to read existing metadata: %w", err)
	}

	// Merge in-memory first so a dry run can enumerate exactly what a real run
	// would do — including which files --prune would delete — rather than
	// returning before it can tell.
	merged := mergeMetadata(existing, run, resourcesDir)

	if run.DryRun {
		log.Info("Dry-run: skipping export metadata write", "path", metaPath)
		if run.Prune {
			previewPrune(&merged, run)
		}
		return nil
	}

	// Prune runs after the merge, deleting exactly the entries the merge marked
	// absent in covered types. It is guarded on a complete, failure-free run.
	if run.Prune {
		pruneCovered(&merged, run, resourcesDir)
	}

	if err := writeMetadata(metaPath, resourcesDir, &merged); err != nil {
		return err
	}
	log.Info("Export metadata written", "path", metaPath, "resources", len(merged.Resources), "types", len(merged.Types))
	return nil
}

// loadMetadata reads an existing metadata file, returning an empty (but
// initialised) Metadata when the file does not exist.
func loadMetadata(metaPath string) (Metadata, error) {
	m := Metadata{
		Types:     map[string]TypeMeta{},
		Resources: map[string]ResourceMeta{},
	}
	data, err := os.ReadFile(metaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return m, err
	}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("failed to parse metadata: %w", err)
	}
	if m.Types == nil {
		m.Types = map[string]TypeMeta{}
	}
	if m.Resources == nil {
		m.Resources = map[string]ResourceMeta{}
	}
	return m, nil
}

// scopeMeta converts a RunScope into its serialisable form with sorted slices
// for deterministic output.
func scopeMeta(s RunScope) ScopeMeta {
	return ScopeMeta{
		Types:         sortedCopy(s.Types),
		ResourceIds:   sortedCopy(s.ResourceIDs),
		ResourceGroup: s.ResourceGroup,
	}
}

// coverageLabel names how this run selected resources, recorded per covered
// type as lastCoveredBy.
func coverageLabel(s RunScope) string {
	switch {
	case len(s.ResourceIDs) > 0:
		return coveredByResourceID
	case s.ResourceGroup != "":
		return coveredByResourceGroup
	case len(s.Types) > 0:
		return coveredByType
	default:
		return coveredByFull
	}
}

// typeOfKey derives the resource type from a metadata key, which is the source
// path "<type>/<name>.yaml" relative to the resources/ directory.
func typeOfKey(key string) string {
	return path.Dir(filepath.ToSlash(key))
}

// sortedCopy returns a sorted copy of in, or nil when empty, so slices never
// alias the caller's data and marshal deterministically.
func sortedCopy(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	sort.Strings(out)
	return out
}

// mergeMetadata folds a run's results into the previous metadata following the
// coverage rules: covered types have their resource entries reconciled; skipped
// types and out-of-scope types are retained untouched.
func mergeMetadata(m Metadata, run ExportRun, resourcesDir string) Metadata {
	s := run.Summary
	now := run.GeneratedAt.UTC().Format(time.RFC3339)

	m.GeneratedAt = now
	m.Tenant = run.Tenant
	m.ToolVersion = run.ToolVersion
	m.Run = RunMeta{
		Complete:              s.Complete,
		IncompleteReason:      s.IncompleteReason,
		Scope:                 scopeMeta(run.Scope),
		TransformConfigSha256: run.TransformConfigSha256,
		ResolveSecrets:        run.ResolveSecrets,
		WritePrompts:          run.WritePrompts,
		Pruned:                false,
	}
	m.NotListed = NotListedMeta{
		Types: skippedTypeNames(s.SkippedTypes),
		Empty: sortedCopy(s.EmptyTypes),
	}

	coveredBy := coverageLabel(run.Scope)

	// Index existing entries by resource id so skipped/filtered results (which
	// carry no file path) can find the entry describing their on-disk YAML.
	keyByID := map[string]string{}
	for key, entry := range m.Resources {
		if entry.ResourceId != "" {
			keyByID[entry.ResourceId] = key
		}
	}

	// Covered types: those this run listed successfully (produced results or
	// listed to empty), minus types whose listing failed. Only meaningful for a
	// full/--type run; id/group runs cover nothing.
	covered := coveredTypes(s, run.Scope)

	// Advance coverage timestamps for covered types.
	for t := range covered {
		tm := m.Types[t]
		tm.LastCoveredAt = now
		tm.LastCoveredBy = coveredBy
		m.Types[t] = tm
	}

	// Record per-type documentation prompt hashes (only populated when
	// prompt writing was enabled, i.e. --no-prompt was not passed).
	for t, sha := range s.PromptSHAByType {
		tm := m.Types[t]
		tm.PromptSha256 = sha
		tm.PromptFileWritten = run.WritePrompts
		m.Types[t] = tm
	}

	// presentKeys collects the entries written this run; touchedIDs collects
	// resources re-observed but not rewritten (skipped/filtered). Absence
	// detection must not flip either to presentInTenant:false.
	presentKeys := map[string]bool{}
	touchedIDs := map[string]bool{}

	for _, r := range s.Results {
		switch {
		case r.Facts != nil && r.YAMLPath != "":
			// Successful write: upsert full facts.
			key := relKey(resourcesDir, r.YAMLPath)
			m.Resources[key] = ResourceMeta{
				ResourceId:        r.Facts.ResourceID,
				DisplayName:       r.Facts.DisplayName,
				SourceSha256:      r.Facts.SourceSha256,
				ODataType:         r.Facts.ODataType,
				Platforms:         r.Facts.Platforms,
				Technologies:      r.Facts.Technologies,
				Artifacts:         sortedCopy(r.Facts.Artifacts),
				PresentInTenant:   true,
				LastSeenAt:        now,
				AssignmentTargets: r.Facts.AssignmentTargets,
				GroupTypes:        sortedCopy(r.Facts.GroupTypes),
				SecurityEnabled:   r.Facts.SecurityEnabled,
			}
			presentKeys[key] = true

		case r.YAMLPath != "" && r.Error == nil && !r.Skipped && !r.Filtered && !r.Cancelled:
			// Dry-run: the resource was fetched and would have been written
			// (YAMLPath is set), but carries no facts because nothing was
			// marshalled. Record presence against any existing entry so absence
			// detection — and the prune preview built on it — stays accurate,
			// without overwriting facts from an earlier real run. A real run
			// never reaches here: a successful write always carries facts, and
			// a failed one carries no YAMLPath.
			key := relKey(resourcesDir, r.YAMLPath)
			entry := m.Resources[key]
			entry.PresentInTenant = true
			entry.LastSeenAt = now
			m.Resources[key] = entry
			presentKeys[key] = true

		case r.Skipped, r.Filtered:
			// Re-observed but not rewritten. Retain the previous entry's facts
			// and mark why it was not refreshed. Without a file path we can only
			// match an existing entry by resource id.
			if key, ok := keyByID[r.ResourceID]; ok {
				entry := m.Resources[key]
				entry.Skipped = r.Skipped
				entry.Filtered = r.Filtered
				m.Resources[key] = entry
				touchedIDs[r.ResourceID] = true
			}
		}
	}

	// Absence detection: on a complete run, an entry in a covered type that was
	// neither rewritten nor re-observed this run is gone from the tenant.
	if s.Complete {
		for key, entry := range m.Resources {
			if !covered[typeOfKey(key)] {
				continue
			}
			if presentKeys[key] || touchedIDs[entry.ResourceId] {
				continue
			}
			entry.PresentInTenant = false
			m.Resources[key] = entry
		}
	}

	return m
}

// relKey returns the metadata map key for a written YAML path: its slash-style
// path relative to the resources/ directory.
func relKey(resourcesDir, yamlPath string) string {
	rel, err := filepath.Rel(resourcesDir, yamlPath)
	if err != nil {
		return filepath.ToSlash(yamlPath)
	}
	return filepath.ToSlash(rel)
}

// skippedTypeNames returns the sorted names of types that could not be listed.
func skippedTypeNames(skipped []models.SkippedType) []string {
	names := make([]string, 0, len(skipped))
	for _, st := range skipped {
		names = append(names, st.ResourceType)
	}
	return sortedCopy(names)
}

// coveredTypes returns the set of resource types this run listed successfully.
// A type is covered when its listing produced results or listed to empty and
// did not fail. Coverage is only meaningful for a full or --type run: id/group
// runs did not enumerate any type and therefore cover nothing.
func coveredTypes(s *pipeline.ExecutionSummary, scope RunScope) map[string]bool {
	covered := map[string]bool{}
	if len(scope.ResourceIDs) != 0 || scope.ResourceGroup != "" {
		return covered
	}
	for _, r := range s.Results {
		if r.ResourceType != "" && !r.Cancelled {
			covered[r.ResourceType] = true
		}
	}
	for _, t := range s.EmptyTypes {
		covered[t] = true
	}
	for _, st := range s.SkippedTypes {
		delete(covered, st.ResourceType)
	}
	return covered
}

// prunableKeys returns the sorted metadata keys a --prune run would delete:
// entries the merge marked absent (presentInTenant:false) within a type this
// run covered. The bool is false when the run is not eligible to prune (an
// incomplete run, or one with failures), with the reason logged — so the real
// prune and the dry-run preview share one eligibility decision and one
// selection and can never diverge.
func prunableKeys(m *Metadata, run ExportRun) ([]string, bool) {
	log := logger.Default
	s := run.Summary

	if !s.Complete {
		log.Warn("Refusing to prune: run is incomplete", "reason", s.IncompleteReason)
		return nil, false
	}
	if s.FailedResources > 0 {
		log.Warn("Refusing to prune: run had failures", "failed", s.FailedResources)
		return nil, false
	}

	covered := coveredTypes(s, run.Scope)

	var keys []string
	for key, entry := range m.Resources {
		if covered[typeOfKey(key)] && !entry.PresentInTenant {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, true
}

// previewPrune logs the files a real --prune would delete, without touching
// disk. It shares prunableKeys with pruneCovered so the preview can never
// diverge from what a real run would remove.
func previewPrune(m *Metadata, run ExportRun) {
	log := logger.Default

	keys, ok := prunableKeys(m, run)
	if !ok {
		return
	}
	if len(keys) == 0 {
		log.Info("Dry-run: prune would delete nothing")
		return
	}
	for _, key := range keys {
		entry := m.Resources[key]
		log.Info("Dry-run: would prune resource no longer in tenant", "path", key, "resource_id", entry.ResourceId)
	}
	log.Info("Dry-run: prune preview complete", "would_delete", len(keys))
}

// pruneCovered deletes files for entries the merge marked absent
// (presentInTenant:false) within a covered type, drops their metadata entries,
// and sets run.pruned. It refuses to run unless the run is complete and had no
// failures, because only then is an absence a reliable deletion signal.
func pruneCovered(m *Metadata, run ExportRun, resourcesDir string) {
	log := logger.Default

	keys, ok := prunableKeys(m, run)
	if !ok {
		return
	}
	m.Run.Pruned = true
	if len(keys) == 0 {
		log.Info("Prune: nothing to delete")
		return
	}

	// Count entries that will remain per type so we can tell when an entire type
	// directory (and thus its doc-prompt.md) is going away.
	remainingByType := map[string]int{}
	for key, entry := range m.Resources {
		if entry.PresentInTenant {
			remainingByType[typeOfKey(key)]++
		}
	}

	deleted := 0
	for _, key := range keys {
		entry := m.Resources[key]
		yamlPath := filepath.Join(resourcesDir, filepath.FromSlash(key))
		if err := removeFile(yamlPath); err != nil {
			log.Warn("Prune: failed to delete file", "path", yamlPath, "error", err)
			continue
		}
		log.Info("Prune: deleted resource no longer in tenant", "path", yamlPath, "resource_id", entry.ResourceId)
		deleted++

		// Sidecar artifacts live alongside the YAML.
		dir := filepath.Dir(yamlPath)
		for _, name := range entry.Artifacts {
			artifactPath := filepath.Join(dir, name)
			if err := removeFile(artifactPath); err != nil {
				log.Warn("Prune: failed to delete artifact", "path", artifactPath, "error", err)
			}
		}
		delete(m.Resources, key)
	}

	// When a covered type has no remaining resources, remove its doc-prompt.md
	// and the now-empty type directory. The prompt is deleted only here, when
	// the whole type directory is being removed; otherwise the run rewrites it.
	covered := coveredTypes(run.Summary, run.Scope)
	for t := range covered {
		if remainingByType[t] > 0 {
			continue
		}
		typeDir := filepath.Join(resourcesDir, filepath.FromSlash(t))
		_ = removeFile(filepath.Join(typeDir, "doc-prompt.md"))
		delete(m.Types, t)
		if err := os.Remove(typeDir); err != nil && !os.IsNotExist(err) {
			log.Debug("Prune: type directory not empty after cleanup", "dir", typeDir, "error", err)
		}
	}

	log.Info("Prune: complete", "deleted", deleted, "candidates", len(keys))
}

// writeMetadata marshals the metadata and writes it into the resources/
// directory, creating that directory if necessary.
func writeMetadata(metaPath, resourcesDir string, m *Metadata) error {
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		return fmt.Errorf("failed to create resources directory: %w", err)
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if err := os.WriteFile(metaPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}
	return nil
}

// removeFile deletes a file, treating an already-absent file as success.
func removeFile(p string) error {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
