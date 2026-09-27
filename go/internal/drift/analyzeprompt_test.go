package drift

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// analyzeTemplate is a minimal valid template carrying every required marker.
var analyzeTemplate = []byte(`intro
<!-- observation:start -->
X
<!-- observation:end -->
<!-- worklist:start -->
X
<!-- worklist:end -->
<!-- refmap:start -->
X
<!-- refmap:end -->
outro
`)

// writeBaselineMetadata persists a baseline as resources/metadata.yaml so the
// engine reads it through the shared loader, exactly like a real export.
func writeBaselineMetadata(t *testing.T, tenantDir string, meta docs.Metadata) {
	t.Helper()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	if err := os.MkdirAll(resourcesDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(&meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourcesDir, docs.MetadataFileName), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// driftedTenant builds a full tenant directory with one changed resource: a
// baseline export, and a persisted drift observation produced by the real
// Compare + WriteObservation path.
func driftedTenant(t *testing.T) string {
	t.Helper()
	tenantDir := t.TempDir()

	oldData := map[string]interface{}{"id": "id-b", "displayName": "B", "setting": "old"}
	newData := map[string]interface{}{"id": "id-b", "displayName": "B", "setting": "new"}

	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/b.yaml": entry(t, "id-b", "B", oldData),
	})
	writeBaselineMetadata(t, tenantDir, meta)

	// The baseline YAML on disk, so Compare records the field deltas.
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	oldBytes, _ := mustMarshal(t, oldData)
	if err := os.MkdirAll(filepath.Join(resourcesDir, filepath.FromSlash(testType)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourcesDir, filepath.FromSlash(testType), "b.yaml"), oldBytes, 0644); err != nil {
		t.Fatal(err)
	}

	rep := Compare(Options{
		Baseline:               meta,
		ResourcesDir:           resourcesDir,
		CurrentTransformSha256: testCfgSha,
		TotalRequests:          1,
		Results:                []*models.TransformResult{tr("id-b", "B", "b", newData)},
	})
	if _, err := WriteObservation(tenantDir, rep, time.Now(), "test", false); err != nil {
		t.Fatalf("write observation: %v", err)
	}
	return tenantDir
}

func TestGenerateAnalyzePromptWritesPrompt(t *testing.T) {
	tenantDir := driftedTenant(t)

	res, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, ExpectDomain: testTenant, Template: analyzeTemplate})
	if err != nil {
		t.Fatalf("GenerateAnalyzePrompt: %v", err)
	}
	if !res.Written || res.NothingToAnalyze {
		t.Fatalf("result = %+v, want a written prompt with findings", res)
	}
	if res.Counts.Changed != 1 {
		t.Errorf("counts = %+v, want one changed", res.Counts)
	}

	wantPath := filepath.Join(tenantDir, DriftDirName, AnalyzeFileName)
	if res.OutPath != wantPath {
		t.Errorf("OutPath = %q, want %q", res.OutPath, wantPath)
	}
	body, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("prompt not on disk: %v", err)
	}
	s := string(body)

	// The worklist names the finding's three files, its drift-document
	// destination (the payload path with the extension swapped) and its delta.
	for _, want := range []string{
		"#### changed: B",
		"resources/" + testType + "/b.yaml",
		"drift/" + testType + "/b.yaml",
		"docs/" + testType + "/b.md",
		"Your drift document (write it): `drift/" + testType + "/b.md`",
		"`setting`: `old` → `new`",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("prompt must contain %q:\n%s", want, s)
		}
	}
	// The observation block names the index destination.
	if !strings.Contains(s, IndexFileName) {
		t.Errorf("prompt must name the index destination %q", IndexFileName)
	}
}

func TestGenerateAnalyzePromptDeterministic(t *testing.T) {
	tenantDir := driftedTenant(t)

	render := func() string {
		res, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
		if err != nil {
			t.Fatalf("GenerateAnalyzePrompt: %v", err)
		}
		body, err := os.ReadFile(res.OutPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}

	if a, b := render(), render(); a != b {
		t.Errorf("two identical runs rendered different prompts:\n%s\n---\n%s", a, b)
	}
}

func TestGenerateAnalyzePromptDryRunWithholdsWrite(t *testing.T) {
	tenantDir := driftedTenant(t)

	res, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate, DryRun: true})
	if err != nil {
		t.Fatalf("GenerateAnalyzePrompt: %v", err)
	}
	if res.Written {
		t.Error("dry run must not report a write")
	}
	if _, err := os.Stat(res.OutPath); !os.IsNotExist(err) {
		t.Errorf("dry run must not write the prompt, found %s", res.OutPath)
	}
}

func TestGenerateAnalyzePromptRefusals(t *testing.T) {
	t.Run("no observation", func(t *testing.T) {
		tenantDir := t.TempDir()
		writeBaselineMetadata(t, tenantDir, baseline(nil))

		_, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
		if !errors.Is(err, ErrNoObservation) {
			t.Errorf("err = %v, want ErrNoObservation", err)
		}
	})

	t.Run("no export metadata", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		if err := os.Remove(filepath.Join(tenantDir, models.ResourcesDirName, docs.MetadataFileName)); err != nil {
			t.Fatal(err)
		}

		_, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
		if !errors.Is(err, docs.ErrNoMetadata) {
			t.Errorf("err = %v, want ErrNoMetadata", err)
		}
	})

	t.Run("superseded observation", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		// A re-download moves the baseline's generatedAt.
		meta := baseline(nil)
		meta.GeneratedAt = "2026-09-02T00:00:00Z"
		writeBaselineMetadata(t, tenantDir, meta)

		_, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
		if !errors.Is(err, ErrObservationSuperseded) {
			t.Errorf("err = %v, want ErrObservationSuperseded", err)
		}
	})

	t.Run("tenant mismatch", func(t *testing.T) {
		tenantDir := driftedTenant(t)

		_, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, ExpectDomain: "other.onmicrosoft.com", Template: analyzeTemplate})
		if !errors.Is(err, docs.ErrTenantMismatch) {
			t.Errorf("err = %v, want ErrTenantMismatch", err)
		}
	})

	t.Run("payload mismatch", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		payload := filepath.Join(tenantDir, DriftDirName, filepath.FromSlash(testType), "b.yaml")
		if err := os.WriteFile(payload, []byte("tampered: true\n"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
		if !errors.Is(err, ErrPayloadMismatch) {
			t.Errorf("err = %v, want ErrPayloadMismatch", err)
		}
	})

	t.Run("broken template", func(t *testing.T) {
		tenantDir := driftedTenant(t)

		_, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: []byte("no markers")})
		if err == nil || !strings.Contains(err.Error(), "observation") {
			t.Errorf("err = %v, want a marker validation error naming the first missing block", err)
		}
	})
}

func TestGenerateAnalyzePromptNothingToAnalyze(t *testing.T) {
	tenantDir := t.TempDir()
	data := map[string]interface{}{"id": "id-a", "displayName": "A"}
	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/a.yaml": entry(t, "id-a", "A", data),
	})
	writeBaselineMetadata(t, tenantDir, meta)

	rep := compare(meta, []*models.TransformResult{tr("id-a", "A", "a", data)})
	if _, err := WriteObservation(tenantDir, rep, time.Now(), "test", false); err != nil {
		t.Fatal(err)
	}

	res, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
	if err != nil {
		t.Fatalf("GenerateAnalyzePrompt: %v", err)
	}
	if !res.NothingToAnalyze || res.Written {
		t.Errorf("result = %+v, want nothing-to-analyze with no write", res)
	}
	if _, err := os.Stat(res.OutPath); !os.IsNotExist(err) {
		t.Errorf("a clean observation must not produce a prompt, found %s", res.OutPath)
	}
}

func TestGenerateAnalyzePromptFlagsMissingSpec(t *testing.T) {
	tenantDir := driftedTenant(t)

	res, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
	if err != nil {
		t.Fatal(err)
	}
	// driftedTenant writes no doc-prompt.md, so the finding's type has no spec.
	if len(res.MissingSpecTypes) != 1 || res.MissingSpecTypes[0] != testType {
		t.Errorf("MissingSpecTypes = %v, want [%s]", res.MissingSpecTypes, testType)
	}

	// With the spec on disk, the caveat disappears and the heading names it.
	specDir := filepath.Join(tenantDir, models.ResourcesDirName, filepath.FromSlash(testType))
	if err := os.MkdirAll(specDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "doc-prompt.md"), []byte("spec"), 0644); err != nil {
		t.Fatal(err)
	}
	res, err = GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.MissingSpecTypes) != 0 {
		t.Errorf("MissingSpecTypes = %v, want none once the spec exists", res.MissingSpecTypes)
	}
}

func TestReportPathForKey(t *testing.T) {
	// The drift document sits at the payload's path with the extension swapped,
	// so all four per-resource files share one key.
	if got, want := ReportPathForKey(testType+"/b.yaml"), testType+"/b.md"; got != want {
		t.Errorf("ReportPathForKey = %q, want %q", got, want)
	}
}

func TestDefaultAnalyzeTemplateValidates(t *testing.T) {
	if err := docs.ValidateMarkers(DefaultAnalyzeTemplate(), requiredAnalyzeMarkers); err != nil {
		t.Fatalf("embedded template must carry every required marker: %v", err)
	}
}

func TestClearTree(t *testing.T) {
	tenantDir := t.TempDir()

	// Siblings that must survive a clear.
	for _, dir := range []string{models.ResourcesDirName, docs.DocsDirName} {
		if err := os.MkdirAll(filepath.Join(tenantDir, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("no tree is a no-op", func(t *testing.T) {
		removed, err := ClearTree(tenantDir)
		if err != nil || removed {
			t.Errorf("ClearTree = (%v, %v), want (false, nil) when there is nothing to clear", removed, err)
		}
	})

	t.Run("clears the tree and only the tree", func(t *testing.T) {
		driftDir := filepath.Join(tenantDir, DriftDirName)
		if err := os.MkdirAll(filepath.Join(driftDir, "sub"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(driftDir, ObservationFileName), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		removed, err := ClearTree(tenantDir)
		if err != nil || !removed {
			t.Fatalf("ClearTree = (%v, %v), want (true, nil)", removed, err)
		}
		if _, err := os.Stat(driftDir); !os.IsNotExist(err) {
			t.Error("drift tree must be gone")
		}
		for _, dir := range []string{models.ResourcesDirName, docs.DocsDirName} {
			if _, err := os.Stat(filepath.Join(tenantDir, dir)); err != nil {
				t.Errorf("sibling %s must survive a clear: %v", dir, err)
			}
		}
	})
}
