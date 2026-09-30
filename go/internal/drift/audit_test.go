package drift

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/docs"

	"gopkg.in/yaml.v3"
)

// attributionFor builds a current attribution of the observation on disk.
func attributionFor(t *testing.T, tenantDir string, findings map[string]AttributionFinding) *Attribution {
	t.Helper()
	obs, err := LoadObservation(tenantDir)
	if err != nil {
		t.Fatalf("load observation: %v", err)
	}
	a := &Attribution{
		Version:             AttributionVersion,
		ObservedAt:          obs.ObservedAt,
		BaselineGeneratedAt: obs.Baseline.GeneratedAt,
		Tenant:              obs.Tenant,
		ToolVersion:         "test",
		QueriedAt:           "2026-09-30T12:00:00Z",
		WorkspaceID:         "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b",
		Window:              AttributionWindow{From: obs.Baseline.GeneratedAt, To: obs.ObservedAt},
		Tables: map[string]TableStatus{
			TableIntuneAuditLogs: {Status: TableStatusFailed, Reason: "permission denied"},
			TableAuditLogs:       {Status: TableStatusOK, Earliest: "2026-01-01T00:00:00Z"},
		},
		Findings: findings,
	}
	a.Tally()
	return a
}

func TestCheckCurrent(t *testing.T) {
	t.Run("current observation", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		obs, meta, err := CheckCurrent(tenantDir, testTenant)
		if err != nil {
			t.Fatalf("CheckCurrent() = %v", err)
		}
		if obs.Baseline.GeneratedAt != meta.GeneratedAt || len(obs.Findings) != 1 {
			t.Errorf("CheckCurrent() returned %+v / %+v, want the observation and its baseline", obs.Baseline, meta.GeneratedAt)
		}
	})

	tests := []struct {
		name   string
		setup  func(t *testing.T) string
		domain string
		want   error
	}{
		{"no observation", func(t *testing.T) string {
			dir := t.TempDir()
			writeBaselineMetadata(t, dir, baseline(nil))
			return dir
		}, "", ErrNoObservation},
		{"superseded", func(t *testing.T) string {
			dir := driftedTenant(t)
			meta := baseline(nil)
			meta.GeneratedAt = "2026-09-02T00:00:00Z"
			writeBaselineMetadata(t, dir, meta)
			return dir
		}, "", ErrObservationSuperseded},
		{"tenant mismatch", driftedTenant, "other.onmicrosoft.com", docs.ErrTenantMismatch},
		{"no baseline", func(t *testing.T) string {
			dir := driftedTenant(t)
			if err := os.Remove(filepath.Join(dir, "resources", docs.MetadataFileName)); err != nil {
				t.Fatal(err)
			}
			return dir
		}, "", docs.ErrNoMetadata},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := CheckCurrent(tt.setup(t), tt.domain); !errors.Is(err, tt.want) {
				t.Errorf("CheckCurrent() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestWriteObservationReturnsTheStampedObservation(t *testing.T) {
	tenantDir := t.TempDir()
	rep := &Report{Observation: Observation{Findings: map[string]Finding{}}, PayloadData: map[string][]byte{}}
	at := time.Date(2026, 9, 30, 10, 0, 0, 0, time.FixedZone("CEST", 2*3600))

	for _, dryRun := range []bool{true, false} {
		written, _, err := WriteObservation(tenantDir, rep, at, "v9", dryRun)
		if err != nil {
			t.Fatalf("WriteObservation(dryRun=%v) = %v", dryRun, err)
		}
		if written.ObservedAt != "2026-09-30T08:00:00Z" || written.ToolVersion != "v9" {
			t.Errorf("dryRun=%v: returned %q/%q, want the stamped UTC time and version", dryRun, written.ObservedAt, written.ToolVersion)
		}
	}
	onDisk, err := LoadObservation(tenantDir)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.ObservedAt != "2026-09-30T08:00:00Z" {
		t.Errorf("on disk observedAt = %q, want the returned value", onDisk.ObservedAt)
	}
}

func TestWriteAttribution(t *testing.T) {
	t.Run("refuses without a drift tree", func(t *testing.T) {
		tenantDir := t.TempDir()
		if _, err := WriteAttribution(tenantDir, &Attribution{}, false); !errors.Is(err, ErrNoObservation) {
			t.Errorf("WriteAttribution() = %v, want ErrNoObservation", err)
		}
		if _, err := os.Stat(filepath.Join(tenantDir, DriftDirName)); !os.IsNotExist(err) {
			t.Error("WriteAttribution must never create the drift tree")
		}
	})

	t.Run("dry run writes nothing", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		path, err := WriteAttribution(tenantDir, attributionFor(t, tenantDir, nil), true)
		if err != nil || path != AuditPath(tenantDir) {
			t.Fatalf("WriteAttribution(dry) = %q, %v", path, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Error("a dry run wrote audit.yaml")
		}
	})

	t.Run("written beside the observation, swept by the next observation", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		a := attributionFor(t, tenantDir, map[string]AttributionFinding{
			testType + "/b.yaml": {Status: AttributionNoEventInWindow, Table: TableAuditLogs, Events: []AttributionEvent{}},
		})
		path, err := WriteAttribution(tenantDir, a, false)
		if err != nil {
			t.Fatalf("WriteAttribution() = %v", err)
		}
		if filepath.Dir(path) != filepath.Join(tenantDir, DriftDirName) || filepath.Base(path) != AuditFileName {
			t.Errorf("path = %q, want drift/audit.yaml", path)
		}
		if _, err := os.Stat(ObservationPath(tenantDir)); err != nil {
			t.Error("writing the attribution disturbed the observation")
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0644 {
			t.Errorf("audit.yaml stat = %v, %v; want 0644", info, err)
		}

		loaded, err := LoadAttribution(tenantDir)
		if err != nil || loaded.Counts.NoEventInWindow != 1 || !loaded.Matches(mustObservation(t, tenantDir)) {
			t.Errorf("LoadAttribution() = %+v, %v; want the written, current attribution", loaded, err)
		}

		// A new drift run clears and rebuilds the tree, sweeping audit.yaml.
		rep := &Report{Observation: Observation{Findings: map[string]Finding{}}, PayloadData: map[string][]byte{}}
		if _, _, err := WriteObservation(tenantDir, rep, time.Now(), "test", false); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadAttribution(tenantDir); !errors.Is(err, ErrNoAttribution) {
			t.Errorf("after a new observation LoadAttribution() = %v, want ErrNoAttribution", err)
		}
	})
}

// TestAttributionTimestampsAreQuotedStrings pins the contract's timestamp
// shape: quoted RFC3339 UTC strings, never YAML timestamps.
func TestAttributionTimestampsAreQuotedStrings(t *testing.T) {
	a := &Attribution{
		Version: AttributionVersion, ObservedAt: "2026-09-30T10:00:00Z", QueriedAt: "2026-09-30T12:00:00Z",
		Window: AttributionWindow{From: "2026-09-28T00:00:00Z", To: "2026-09-30T10:00:00Z"},
		Tables: map[string]TableStatus{TableAuditLogs: {Status: TableStatusOK, Earliest: "2026-01-01T00:00:00Z"}},
		Findings: map[string]AttributionFinding{"t/a.yaml": {Status: AttributionMatched, Table: TableAuditLogs,
			Events: []AttributionEvent{{At: "2026-09-29T09:00:00Z", Actor: "u@x", ActorType: "user", Result: "success"}}}},
	}
	data, err := yaml.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`observedAt: "2026-09-30T10:00:00Z"`, `from: "2026-09-28T00:00:00Z"`, `earliest: "2026-01-01T00:00:00Z"`, `at: "2026-09-29T09:00:00Z"`, `queriedAt: "2026-09-30T12:00:00Z"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("marshalled attribution lacks %s:\n%s", want, data)
		}
	}
}

func TestLoadAttributionAndMatches(t *testing.T) {
	tenantDir := driftedTenant(t)
	if _, err := LoadAttribution(tenantDir); !errors.Is(err, ErrNoAttribution) {
		t.Fatalf("LoadAttribution() = %v, want ErrNoAttribution when absent", err)
	}
	if err := os.WriteFile(AuditPath(tenantDir), []byte("version: [\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAttribution(tenantDir); err == nil || errors.Is(err, ErrNoAttribution) {
		t.Errorf("LoadAttribution(broken) = %v, want a parse error", err)
	}

	obs := mustObservation(t, tenantDir)
	a := &Attribution{ObservedAt: obs.ObservedAt, BaselineGeneratedAt: obs.Baseline.GeneratedAt}
	if !a.Matches(obs) {
		t.Error("Matches() = false for the observation it attributes")
	}
	a.ObservedAt = "2020-01-01T00:00:00Z"
	if a.Matches(obs) {
		t.Error("Matches() = true for another observation time")
	}
	a.ObservedAt = obs.ObservedAt
	a.BaselineGeneratedAt = "2020-01-01T00:00:00Z"
	if a.Matches(obs) {
		t.Error("Matches() = true for another baseline")
	}
}

// TestAnalyzePromptSplicesAttribution covers the three states of audit.yaml:
// current (actors per finding), absent, and outdated (never used).
func TestAnalyzePromptSplicesAttribution(t *testing.T) {
	key := testType + "/b.yaml"
	render := func(t *testing.T, tenantDir string) string {
		t.Helper()
		out := filepath.Join(t.TempDir(), "analyze.md")
		if _, err := GenerateAnalyzePrompt(AnalyzeOptions{TenantDir: tenantDir, Template: analyzeTemplate, OutPath: out}); err != nil {
			t.Fatalf("GenerateAnalyzePrompt() = %v", err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	t.Run("absent", func(t *testing.T) {
		s := render(t, driftedTenant(t))
		if !strings.Contains(s, "- Attribution: none (no drift/audit.yaml — configure audit-workspace-id or run 'azure-rd resource audit')") {
			t.Errorf("prompt lacks the none line:\n%s", s)
		}
		if strings.Contains(s, "Changed by") || strings.Contains(s, "Attribution unavailable") {
			t.Error("no per-finding attribution lines without an audit.yaml")
		}
	})

	t.Run("current", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		a := attributionFor(t, tenantDir, map[string]AttributionFinding{key: {
			Status: AttributionMatched, Table: TableAuditLogs, Events: []AttributionEvent{
				{At: "2026-09-29T10:00:00Z", Actor: "jane@x", ActorType: "user", Activity: "Update group", Result: "success", CorrelationID: "c2"},
				{At: "2026-09-29T09:00:00Z", Actor: "Pipeline", ActorType: "application", Activity: "Add member", Result: "failure", CorrelationID: "c1"},
			}}})
		if _, err := WriteAttribution(tenantDir, a, false); err != nil {
			t.Fatal(err)
		}
		s := render(t, tenantDir)
		for _, want := range []string{
			"- Attribution: workspace `0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b`, queried at `2026-09-30T12:00:00Z`; tables: `AuditLogs` ok (earliest row `2026-01-01T00:00:00Z`) · `IntuneAuditLogs` failed (permission denied)",
			"  - Findings: matched 1 · no event in window 0",
			"ingestion lag",
			"- Changed by: jane@x (user) at `2026-09-29T10:00:00Z` — Update group, success, correlation `c2`\n- Changed by: Pipeline (application)",
		} {
			if !strings.Contains(s, want) {
				t.Errorf("prompt lacks %q:\n%s", want, s)
			}
		}
		if again := render(t, tenantDir); again != s {
			t.Error("the prompt must be deterministic with an attribution present")
		}

		a.Findings[key] = AttributionFinding{Status: AttributionRetentionExceeded, Table: TableAuditLogs, Reason: "table has no rows", Events: []AttributionEvent{}}
		if _, err := WriteAttribution(tenantDir, a, false); err != nil {
			t.Fatal(err)
		}
		if s := render(t, tenantDir); !strings.Contains(s, "- Attribution unavailable (retention-exceeded: table has no rows)") {
			t.Errorf("prompt lacks the unavailable line:\n%s", s)
		}
	})

	t.Run("outdated", func(t *testing.T) {
		tenantDir := driftedTenant(t)
		a := attributionFor(t, tenantDir, map[string]AttributionFinding{key: {Status: AttributionMatched, Table: TableAuditLogs,
			Events: []AttributionEvent{{At: "2026-09-29T10:00:00Z", Actor: "jane@x", ActorType: "user"}}}})
		a.ObservedAt = "2020-01-01T00:00:00Z"
		if _, err := WriteAttribution(tenantDir, a, false); err != nil {
			t.Fatal(err)
		}
		s := render(t, tenantDir)
		if !strings.Contains(s, "- Attribution: outdated (belongs to observation `2020-01-01T00:00:00Z`)") {
			t.Errorf("prompt lacks the outdated line:\n%s", s)
		}
		if strings.Contains(s, "jane@x") {
			t.Error("an outdated attribution must never be used")
		}
	})
}

func TestDefaultTemplateNamesAuditYAMLAsReadOnly(t *testing.T) {
	s := string(DefaultAnalyzeTemplate())
	if !strings.Contains(s, "`drift/audit.yaml`") || !strings.Contains(s, "2b'.") {
		t.Error("the template must name drift/audit.yaml as never-touch and carry the 2b' attribution note")
	}
}

func mustObservation(t *testing.T, tenantDir string) Observation {
	t.Helper()
	obs, err := LoadObservation(tenantDir)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}
