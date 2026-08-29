package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure-resource-downloader/internal/models"
)

// runWriter feeds the given transform results through a Writer and returns the
// write results. It closes the input channel so the writer's worker pool
// terminates, then drains every emitted result.
func runWriter(w *Writer, results []*models.TransformResult) []*models.WriteResult {
	in := make(chan *models.TransformResult)
	go func() {
		defer close(in)
		for _, r := range results {
			in <- r
		}
	}()

	out := w.Write(context.Background(), in)
	var got []*models.WriteResult
	for wr := range out {
		got = append(got, wr)
	}
	return got
}

// pathByResourceID indexes write results by the resource id recorded in their
// facts, so assertions do not depend on the (non-deterministic) order the
// concurrent write workers emit results.
func pathByResourceID(results []*models.WriteResult) map[string]string {
	out := map[string]string{}
	for _, wr := range results {
		if wr.Facts != nil {
			out[wr.Facts.ResourceID] = wr.YAMLPath
		}
	}
	return out
}

// TestWriterDisambiguatesCollidingNames guards against the filename-collision
// data-loss bug: two resources of the same type whose display names sanitize to
// the same string must be written to distinct files (and distinct metadata
// keys), never silently overwriting one another. This mirrors real Intune data
// where, e.g., several default deviceEnrollmentConfigurations share the display
// name "All users and all devices". The bare name goes to the lowest resource
// id deterministically, not to whichever resource is written first.
func TestWriterDisambiguatesCollidingNames(t *testing.T) {
	const (
		typ  = "Microsoft.Graph/deviceEnrollmentConfigurations"
		name = "all_users_and_all_devices"
		// "...DefaultPlatformRestrictions" < "...DefaultWindowsHelloForBusiness"
		// lexically, so PlatformRestrictions is the deterministic bare-name owner.
		idPR   = "025ab80e_DefaultPlatformRestrictions"
		idWHfB = "025ab80e_DefaultWindowsHelloForBusiness"
	)
	newResults := func() []*models.TransformResult {
		return []*models.TransformResult{
			{ResourceID: idWHfB, ResourceType: typ, DisplayName: "All users and all devices", SanitizedName: name, CleanedData: map[string]interface{}{"id": idWHfB}},
			{ResourceID: idPR, ResourceType: typ, DisplayName: "All users and all devices", SanitizedName: name, CleanedData: map[string]interface{}{"id": idPR}},
		}
	}

	dir := t.TempDir()
	w := NewWriter(dir, 4, false, false)
	got := runWriter(w, newResults())
	if len(got) != 2 {
		t.Fatalf("got %d write results, want 2", len(got))
	}
	for _, wr := range got {
		if wr.Error != nil {
			t.Fatalf("unexpected write error: %v", wr.Error)
		}
	}

	byID := pathByResourceID(got)
	wantBare := filepath.Join(dir, models.ResourcesDirName, typ, name+".yaml")
	wantSuffixed := filepath.Join(dir, models.ResourcesDirName, typ, name+"_"+nameDiscriminator(idWHfB)+".yaml")
	if byID[idPR] != wantBare {
		t.Errorf("lowest-id resource path = %q, want bare %q", byID[idPR], wantBare)
	}
	if byID[idWHfB] != wantSuffixed {
		t.Errorf("higher-id resource path = %q, want suffixed %q", byID[idWHfB], wantSuffixed)
	}

	// Both files must exist on disk with the correct resource's content.
	assertFileContains(t, byID[idPR], "DefaultPlatformRestrictions")
	assertFileContains(t, byID[idWHfB], "DefaultWindowsHelloForBusiness")
}

// TestWriterCollisionNamingIsOrderIndependent feeds the same colliding resources
// in opposite orders and asserts the resulting resource-id -> file-name mapping
// is identical. This is the property that fixes the run-to-run flip: the bare
// name no longer depends on which resource the concurrent pipeline delivered
// first.
func TestWriterCollisionNamingIsOrderIndependent(t *testing.T) {
	const typ = "Microsoft.Graph/groups"
	mk := func(id string) *models.TransformResult {
		return &models.TransformResult{ResourceID: id, ResourceType: typ, DisplayName: "Team", SanitizedName: "team", CleanedData: map[string]interface{}{"id": id}}
	}
	forward := []*models.TransformResult{mk("id-a"), mk("id-b"), mk("id-c")}
	reverse := []*models.TransformResult{mk("id-c"), mk("id-b"), mk("id-a")}

	rel := func(dir string, results []*models.WriteResult) map[string]string {
		out := map[string]string{}
		for id, p := range pathByResourceID(results) {
			r, err := filepath.Rel(dir, p)
			if err != nil {
				t.Fatalf("Rel: %v", err)
			}
			out[id] = r
		}
		return out
	}

	dir1 := t.TempDir()
	got1 := rel(dir1, runWriter(NewWriter(dir1, 4, false, false), forward))
	dir2 := t.TempDir()
	got2 := rel(dir2, runWriter(NewWriter(dir2, 4, false, false), reverse))

	if len(got1) != 3 || len(got2) != 3 {
		t.Fatalf("expected 3 mappings each, got %d and %d", len(got1), len(got2))
	}
	for id, p := range got1 {
		if got2[id] != p {
			t.Errorf("resource %q: forward path %q != reverse path %q (naming not order-independent)", id, p, got2[id])
		}
	}
	// The lowest id keeps the bare name regardless of feed order.
	if !strings.HasSuffix(got1["id-a"], "/team.yaml") {
		t.Errorf("lowest id must own the bare name, got %q", got1["id-a"])
	}
}

// TestWriterCollisionIsLosslessUnderConcurrency verifies that with many workers
// (non-deterministic arrival order) every colliding resource is still written
// to a unique file — the core guarantee is no data loss, regardless of which
// sibling happens to claim the bare name.
func TestWriterCollisionIsLosslessUnderConcurrency(t *testing.T) {
	dir := t.TempDir()
	w := NewWriter(dir, 8, false, false)

	const n = 20
	results := make([]*models.TransformResult, 0, n)
	for i := 0; i < n; i++ {
		id := "id-" + string(rune('a'+i))
		results = append(results, &models.TransformResult{
			ResourceID:    id,
			ResourceType:  "Microsoft.Graph/mobileApps",
			DisplayName:   "3CX",
			SanitizedName: "resource_3cx",
			CleanedData:   map[string]interface{}{"id": id},
		})
	}

	got := runWriter(w, results)
	if len(got) != n {
		t.Fatalf("got %d write results, want %d", len(got), n)
	}

	paths := map[string]bool{}
	ids := map[string]bool{}
	for _, wr := range got {
		if wr.Error != nil {
			t.Fatalf("unexpected write error: %v", wr.Error)
		}
		if paths[wr.YAMLPath] {
			t.Fatalf("duplicate path written: %q", wr.YAMLPath)
		}
		paths[wr.YAMLPath] = true
		if _, err := os.Stat(wr.YAMLPath); err != nil {
			t.Fatalf("expected file to exist: %q (%v)", wr.YAMLPath, err)
		}
		if wr.Facts != nil {
			ids[wr.Facts.ResourceID] = true
		}
	}
	if len(paths) != n {
		t.Errorf("got %d distinct paths, want %d (a collision overwrote a sibling)", len(paths), n)
	}
	if len(ids) != n {
		t.Errorf("got %d distinct resource ids in facts, want %d", len(ids), n)
	}
}

// TestReserveFileName exercises the reservation logic directly: the first claim
// keeps the sanitized name, later claims get a deterministic per-id suffix, and
// distinct types never interfere.
func TestReserveFileName(t *testing.T) {
	w := NewWriter(t.TempDir(), 1, false, false)

	const typ = "Microsoft.Graph/groups"
	first := w.reserveFileName(typ, "team", "id-1")
	if first != "team" {
		t.Errorf("first reservation = %q, want %q", first, "team")
	}

	second := w.reserveFileName(typ, "team", "id-2")
	wantSecond := "team_" + nameDiscriminator("id-2")
	if second != wantSecond {
		t.Errorf("second reservation = %q, want %q", second, wantSecond)
	}

	// A reservation is deterministic for a given id but never returns an
	// already-used name, so re-reserving the same colliding id yields a
	// counter-suffixed variant rather than overwriting.
	third := w.reserveFileName(typ, "team", "id-2")
	if third == second {
		t.Errorf("re-reserving the same id must not return an already-used name %q", third)
	}

	// The same sanitized name under a different type is independent.
	other := w.reserveFileName("Microsoft.Graph/mobileApps", "team", "id-1")
	if other != "team" {
		t.Errorf("reservation under a different type = %q, want %q", other, "team")
	}
}

// TestNameDiscriminatorDeterministic documents that the discriminator depends
// only on the resource ID, not on run order, so a resource maps to the same
// disambiguated name every run.
func TestNameDiscriminatorDeterministic(t *testing.T) {
	a := nameDiscriminator("some-resource-id")
	b := nameDiscriminator("some-resource-id")
	if a != b {
		t.Errorf("discriminator not stable: %q vs %q", a, b)
	}
	if len(a) != 8 {
		t.Errorf("discriminator length = %d, want 8", len(a))
	}
	if nameDiscriminator("other-id") == a {
		t.Errorf("different ids must not share a discriminator")
	}
}

func assertFileContains(t *testing.T, path, substr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %q: %v", path, err)
	}
	if !strings.Contains(string(data), substr) {
		t.Errorf("file %q does not contain %q; got:\n%s", path, substr, string(data))
	}
}
