package docs

import (
	"path/filepath"
	"regexp"
	"testing"

	"azure-resource-downloader/internal/models"
)

// TestPerEntryTransformConfigAttestation guards the per-entry config hash: it
// is stamped from the run that writes an entry's bytes and must survive a
// later partial run under a different configuration untouched, because the
// run-level hash attests only the types the last run covered. Without the
// per-entry fact, a drift check over such a mixed baseline would read a config
// change as mass drift.
func TestPerEntryTransformConfigAttestation(t *testing.T) {
	output := t.TempDir()
	resourcesDir := filepath.Join(output, models.ResourcesDirName)
	const otherType = "Microsoft.Graph/groups"

	aPath := writeYAML(t, resourcesDir, testType, "alpha")
	gPath := writeYAML(t, resourcesDir, otherType, "team")

	// Run 1: full export under config A writes both entries.
	run1 := exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
		successResult(gPath, otherType, "team", "id-g"),
	}, nil, nil), RunScope{}, false)
	run1.TransformConfigSha256 = "config-a"
	run1.FiltersSha256 = "filters-a"
	if err := WriteExportMetadata(run1); err != nil {
		t.Fatalf("run1: %v", err)
	}

	meta, err := loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load after run1: %v", err)
	}
	for key, entry := range meta.Resources {
		if entry.TransformConfigSha256 != "config-a" {
			t.Errorf("%s attests %q after run1, want config-a", key, entry.TransformConfigSha256)
		}
	}
	if meta.Run.FiltersSha256 != "filters-a" {
		t.Errorf("run filtersSha256 = %q, want filters-a", meta.Run.FiltersSha256)
	}

	// Run 2: a --type-scoped run under config B rewrites only testType. The
	// out-of-scope entry must keep attesting config A — the fact travels with
	// the bytes it describes, never with the last run.
	run2 := exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
	}, nil, nil), RunScope{Types: []string{testType}}, false)
	run2.TransformConfigSha256 = "config-b"
	if err := WriteExportMetadata(run2); err != nil {
		t.Fatalf("run2: %v", err)
	}

	meta, err = loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load after run2: %v", err)
	}
	if got := meta.Resources[testType+"/alpha.yaml"].TransformConfigSha256; got != "config-b" {
		t.Errorf("rewritten entry attests %q, want config-b", got)
	}
	if got := meta.Resources[otherType+"/team.yaml"].TransformConfigSha256; got != "config-a" {
		t.Errorf("out-of-scope entry attests %q, want config-a retained", got)
	}
	if meta.Run.TransformConfigSha256 != "config-b" {
		t.Errorf("run-level hash = %q, want the last run's config-b", meta.Run.TransformConfigSha256)
	}
}

// TestHashResourceFilters guards the canonical filter hash: equivalent
// configurations hash identically regardless of declaration order, different
// configurations differ, and no filters hashes stably (never to "").
func TestHashResourceFilters(t *testing.T) {
	mustFilter := func(resType string, props map[string]string) models.ResourceFilter {
		f := models.ResourceFilter{ResourceType: resType}
		for prop, pattern := range props {
			f.Properties = append(f.Properties, models.PropertyFilter{Property: prop, Pattern: regexp.MustCompile(pattern)})
		}
		return f
	}

	a := []models.ResourceFilter{
		mustFilter("Microsoft.Graph/groups", map[string]string{"displayName": "^GBL_"}),
		mustFilter(testType, map[string]string{"platforms": "windows"}),
	}
	b := []models.ResourceFilter{
		mustFilter(testType, map[string]string{"platforms": "windows"}),
		mustFilter("Microsoft.Graph/groups", map[string]string{"displayName": "^GBL_"}),
	}
	if HashResourceFilters(a) != HashResourceFilters(b) {
		t.Error("equivalent filter configurations must hash identically regardless of order")
	}

	c := []models.ResourceFilter{mustFilter(testType, map[string]string{"platforms": "macOS"})}
	if HashResourceFilters(a) == HashResourceFilters(c) {
		t.Error("different filter configurations must hash differently")
	}

	if HashResourceFilters(nil) == "" {
		t.Error("no filters must hash stably, never to the unattested sentinel \"\"")
	}
	if HashResourceFilters(nil) != HashResourceFilters([]models.ResourceFilter{}) {
		t.Error("nil and empty filter sets are the same configuration")
	}
}
