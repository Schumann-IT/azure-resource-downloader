package docs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure-resource-downloader/internal/models"
)

// writeDoc writes a generated document with frontmatter for the given metadata
// key at its mirrored docs/ path.
func writeDoc(t *testing.T, tenantDir, key, srcSha, promptSha string) {
	t.Helper()
	docPath := filepath.Join(tenantDir, filepath.FromSlash(docRel(key)))
	if err := os.MkdirAll(filepath.Dir(docPath), 0755); err != nil {
		t.Fatalf("mkdir doc: %v", err)
	}
	fm := fmt.Sprintf("---\nsource: %s\nsourceSha256: %s\npromptSha256: %s\ngeneratedAt: 2026-01-01T00:00:00Z\n---\n# doc\n",
		srcRel(key), srcSha, promptSha)
	if err := os.WriteFile(docPath, []byte(fm), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

// writePromptFile writes a doc-prompt.md for a resource type.
func writePromptFile(t *testing.T, resourcesDir, rtype string) {
	t.Helper()
	dir := filepath.Join(resourcesDir, filepath.FromSlash(rtype))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir type: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, docPromptFileName), []byte("spec"), 0644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
}

func writeMeta(t *testing.T, tenantDir string, m *Metadata) {
	t.Helper()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	metaPath := filepath.Join(resourcesDir, MetadataFileName)
	if err := writeMetadata(metaPath, resourcesDir, m); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

func groupTarget(groupID string) map[string]interface{} {
	return map[string]interface{}{
		"target": map[string]interface{}{
			"@odata.type": "#microsoft.graph.groupAssignmentTarget",
			"groupId":     groupID,
		},
	}
}

func reasonsByDoc(items []WorkItem) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		out[it.DocPath] = it.Reason
	}
	return out
}

const compType = "Microsoft.Graph/deviceCompliancePolicies"

func TestGeneratePromptClassifiesStaleness(t *testing.T) {
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)

	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z",
		Tenant:      "example.com",
		Run:         RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType: {PromptSha256: "p-comp"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/alpha.yaml":               {ResourceId: "a", DisplayName: "alpha", SourceSha256: "s-alpha", PresentInTenant: true},
			compType + "/beta.yaml":                {ResourceId: "b", DisplayName: "beta", SourceSha256: "s-beta", PresentInTenant: true},
			compType + "/gamma.yaml":               {ResourceId: "g", DisplayName: "gamma", SourceSha256: "s-gamma", PresentInTenant: true},
			compType + "/delta.yaml":               {ResourceId: "d", DisplayName: "delta", SourceSha256: "s-delta", PresentInTenant: true},
			compType + "/eps.yaml":                 {ResourceId: "e", DisplayName: "eps", SourceSha256: "s-eps", PresentInTenant: false},
			autopilotIdentitiesType + "/dev1.yaml": {ResourceId: "dev1", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)

	// alpha: current. gamma: stale source. delta: stale prompt. eps: orphan doc.
	writeDoc(t, tenantDir, compType+"/alpha.yaml", "s-alpha", "p-comp")
	writeDoc(t, tenantDir, compType+"/gamma.yaml", "OLD", "p-comp")
	writeDoc(t, tenantDir, compType+"/delta.yaml", "s-delta", "OLD-PROMPT")
	writeDoc(t, tenantDir, compType+"/eps.yaml", "s-eps", "p-comp")
	// beta: no document at all.

	res, err := GeneratePrompt(GeneratePromptOptions{
		TenantDir: tenantDir,
		Template:  DefaultGeneratePromptTemplate(),
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}

	reasons := reasonsByDoc(res.ToGenerate)
	if len(reasons) != 3 {
		t.Fatalf("expected 3 to generate, got %d: %v", len(reasons), reasons)
	}
	wantContains := map[string]string{
		"docs/" + compType + "/beta.md":  "no document",
		"docs/" + compType + "/gamma.md": "resource changed",
		"docs/" + compType + "/delta.md": "spec",
	}
	for doc, sub := range wantContains {
		got, ok := reasons[doc]
		if !ok {
			t.Errorf("expected %s in work list", doc)
			continue
		}
		if !strings.Contains(got, sub) {
			t.Errorf("%s reason %q does not contain %q", doc, got, sub)
		}
	}
	if _, ok := reasons["docs/"+compType+"/alpha.md"]; ok {
		t.Error("alpha is current and must not be in the work list")
	}

	// eps is an orphan; the autopilot record is excluded entirely.
	if len(res.Orphans) != 1 || res.Orphans[0] != srcRel(compType+"/eps.yaml") {
		t.Errorf("expected one orphan (eps), got %v", res.Orphans)
	}
	for _, o := range res.Orphans {
		if strings.Contains(o, "windowsAutopilot") {
			t.Errorf("autopilot record must not be reported as orphan: %s", o)
		}
	}
}

func TestGeneratePromptReferencedGroups(t *testing.T) {
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)

	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z",
		Tenant:      "example.com",
		Run:         RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:   {PromptSha256: "p-comp"},
			groupsType: {PromptSha256: "p-grp"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId:      "pol",
				SourceSha256:    "s-pol",
				PresentInTenant: true,
				AssignmentTargets: []interface{}{
					groupTarget("G1"),
					groupTarget("G2-dangling"),
				},
			},
			groupsType + "/g1.yaml": {ResourceId: "G1", DisplayName: "Group One", SourceSha256: "s-g1", PresentInTenant: true},
			groupsType + "/g3.yaml": {ResourceId: "G3", DisplayName: "Unreferenced", SourceSha256: "s-g3", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, groupsType)
	// policy already has a current doc; G1 (referenced) has none -> generate.
	writeDoc(t, tenantDir, compType+"/policy.yaml", "s-pol", "p-comp")

	res, err := GeneratePrompt(GeneratePromptOptions{
		TenantDir: tenantDir,
		Template:  DefaultGeneratePromptTemplate(),
		DryRun:    false,
	})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}

	if res.ReferencedGroups != 2 {
		t.Errorf("ReferencedGroups = %d, want 2", res.ReferencedGroups)
	}
	if len(res.DanglingGroupIDs) != 1 || res.DanglingGroupIDs[0] != "G2-dangling" {
		t.Errorf("DanglingGroupIDs = %v, want [G2-dangling]", res.DanglingGroupIDs)
	}

	// Referenced group G1 has no document -> it is in list 1.
	reasons := reasonsByDoc(res.ToGenerate)
	if _, ok := reasons["docs/"+groupsType+"/g1.md"]; !ok {
		t.Errorf("referenced group G1 must be in the work list, got %v", reasons)
	}
	// Unreferenced group G3 must never appear.
	if _, ok := reasons["docs/"+groupsType+"/g3.md"]; ok {
		t.Error("unreferenced group G3 must be excluded")
	}

	// The written prompt must resolve G1 and flag G2 as dangling.
	out, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, "[Group One](docs/"+groupsType+"/g1.md)") {
		t.Error("refmap must link the resolved group")
	}
	if !strings.Contains(body, "G2-dangling` → ⚠️ not in export") {
		t.Error("refmap must flag the dangling group")
	}
}

// docFM describes the frontmatter (and optional assignment markers) to write
// into a generated document for list-2 tests.
type docFM struct {
	srcSha         string
	promptSha      string
	assignmentsSha string
	targetedBySha  string
	withMarkers    bool
}

// writeDocFM writes a document for a metadata key with the given frontmatter and
// optional assignment markers.
func writeDocFM(t *testing.T, tenantDir, key string, fm docFM) {
	t.Helper()
	docPath := filepath.Join(tenantDir, filepath.FromSlash(docRel(key)))
	if err := os.MkdirAll(filepath.Dir(docPath), 0755); err != nil {
		t.Fatalf("mkdir doc: %v", err)
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "source: %s\n", srcRel(key))
	fmt.Fprintf(&b, "sourceSha256: %s\n", fm.srcSha)
	fmt.Fprintf(&b, "promptSha256: %s\n", fm.promptSha)
	if fm.assignmentsSha != "" {
		fmt.Fprintf(&b, "assignmentsSha256: %s\n", fm.assignmentsSha)
	}
	if fm.targetedBySha != "" {
		fmt.Fprintf(&b, "targetedBySha256: %s\n", fm.targetedBySha)
	}
	b.WriteString("generatedAt: 2026-01-01T00:00:00Z\n---\n# doc\n")
	if fm.withMarkers {
		b.WriteString("\n<!-- assignments:start -->\ntable\n<!-- assignments:end -->\n")
	}
	if err := os.WriteFile(docPath, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
}

func forwardHash(m *Metadata, key string) string {
	return assignmentsSha256(parseAssignments(m.Resources[key].AssignmentTargets), buildGroupInfo(m), buildFilterInfo(m))
}

func reverseHash(m *Metadata, groupID string) string {
	return targetedBySha256(buildTargetedBy(m)[groupID], buildFilterInfo(m))
}

func docPaths(items []RespliceItem) map[string]RespliceItem {
	out := map[string]RespliceItem{}
	for _, it := range items {
		out[it.DocPath] = it
	}
	return out
}

// assignmentScenario builds a metadata + tree with one assignment-capable
// policy targeting a present group G1, both with current source/prompt docs. It
// returns the tenant dir and metadata so tests can vary the doc frontmatter.
func assignmentScenario(t *testing.T) (string, *Metadata) {
	t.Helper()
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:   {PromptSha256: "p-comp", HasAssignments: true},
			groupsType: {PromptSha256: "p-grp"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: "pol", DisplayName: "Policy", SourceSha256: "s-pol", PresentInTenant: true,
				AssignmentTargets: []interface{}{groupTarget("G1")},
			},
			groupsType + "/g1.yaml": {
				ResourceId: "G1", DisplayName: "Group One", SourceSha256: "s-g1", PresentInTenant: true,
				GroupTypes: []string{"DynamicMembership"}, SecurityEnabled: boolPtr(true),
			},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, groupsType)
	// Keep the group document current and correctly reverse-hashed so the group
	// itself never lands in a list unless a test intends it to.
	writeDocFM(t, tenantDir, groupsType+"/g1.yaml", docFM{srcSha: "s-g1", promptSha: "p-grp", targetedBySha: reverseHash(m, "G1")})
	return tenantDir, m
}

func TestGeneratePromptForwardResplice(t *testing.T) {
	tenantDir, m := assignmentScenario(t)
	// Policy is current (source/prompt match) with markers, but its recorded
	// assignmentsSha256 is stale.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", assignmentsSha: "STALE", withMarkers: true})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.ToGenerate) != 0 {
		t.Errorf("current policy must not be in list 1: %+v", res.ToGenerate)
	}
	if len(res.Migrate) != 0 {
		t.Errorf("policy has markers, must not migrate: %+v", res.Migrate)
	}
	fwd := docPaths(res.ForwardResplice)
	it, ok := fwd["docs/"+compType+"/policy.md"]
	if !ok {
		t.Fatalf("policy must be in ForwardResplice, got %+v", res.ForwardResplice)
	}
	if it.Hash != forwardHash(m, compType+"/policy.yaml") {
		t.Errorf("forward hash = %q, want %q", it.Hash, forwardHash(m, compType+"/policy.yaml"))
	}
	if len(res.ReverseResplice) != 0 {
		t.Errorf("group is correctly hashed, must not reverse-resplice: %+v", res.ReverseResplice)
	}

	// Rewriting the doc with the correct hash clears the forward re-splice.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", assignmentsSha: forwardHash(m, compType+"/policy.yaml"), withMarkers: true})
	res, err = GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt (rerun): %v", err)
	}
	if len(res.ForwardResplice) != 0 {
		t.Errorf("matching hash must clear ForwardResplice: %+v", res.ForwardResplice)
	}
}

func TestGeneratePromptMigrate(t *testing.T) {
	tenantDir, m := assignmentScenario(t)
	// Policy is current but predates markers: it has an assignments table (here
	// just a body) and no <!-- assignments:start --> marker.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", withMarkers: false})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.ForwardResplice) != 0 {
		t.Errorf("a marker-less doc migrates rather than re-splices: %+v", res.ForwardResplice)
	}
	if len(res.Migrate) != 1 || res.Migrate[0].DocPath != "docs/"+compType+"/policy.md" {
		t.Fatalf("policy must be in Migrate, got %+v", res.Migrate)
	}
	if res.Migrate[0].AssignmentsSha256 != forwardHash(m, compType+"/policy.yaml") {
		t.Errorf("migrate item must carry the forward hash to write after migration")
	}
	if !res.HasPendingWork() {
		t.Error("a migrate-only run still has pending work")
	}
}

func TestGeneratePromptReverseResplice(t *testing.T) {
	tenantDir, m := assignmentScenario(t)
	// Policy is fully current (source/prompt/assignments all match).
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", assignmentsSha: forwardHash(m, compType+"/policy.yaml"), withMarkers: true})
	// The group document's reverse hash is stale.
	writeDocFM(t, tenantDir, groupsType+"/g1.yaml", docFM{srcSha: "s-g1", promptSha: "p-grp", targetedBySha: "STALE"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.ToGenerate) != 0 || len(res.ForwardResplice) != 0 {
		t.Errorf("only the group's reverse block is stale: gen=%+v fwd=%+v", res.ToGenerate, res.ForwardResplice)
	}
	rev := docPaths(res.ReverseResplice)
	it, ok := rev["docs/"+groupsType+"/g1.md"]
	if !ok {
		t.Fatalf("group must be in ReverseResplice, got %+v", res.ReverseResplice)
	}
	if it.Hash != reverseHash(m, "G1") {
		t.Errorf("reverse hash = %q, want %q", it.Hash, reverseHash(m, "G1"))
	}
}

func TestGeneratePromptList1ExcludedFromList2(t *testing.T) {
	tenantDir, m := assignmentScenario(t)
	// Policy source moved: it is regenerated wholesale, so it must NOT also be a
	// forward re-splice candidate, and its work-list row carries the new hash.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "OLD", promptSha: "p-comp", assignmentsSha: "STALE", withMarkers: true})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	gen := reasonsByDoc(res.ToGenerate)
	if _, ok := gen["docs/"+compType+"/policy.md"]; !ok {
		t.Fatalf("stale-source policy must be in list 1: %+v", res.ToGenerate)
	}
	if len(res.ForwardResplice) != 0 {
		t.Errorf("a list-1 document must not also appear in list 2: %+v", res.ForwardResplice)
	}
	var item WorkItem
	for _, it := range res.ToGenerate {
		if it.DocPath == "docs/"+compType+"/policy.md" {
			item = it
		}
	}
	if item.AssignmentsSha256 != forwardHash(m, compType+"/policy.yaml") {
		t.Errorf("list-1 assignment-capable item must carry the forward hash, got %q", item.AssignmentsSha256)
	}
}

func TestGeneratePromptDanglingFilter(t *testing.T) {
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:              {PromptSha256: "p-comp", HasAssignments: true},
			assignmentFiltersType: {PromptSha256: "p-flt"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: "pol", SourceSha256: "s-pol", PresentInTenant: true,
				AssignmentTargets: []interface{}{
					groupTargetWithFilter("", "F-present", "include"),
					groupTargetWithFilter("", "F-missing", "exclude"),
					groupTargetWithFilter("", noFilterSentinel, "none"),
				},
			},
			assignmentFiltersType + "/f1.yaml": {ResourceId: "F-present", DisplayName: "Present Filter", SourceSha256: "s-f1", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, assignmentFiltersType)
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", assignmentsSha: forwardHash(m, compType+"/policy.yaml"), withMarkers: true})
	writeDocFM(t, tenantDir, assignmentFiltersType+"/f1.yaml", docFM{srcSha: "s-f1", promptSha: "p-flt"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	// Only F-missing is dangling; the sentinel and the present filter are not.
	if len(res.DanglingFilterIDs) != 1 || res.DanglingFilterIDs[0] != "F-missing" {
		t.Errorf("DanglingFilterIDs = %v, want [F-missing]", res.DanglingFilterIDs)
	}
}

func TestGeneratePromptTenantMismatch(t *testing.T) {
	tenantDir := t.TempDir()
	m := &Metadata{Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{}, Resources: map[string]ResourceMeta{}}
	writeMeta(t, tenantDir, m)

	_, err := GeneratePrompt(GeneratePromptOptions{
		TenantDir:    tenantDir,
		ExpectDomain: "other.com",
		Template:     DefaultGeneratePromptTemplate(),
	})
	if !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("expected ErrTenantMismatch, got %v", err)
	}
}

func TestGeneratePromptNoMetadata(t *testing.T) {
	_, err := GeneratePrompt(GeneratePromptOptions{
		TenantDir: t.TempDir(),
		Template:  DefaultGeneratePromptTemplate(),
	})
	if !errors.Is(err, ErrNoMetadata) {
		t.Fatalf("expected ErrNoMetadata, got %v", err)
	}
}

func TestGeneratePromptPromptMissingType(t *testing.T) {
	tenantDir := t.TempDir()
	m := &Metadata{
		Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{compType: {PromptSha256: "p-comp"}},
		Resources: map[string]ResourceMeta{
			compType + "/alpha.yaml": {ResourceId: "a", SourceSha256: "s-a", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	// No doc-prompt.md written for compType.

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.ToGenerate) != 0 {
		t.Errorf("no document should be generatable without a prompt file, got %v", res.ToGenerate)
	}
	if len(res.PromptMissingTypes) != 1 || res.PromptMissingTypes[0] != compType {
		t.Errorf("PromptMissingTypes = %v, want [%s]", res.PromptMissingTypes, compType)
	}
}

func TestGeneratePromptDeterministicAndDryRun(t *testing.T) {
	build := func() *Metadata {
		return &Metadata{
			GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
			Types: map[string]TypeMeta{compType: {PromptSha256: "p-comp"}},
			Resources: map[string]ResourceMeta{
				compType + "/a.yaml": {ResourceId: "a", SourceSha256: "sa", PresentInTenant: true},
				compType + "/b.yaml": {ResourceId: "b", SourceSha256: "sb", PresentInTenant: true},
			},
		}
	}

	// Dry-run writes nothing.
	dir1 := t.TempDir()
	writeMeta(t, dir1, build())
	writePromptFile(t, filepath.Join(dir1, models.ResourcesDirName), compType)
	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: dir1, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Written {
		t.Error("dry run must not write")
	}
	if _, statErr := os.Stat(filepath.Join(dir1, DocsDirName, GenerateFileName)); !os.IsNotExist(statErr) {
		t.Error("dry run must not create generate.md")
	}

	// Two real runs over the same unchanged export are byte-identical (the
	// generate.md written by the first run sits at the docs/ root and is not a
	// documented resource, so it does not affect the second run).
	dir := t.TempDir()
	writeMeta(t, dir, build())
	writePromptFile(t, filepath.Join(dir, models.ResourcesDirName), compType)
	run := func() []byte {
		r, e := GeneratePrompt(GeneratePromptOptions{TenantDir: dir, Template: DefaultGeneratePromptTemplate()})
		if e != nil {
			t.Fatalf("run: %v", e)
		}
		b, e := os.ReadFile(r.OutPath)
		if e != nil {
			t.Fatalf("read: %v", e)
		}
		return b
	}
	first := string(run())
	second := string(run())
	if first != second {
		t.Error("generate.md must be deterministic across runs")
	}
}

func TestValidateMarkers(t *testing.T) {
	if err := validateMarkers(DefaultGeneratePromptTemplate(), requiredMarkers); err != nil {
		t.Fatalf("default template must validate: %v", err)
	}

	broken := []byte("no markers here")
	err := validateMarkers(broken, []string{"worklist"})
	if err == nil || !strings.Contains(err.Error(), "worklist") {
		t.Fatalf("expected error naming worklist, got %v", err)
	}
}

func TestSpliceMarker(t *testing.T) {
	tmpl := []byte("before\n<!-- x:start -->\nOLD\n<!-- x:end -->\nafter\n")
	out, err := spliceMarker(tmpl, "x", "NEW")
	if err != nil {
		t.Fatalf("splice: %v", err)
	}
	s := string(out)
	if strings.Contains(s, "OLD") {
		t.Error("old content should be replaced")
	}
	if !strings.Contains(s, "<!-- x:start -->\nNEW\n<!-- x:end -->") {
		t.Errorf("markers must be preserved around new content: %q", s)
	}
}

func TestParseFrontmatter(t *testing.T) {
	valid := []byte("---\nsource: a.yaml\nsourceSha256: abc\npromptSha256: def\n---\n# body\n")
	fm, ok := parseFrontmatter(valid)
	if !ok {
		t.Fatal("expected valid frontmatter")
	}
	if fm.SourceSha256 != "abc" || fm.PromptSha256 != "def" {
		t.Errorf("parsed = %+v", fm)
	}

	for _, bad := range [][]byte{
		[]byte("# no frontmatter\n"),
		[]byte("---\nunterminated: true\n"),
	} {
		if _, ok := parseFrontmatter(bad); ok {
			t.Errorf("expected parse failure for %q", bad)
		}
	}
}
