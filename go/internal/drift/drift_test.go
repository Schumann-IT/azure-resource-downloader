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
	"azure-resource-downloader/internal/pipeline"

	"gopkg.in/yaml.v3"
)

const (
	testType    = "Microsoft.Graph/groups"
	testCfgSha  = "current-transform-config-sha"
	testFilters = "current-filters-sha"
	testTenant  = "contoso.onmicrosoft.com"
)

// mustMarshal returns the exact bytes a download would write for data and
// their hash, through the shared marshal path.
func mustMarshal(t *testing.T, data map[string]interface{}) ([]byte, string) {
	t.Helper()
	b, err := pipeline.MarshalResourceYAML(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b, sha256Hex(b)
}

// entry builds an attested, present baseline entry whose sourceSha256 matches
// data's marshalled bytes.
func entry(t *testing.T, id, displayName string, data map[string]interface{}) docs.ResourceMeta {
	t.Helper()
	_, sha := mustMarshal(t, data)
	return docs.ResourceMeta{
		ResourceId:            id,
		DisplayName:           displayName,
		SourceSha256:          sha,
		PresentInTenant:       true,
		TransformConfigSha256: testCfgSha,
	}
}

// baseline wraps entries into a comparable export metadata.
func baseline(entries map[string]docs.ResourceMeta) docs.Metadata {
	return docs.Metadata{
		GeneratedAt: "2026-09-01T00:00:00Z",
		Tenant:      testTenant,
		ToolVersion: "test",
		Run: docs.RunMeta{
			Complete:              true,
			TransformConfigSha256: testCfgSha,
			FiltersSha256:         testFilters,
		},
		Types:     map[string]docs.TypeMeta{},
		Resources: entries,
	}
}

// tr builds a writable transform result.
func tr(id, displayName, sanitized string, data map[string]interface{}) *models.TransformResult {
	return &models.TransformResult{
		ResourceID:    id,
		ResourceType:  testType,
		DisplayName:   displayName,
		SanitizedName: sanitized,
		CleanedData:   data,
	}
}

// compare runs Compare over a full-scope run whose every request produced a
// result.
func compare(meta docs.Metadata, results []*models.TransformResult) *Report {
	return Compare(Options{
		Baseline:               meta,
		CurrentTransformSha256: testCfgSha,
		TotalRequests:          len(results),
		Results:                results,
	})
}

func TestCompareVerdicts(t *testing.T) {
	unchangedData := map[string]interface{}{"id": "id-a", "displayName": "A"}
	changedOld := map[string]interface{}{"id": "id-b", "displayName": "B", "setting": "old"}
	changedNew := map[string]interface{}{"id": "id-b", "displayName": "B", "setting": "new"}
	renamedOld := map[string]interface{}{"id": "id-c", "displayName": "C old"}
	renamedNew := map[string]interface{}{"id": "id-c", "displayName": "C new"}
	removedData := map[string]interface{}{"id": "id-d", "displayName": "D"}
	addedData := map[string]interface{}{"id": "id-e", "displayName": "E"}

	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/a.yaml":     entry(t, "id-a", "A", unchangedData),
		testType + "/b.yaml":     entry(t, "id-b", "B", changedOld),
		testType + "/c_old.yaml": entry(t, "id-c", "C old", renamedOld),
		testType + "/d.yaml":     entry(t, "id-d", "D", removedData),
	})

	rep := compare(meta, []*models.TransformResult{
		tr("id-a", "A", "a", unchangedData),
		tr("id-b", "B", "b", changedNew),
		tr("id-c", "C new", "c_new", renamedNew),
		tr("id-e", "E", "e", addedData),
	})
	obs := rep.Observation

	want := Counts{Compared: 3, Unchanged: 1, Changed: 1, Renamed: 1, Added: 1, Removed: 1}
	if obs.Counts != want {
		t.Fatalf("counts = %+v, want %+v", obs.Counts, want)
	}
	if !rep.DriftFound {
		t.Error("DriftFound = false, want true")
	}
	if !obs.Run.Complete {
		t.Errorf("run not complete: %s", obs.Run.IncompleteReason)
	}

	// Changed: finding at the baseline key, both hashes recorded.
	changed, ok := obs.Findings[testType+"/b.yaml"]
	if !ok || changed.Verdict != VerdictChanged {
		t.Fatalf("no changed finding at baseline key: %+v", obs.Findings)
	}
	_, wantOldSha := mustMarshal(t, changedOld)
	_, wantNewSha := mustMarshal(t, changedNew)
	if changed.BaselineSha256 != wantOldSha || changed.PayloadSha256 != wantNewSha {
		t.Errorf("changed hashes = (%s, %s), want (%s, %s)", changed.BaselineSha256, changed.PayloadSha256, wantOldSha, wantNewSha)
	}
	if changed.DocPath != "docs/"+testType+"/b.md" {
		t.Errorf("changed docPath = %q, want the derived document path", changed.DocPath)
	}

	// Renamed: the payload lands at the NEW key and the finding carries both
	// keys, so old and new bytes stay joinable.
	renamed, ok := obs.Findings[testType+"/c_new.yaml"]
	if !ok || renamed.Verdict != VerdictRenamed {
		t.Fatalf("no renamed finding at the new key: %+v", obs.Findings)
	}
	if renamed.BaselineKey != testType+"/c_old.yaml" {
		t.Errorf("renamed baselineKey = %q, want the old key", renamed.BaselineKey)
	}
	if renamed.PreviousDisplayName != "C old" || renamed.DisplayName != "C new" {
		t.Errorf("renamed names = (%q, %q), want both old and new", renamed.PreviousDisplayName, renamed.DisplayName)
	}
	if _, ok := rep.PayloadData[testType+"/c_new.yaml"]; !ok {
		t.Error("renamed payload bytes missing at the new key")
	}

	// Added and removed.
	if f := obs.Findings[testType+"/e.yaml"]; f.Verdict != VerdictAdded || f.BaselineKey != "" {
		t.Errorf("added finding = %+v, want verdict added with no baseline key", f)
	}
	if f := obs.Findings[testType+"/d.yaml"]; f.Verdict != VerdictRemoved || f.ResourceID != "id-d" {
		t.Errorf("removed finding = %+v, want verdict removed for id-d", f)
	}
	if _, ok := rep.PayloadData[testType+"/d.yaml"]; ok {
		t.Error("a removal has no bytes to persist, but payload data was recorded")
	}

	// Payload list matches the payload data exactly.
	if len(obs.Payloads) != 3 || len(rep.PayloadData) != 3 {
		t.Errorf("payloads = %v (%d data entries), want exactly added+changed+renamed", obs.Payloads, len(rep.PayloadData))
	}
}

// TestCompareAddedPayloadRespectsBaselineNames guards the collision rule: an
// added resource whose name the baseline already holds must land at the
// discriminated path a subsequent download would choose, never at the baseline
// file's own path.
func TestCompareAddedPayloadRespectsBaselineNames(t *testing.T) {
	existing := map[string]interface{}{"id": "id-1", "displayName": "Team"}
	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/team.yaml": entry(t, "id-1", "Team", existing),
	})

	added := map[string]interface{}{"id": "id-2", "displayName": "Team"}
	rep := compare(meta, []*models.TransformResult{
		tr("id-1", "Team", "team", existing),
		tr("id-2", "Team", "team", added),
	})

	if rep.Observation.Counts.Added != 1 || rep.Observation.Counts.Unchanged != 1 {
		t.Fatalf("counts = %+v, want one unchanged and one added", rep.Observation.Counts)
	}
	for key, f := range rep.Observation.Findings {
		if f.Verdict != VerdictAdded {
			continue
		}
		if key == testType+"/team.yaml" {
			t.Errorf("added payload claimed the baseline's own path %q", key)
		}
		if !strings.HasPrefix(key, testType+"/team_") {
			t.Errorf("added payload key = %q, want a discriminated team_* name", key)
		}
	}
}

func TestCompareRemovalSuppressionAndCoverage(t *testing.T) {
	gone := map[string]interface{}{"id": "id-gone"}
	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/gone.yaml": entry(t, "id-gone", "Gone", gone),
	})

	t.Run("incomplete run suppresses removals", func(t *testing.T) {
		// The type IS covered (a non-cancelled result exists), but the run is
		// incomplete (a cancelled request): the removal must be suppressed and
		// said so, not silently absent.
		rep := Compare(Options{
			Baseline:               meta,
			CurrentTransformSha256: testCfgSha,
			TotalRequests:          2,
			Results: []*models.TransformResult{
				tr("id-x", "X", "x", map[string]interface{}{"id": "id-x"}),
				{ResourceID: "id-y", ResourceType: testType, Cancelled: true},
			},
		})
		if rep.Observation.Counts.Removed != 0 {
			t.Errorf("removed = %d, want 0 on an incomplete run", rep.Observation.Counts.Removed)
		}
		if !rep.Observation.RemovalsSuppressed {
			t.Error("RemovalsSuppressed = false, want true")
		}
		if rep.Observation.Run.Complete {
			t.Error("run reported complete despite a cancelled request")
		}
	})

	t.Run("uncovered type asserts nothing", func(t *testing.T) {
		// An unlistable type is unknown, not empty: no removals, and the type
		// is reported as unknown.
		rep := Compare(Options{
			Baseline:               meta,
			CurrentTransformSha256: testCfgSha,
			TotalRequests:          0,
			SkippedTypes:           []models.SkippedType{{ResourceType: testType, Reason: "403"}},
		})
		if rep.Observation.Counts.Removed != 0 {
			t.Errorf("removed = %d, want 0 for an unlistable type", rep.Observation.Counts.Removed)
		}
		if len(rep.Observation.UnknownTypes) != 1 || rep.Observation.UnknownTypes[0] != testType {
			t.Errorf("unknownTypes = %v, want [%s]", rep.Observation.UnknownTypes, testType)
		}
	})

	t.Run("id-scoped run covers nothing", func(t *testing.T) {
		rep := Compare(Options{
			Baseline:               meta,
			CurrentTransformSha256: testCfgSha,
			Scope:                  docs.RunScope{ResourceIDs: []string{"id-other"}},
			TotalRequests:          1,
			Results:                []*models.TransformResult{tr("id-other", "Other", "other", map[string]interface{}{"id": "id-other"})},
		})
		if rep.Observation.Counts.Removed != 0 {
			t.Errorf("removed = %d, want 0 for an id-scoped run", rep.Observation.Counts.Removed)
		}
	})

	t.Run("covered empty type asserts removal", func(t *testing.T) {
		rep := Compare(Options{
			Baseline:               meta,
			CurrentTransformSha256: testCfgSha,
			TotalRequests:          0,
			EmptyTypes:             []string{testType},
		})
		if rep.Observation.Counts.Removed != 1 {
			t.Errorf("removed = %d, want 1 when the type listed to empty", rep.Observation.Counts.Removed)
		}
	})
}

func TestCompareUnattestedEntriesAreNotDrift(t *testing.T) {
	data := map[string]interface{}{"id": "id-u", "displayName": "U", "setting": "old"}
	changed := map[string]interface{}{"id": "id-u", "displayName": "U", "setting": "new"}

	noHash := entry(t, "id-u", "U", data)
	noHash.TransformConfigSha256 = ""
	otherCfg := entry(t, "id-v", "V", data)
	otherCfg.TransformConfigSha256 = "some-older-config"
	noSource := entry(t, "id-w", "W", data)
	noSource.SourceSha256 = ""
	noSource.Skipped = true

	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/u.yaml": noHash,
		testType + "/v.yaml": otherCfg,
		testType + "/w.yaml": noSource,
	})

	rep := compare(meta, []*models.TransformResult{
		tr("id-u", "U", "u", changed),
		tr("id-v", "V", "v", changed),
		tr("id-w", "W", "w", changed),
	})
	obs := rep.Observation

	if obs.Counts.Unattested != 3 {
		t.Fatalf("unattested = %d, want 3", obs.Counts.Unattested)
	}
	if rep.DriftFound || len(obs.Findings) != 0 {
		t.Errorf("unattested entries produced findings: %+v", obs.Findings)
	}
	if len(obs.NotComparable) != 3 {
		t.Fatalf("notComparable = %+v, want 3 entries with reasons", obs.NotComparable)
	}
	for _, nc := range obs.NotComparable {
		if nc.Reason == "" {
			t.Errorf("notComparable entry %q has no reason", nc.Key)
		}
	}
	// A re-observed unattested entry is never a removal candidate either.
	if obs.Counts.Removed != 0 {
		t.Errorf("removed = %d, want 0", obs.Counts.Removed)
	}
}

func TestCompareExclusionsProtectAgainstFalseRemovals(t *testing.T) {
	dataA := map[string]interface{}{"id": "id-a"}
	dataB := map[string]interface{}{"id": "id-b"}

	filteredEntry := entry(t, "id-f", "F", dataA)
	filteredEntry.Filtered = true
	skippedEntry := entry(t, "id-s", "S", dataB)
	skippedEntry.Skipped = true

	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/a.yaml": entry(t, "id-a", "A", dataA),
		testType + "/b.yaml": entry(t, "id-b", "B", dataB),
		testType + "/f.yaml": filteredEntry,
		testType + "/s.yaml": skippedEntry,
	})

	// id-a is skipped (permissions) and id-b is filtered this run: both are
	// re-observed, so neither may be reported as removed. The baseline's own
	// filtered/skipped entries are excluded from removal outright.
	rep := compare(meta, []*models.TransformResult{
		{ResourceID: "id-a", ResourceType: testType, Skipped: true, SkipReason: "403"},
		{ResourceID: "id-b", ResourceType: testType, Filtered: true},
	})
	obs := rep.Observation

	if obs.Counts.Excluded != 2 {
		t.Errorf("excluded = %d, want 2", obs.Counts.Excluded)
	}
	if obs.Counts.Removed != 0 {
		t.Errorf("removed = %d, want 0 (skipped/filtered must never read as removed)", obs.Counts.Removed)
	}
	if rep.DriftFound {
		t.Error("DriftFound = true, want false")
	}
}

func TestCompareFailedFetchIsNotARemoval(t *testing.T) {
	data := map[string]interface{}{"id": "id-a"}
	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/a.yaml": entry(t, "id-a", "A", data),
	})

	rep := compare(meta, []*models.TransformResult{
		{ResourceID: "id-a", ResourceType: testType, Error: errors.New("boom")},
	})
	obs := rep.Observation

	if obs.Counts.Failed != 1 {
		t.Errorf("failed = %d, want 1", obs.Counts.Failed)
	}
	if obs.Counts.Removed != 0 {
		t.Errorf("removed = %d, want 0 (a failed fetch is unknown, not absent)", obs.Counts.Removed)
	}
}

func TestCompareChangedDeltasFromBaselineFile(t *testing.T) {
	oldData := map[string]interface{}{"id": "id-b", "displayName": "B", "setting": "old"}
	newData := map[string]interface{}{"id": "id-b", "displayName": "B", "setting": "new"}

	// Put the baseline YAML on disk where the export holds it.
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	oldBytes, _ := mustMarshal(t, oldData)
	if err := os.MkdirAll(filepath.Join(resourcesDir, testType), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resourcesDir, testType, "b.yaml"), oldBytes, 0644); err != nil {
		t.Fatal(err)
	}

	meta := baseline(map[string]docs.ResourceMeta{
		testType + "/b.yaml": entry(t, "id-b", "B", oldData),
	})

	rep := Compare(Options{
		Baseline:               meta,
		ResourcesDir:           resourcesDir,
		CurrentTransformSha256: testCfgSha,
		TotalRequests:          1,
		Results:                []*models.TransformResult{tr("id-b", "B", "b", newData)},
	})

	f := rep.Observation.Findings[testType+"/b.yaml"]
	if f.Verdict != VerdictChanged {
		t.Fatalf("verdict = %q, want changed", f.Verdict)
	}
	if len(f.Deltas) != 1 || f.Deltas[0].Path != "setting" || f.Deltas[0].Old != "old" || f.Deltas[0].New != "new" {
		t.Errorf("deltas = %+v, want [setting: old -> new]", f.Deltas)
	}
	if f.DeltaNote != "" {
		t.Errorf("deltaNote = %q, want empty", f.DeltaNote)
	}
}

// TestCompareDeterministic guards the artifact's byte stability: the same
// inputs must produce an identical observation (and findings hash) apart from
// the moment of observation, so "the same drift as last time" is a single
// comparison.
func TestCompareDeterministic(t *testing.T) {
	build := func() []byte {
		oldData := map[string]interface{}{"id": "id-b", "setting": "old", "displayName": "B"}
		meta := baseline(map[string]docs.ResourceMeta{
			testType + "/b.yaml": entry(t, "id-b", "B", oldData),
			testType + "/d.yaml": entry(t, "id-d", "D", map[string]interface{}{"id": "id-d"}),
		})
		rep := compare(meta, []*models.TransformResult{
			tr("id-b", "B", "b", map[string]interface{}{"id": "id-b", "setting": "new", "displayName": "B"}),
			tr("id-e", "E", "e", map[string]interface{}{"id": "id-e"}),
		})
		rep.Observation.ObservedAt = "FIXED"
		data, err := yaml.Marshal(&rep.Observation)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return data
	}

	a, b := build(), build()
	if string(a) != string(b) {
		t.Errorf("two identical comparisons marshalled differently:\n%s\n---\n%s", a, b)
	}
}

func TestPreflight(t *testing.T) {
	comparable := baseline(nil)

	t.Run("comparable baseline passes without warnings", func(t *testing.T) {
		warnings, err := Preflight(comparable, testTenant, testCfgSha, testFilters)
		if err != nil || len(warnings) != 0 {
			t.Errorf("Preflight = (%v, %v), want clean pass", warnings, err)
		}
	})

	t.Run("tenant mismatch refuses", func(t *testing.T) {
		_, err := Preflight(comparable, "other.example.com", testCfgSha, testFilters)
		if !errors.Is(err, docs.ErrTenantMismatch) {
			t.Errorf("err = %v, want ErrTenantMismatch", err)
		}
	})

	t.Run("transform config mismatch refuses", func(t *testing.T) {
		_, err := Preflight(comparable, testTenant, "a-different-config", testFilters)
		if !errors.Is(err, ErrNotComparable) {
			t.Errorf("err = %v, want ErrNotComparable", err)
		}
	})

	t.Run("filter config mismatch refuses", func(t *testing.T) {
		_, err := Preflight(comparable, testTenant, testCfgSha, "different-filters")
		if !errors.Is(err, ErrNotComparable) {
			t.Errorf("err = %v, want ErrNotComparable", err)
		}
	})

	t.Run("missing attestations warn instead of refusing", func(t *testing.T) {
		old := baseline(nil)
		old.Run.TransformConfigSha256 = ""
		old.Run.FiltersSha256 = ""
		warnings, err := Preflight(old, testTenant, testCfgSha, testFilters)
		if err != nil {
			t.Fatalf("err = %v, want nil for a pre-field baseline", err)
		}
		if len(warnings) != 2 {
			t.Errorf("warnings = %v, want two (transform + filters unattested)", warnings)
		}
	})
}

func TestBase64FileModeWarning(t *testing.T) {
	if w := Base64FileModeWarning(models.DefaultTransformerConfigs()); w != "" {
		t.Errorf("default config warned: %q", w)
	}
	configs := []models.TransformerConfig{{
		Name:   string(models.TransformerBase64Decode),
		Config: map[string]interface{}{"mode": "file", "remove-source": true},
	}}
	if w := Base64FileModeWarning(configs); w == "" {
		t.Error("file mode with remove-source must warn that artifact drift is undetected")
	}
}

func TestWriteObservationOwnsItsTree(t *testing.T) {
	tenantDir := t.TempDir()
	driftDir := filepath.Join(tenantDir, DriftDirName)

	// Sibling trees that a drift run must never touch.
	resourceFile := filepath.Join(tenantDir, models.ResourcesDirName, testType, "a.yaml")
	docFile := filepath.Join(tenantDir, docs.DocsDirName, testType, "a.md")
	for _, p := range []string{resourceFile, docFile} {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("untouchable"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// A leftover payload from an earlier observation (e.g. a reverted
	// resource) must not survive the rebuild.
	leftover := filepath.Join(driftDir, testType, "reverted.yaml")
	if err := os.MkdirAll(filepath.Dir(leftover), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	rep := &Report{
		Observation: Observation{
			Tenant:       testTenant,
			UnknownTypes: []string{},
			Findings:     map[string]Finding{testType + "/b.yaml": {Verdict: VerdictChanged}},
			Payloads:     []string{testType + "/b.yaml"},
		},
		PayloadData: map[string][]byte{testType + "/b.yaml": []byte("new bytes")},
	}

	metaPath, err := WriteObservation(tenantDir, rep, time.Now(), "test", false)
	if err != nil {
		t.Fatalf("WriteObservation: %v", err)
	}

	// The tree holds exactly the current observation.
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Error("leftover payload from an earlier observation survived the rebuild")
	}
	payload, err := os.ReadFile(filepath.Join(driftDir, testType, "b.yaml"))
	if err != nil || string(payload) != "new bytes" {
		t.Errorf("payload = (%q, %v), want the current bytes", payload, err)
	}
	raw, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("observation metadata not written: %v", err)
	}
	var obs Observation
	if err := yaml.Unmarshal(raw, &obs); err != nil {
		t.Fatalf("observation metadata not parseable: %v", err)
	}
	if obs.ObservedAt == "" || obs.ToolVersion != "test" {
		t.Errorf("observation not self-describing: %+v", obs)
	}
	if len(obs.Payloads) != 1 || obs.Payloads[0] != testType+"/b.yaml" {
		t.Errorf("payload list = %v, want exactly the written payload", obs.Payloads)
	}

	// The sibling trees are untouched.
	for _, p := range []string{resourceFile, docFile} {
		data, err := os.ReadFile(p)
		if err != nil || string(data) != "untouchable" {
			t.Errorf("sibling tree file %q was touched: (%q, %v)", p, data, err)
		}
	}
}

func TestWriteObservationDryRunWritesNothingAndClearsNothing(t *testing.T) {
	tenantDir := t.TempDir()
	driftDir := filepath.Join(tenantDir, DriftDirName)

	// A previous observation stays intact under --dry-run.
	previous := filepath.Join(driftDir, ObservationFileName)
	if err := os.MkdirAll(driftDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(previous, []byte("previous observation"), 0644); err != nil {
		t.Fatal(err)
	}

	rep := &Report{
		Observation: Observation{Findings: map[string]Finding{}, Payloads: []string{testType + "/b.yaml"}},
		PayloadData: map[string][]byte{testType + "/b.yaml": []byte("new bytes")},
	}
	if _, err := WriteObservation(tenantDir, rep, time.Now(), "test", true); err != nil {
		t.Fatalf("WriteObservation dry-run: %v", err)
	}

	data, err := os.ReadFile(previous)
	if err != nil || string(data) != "previous observation" {
		t.Errorf("previous observation = (%q, %v), want it untouched", data, err)
	}
	if _, err := os.Stat(filepath.Join(driftDir, testType)); !os.IsNotExist(err) {
		t.Error("dry run wrote payloads")
	}
}
