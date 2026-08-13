package docs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"
)

const testType = "Microsoft.Graph/deviceCompliancePolicies"

// writeYAML creates a placeholder resource file so prune has something to delete.
func writeYAML(t *testing.T, resourcesDir, resType, name string) string {
	t.Helper()
	dir := filepath.Join(resourcesDir, filepath.FromSlash(resType))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(p, []byte("displayName: "+name+"\n"), 0644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return p
}

func successResult(yamlPath, resType, name, id string) *models.WriteResult {
	return &models.WriteResult{
		ResourceID:   id,
		ResourceType: resType,
		YAMLPath:     yamlPath,
		Facts: &models.ResourceFacts{
			ResourceID:   id,
			DisplayName:  name,
			SourceSha256: "sha-" + name,
		},
	}
}

func newSummary(complete bool, results []*models.WriteResult, empty []string, skipped []models.SkippedType) *pipeline.ExecutionSummary {
	return &pipeline.ExecutionSummary{
		TotalResources: len(results),
		Complete:       complete,
		Results:        results,
		EmptyTypes:     empty,
		SkippedTypes:   skipped,
	}
}

func exportRun(output string, summary *pipeline.ExecutionSummary, scope RunScope, prune bool) ExportRun {
	return ExportRun{
		Output:      output,
		Tenant:      "example.com",
		ToolVersion: "azure-rd test",
		GeneratedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Scope:       scope,
		Summary:     summary,
		Prune:       prune,
	}
}

func TestWriteExportMetadataFreshAndAbsence(t *testing.T) {
	output := t.TempDir()
	resourcesDir := filepath.Join(output, models.ResourcesDirName)
	scope := RunScope{Types: []string{testType}}

	aPath := writeYAML(t, resourcesDir, testType, "alpha")
	bPath := writeYAML(t, resourcesDir, testType, "bravo")

	run1 := exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
		successResult(bPath, testType, "bravo", "id-b"),
	}, nil, nil), scope, false)
	if err := WriteExportMetadata(run1); err != nil {
		t.Fatalf("run1: %v", err)
	}

	meta, err := loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load after run1: %v", err)
	}
	if len(meta.Resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(meta.Resources))
	}
	for key, entry := range meta.Resources {
		if !entry.PresentInTenant {
			t.Errorf("%s should be present after run1", key)
		}
	}
	if tm := meta.Types[testType]; tm.LastCoveredBy != coveredByType {
		t.Errorf("expected type covered by --type, got %q", tm.LastCoveredBy)
	}

	// Run 2: bravo is gone from the tenant.
	run2 := exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
	}, nil, nil), scope, false)
	if err := WriteExportMetadata(run2); err != nil {
		t.Fatalf("run2: %v", err)
	}

	meta, err = loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load after run2: %v", err)
	}
	if len(meta.Resources) != 2 {
		t.Fatalf("expected 2 resources retained, got %d", len(meta.Resources))
	}
	if !meta.Resources["Microsoft.Graph/deviceCompliancePolicies/alpha.yaml"].PresentInTenant {
		t.Error("alpha should still be present")
	}
	if meta.Resources["Microsoft.Graph/deviceCompliancePolicies/bravo.yaml"].PresentInTenant {
		t.Error("bravo should be marked absent after run2")
	}
}

func TestPruneDeletesAbsentResources(t *testing.T) {
	output := t.TempDir()
	resourcesDir := filepath.Join(output, models.ResourcesDirName)
	scope := RunScope{Types: []string{testType}}

	aPath := writeYAML(t, resourcesDir, testType, "alpha")
	bPath := writeYAML(t, resourcesDir, testType, "bravo")

	// Seed: both present.
	if err := WriteExportMetadata(exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
		successResult(bPath, testType, "bravo", "id-b"),
	}, nil, nil), scope, false)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Prune run: only alpha remains in the tenant.
	if err := WriteExportMetadata(exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
	}, nil, nil), scope, true)); err != nil {
		t.Fatalf("prune run: %v", err)
	}

	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Errorf("bravo.yaml should have been pruned, stat err = %v", err)
	}
	if _, err := os.Stat(aPath); err != nil {
		t.Errorf("alpha.yaml should remain: %v", err)
	}

	meta, err := loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !meta.Run.Pruned {
		t.Error("run.pruned should be true")
	}
	if _, ok := meta.Resources["Microsoft.Graph/deviceCompliancePolicies/bravo.yaml"]; ok {
		t.Error("bravo entry should be removed after prune")
	}
	if len(meta.Resources) != 1 {
		t.Errorf("expected 1 resource after prune, got %d", len(meta.Resources))
	}
}

func TestPruneRefusesIncompleteRun(t *testing.T) {
	output := t.TempDir()
	resourcesDir := filepath.Join(output, models.ResourcesDirName)
	scope := RunScope{Types: []string{testType}}

	aPath := writeYAML(t, resourcesDir, testType, "alpha")
	bPath := writeYAML(t, resourcesDir, testType, "bravo")

	if err := WriteExportMetadata(exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
		successResult(bPath, testType, "bravo", "id-b"),
	}, nil, nil), scope, false)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Incomplete run must not prune, even though bravo appears absent.
	incomplete := newSummary(false, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
	}, nil, []models.SkippedType{{ResourceType: testType, Reason: "boom"}})
	incomplete.IncompleteReason = "1 resource types could not be listed"
	if err := WriteExportMetadata(exportRun(output, incomplete, scope, true)); err != nil {
		t.Fatalf("incomplete run: %v", err)
	}

	if _, err := os.Stat(bPath); err != nil {
		t.Errorf("bravo.yaml must not be pruned on an incomplete run: %v", err)
	}
	meta, err := loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if meta.Run.Pruned {
		t.Error("run.pruned must be false on an incomplete run")
	}
}

func TestMergeRetainsOutOfScopeEntries(t *testing.T) {
	output := t.TempDir()
	resourcesDir := filepath.Join(output, models.ResourcesDirName)

	otherType := "Microsoft.Graph/groups"
	aPath := writeYAML(t, resourcesDir, testType, "alpha")
	gPath := writeYAML(t, resourcesDir, otherType, "grp")

	// Seed a full run covering both types.
	if err := WriteExportMetadata(exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
		successResult(gPath, otherType, "grp", "id-g"),
	}, nil, nil), RunScope{}, false)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// A --type run for only testType must not touch the groups entry.
	if err := WriteExportMetadata(exportRun(output, newSummary(true, []*models.WriteResult{
		successResult(aPath, testType, "alpha", "id-a"),
	}, nil, nil), RunScope{Types: []string{testType}}, false)); err != nil {
		t.Fatalf("type run: %v", err)
	}

	meta, err := loadMetadata(filepath.Join(resourcesDir, MetadataFileName))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := meta.Resources["Microsoft.Graph/groups/grp.yaml"]
	if !g.PresentInTenant {
		t.Error("out-of-scope groups entry must remain present")
	}
}

func TestHashTransformConfig(t *testing.T) {
	cfg := []models.TransformerConfig{
		{Name: "cleaning", Config: map[string]interface{}{"clean-empty": true}},
	}
	h1 := HashTransformConfig(cfg, false)
	h2 := HashTransformConfig(cfg, false)
	if h1 == "" || h1 != h2 {
		t.Fatalf("hash should be stable and non-empty, got %q and %q", h1, h2)
	}
	if HashTransformConfig(cfg, true) == h1 {
		t.Error("resolve-secrets should change the transform config hash")
	}
}
