package docs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"
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
	srcSha                  string
	promptSha               string
	assignmentsSha          string
	targetedBySha           string
	usedBySha               string
	notificationsSha        string
	withMarkers             bool
	withNotificationsMarker bool
	summary                 string
	platformGroup           string
	functionGroup           string
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
	if fm.usedBySha != "" {
		fmt.Fprintf(&b, "usedBySha256: %s\n", fm.usedBySha)
	}
	if fm.notificationsSha != "" {
		fmt.Fprintf(&b, "notificationsSha256: %s\n", fm.notificationsSha)
	}
	if fm.summary != "" {
		fmt.Fprintf(&b, "summary: %s\n", fm.summary)
	}
	if fm.platformGroup != "" {
		fmt.Fprintf(&b, "platformGroup: %s\n", fm.platformGroup)
	}
	if fm.functionGroup != "" {
		fmt.Fprintf(&b, "functionGroup: %s\n", fm.functionGroup)
	}
	b.WriteString("generatedAt: 2026-01-01T00:00:00Z\n---\n# doc\n")
	if fm.withMarkers {
		b.WriteString("\n<!-- assignments:start -->\ntable\n<!-- assignments:end -->\n")
	}
	if fm.withNotificationsMarker {
		b.WriteString("\n<!-- notifications:start -->\nnotifies via template\n<!-- notifications:end -->\n")
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

func TestRenderMigrate(t *testing.T) {
	const header = "| Document | Type | Reason | assignmentsSha256 | notificationsSha256 |\n|---|---|---|---|---|\n"
	const assignReason = "document predates the assignment markers"
	const notifReason = "document predates the noncompliance-notification markers"
	tests := []struct {
		name  string
		items []WorkItem
		want  string
	}{
		{
			name: "empty",
			want: "_No documents need migrating — every current document already carries the markers its content needs._",
		},
		{
			name:  "assignments only",
			items: []WorkItem{{DocPath: "docs/a/x.md", ResourceType: "a", Reason: assignReason, AssignmentsSha256: "AAA"}},
			want:  header + "| `docs/a/x.md` | a | " + assignReason + " | `AAA` |  |",
		},
		{
			name:  "notifications only",
			items: []WorkItem{{DocPath: "docs/a/x.md", ResourceType: "a", Reason: notifReason, NotificationsSha256: "NNN"}},
			want:  header + "| `docs/a/x.md` | a | " + notifReason + " |  | `NNN` |",
		},
		{
			name: "two items of one document merge and rows sort by document",
			items: []WorkItem{
				{DocPath: "docs/b/z.md", ResourceType: "b", Reason: assignReason, AssignmentsSha256: "ZZZ"},
				{DocPath: "docs/a/x.md", ResourceType: "a", Reason: assignReason, AssignmentsSha256: "AAA"},
				{DocPath: "docs/a/x.md", ResourceType: "a", Reason: notifReason, NotificationsSha256: "NNN"},
			},
			want: header +
				"| `docs/a/x.md` | a | " + assignReason + "; " + notifReason + " | `AAA` | `NNN` |\n" +
				"| `docs/b/z.md` | b | " + assignReason + " | `ZZZ` |  |",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderMigrate(tt.items); got != tt.want {
				t.Errorf("renderMigrate =\n%s\nwant\n%s", got, tt.want)
			}
		})
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

func TestRenderSummaryFacts(t *testing.T) {
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z",
		Tenant:      "example.com",
		Run:         RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:                {HasAssignments: true},
			groupsType:              {},
			autopilotIdentitiesType: {},
		},
		Resources: map[string]ResourceMeta{
			// p1: assigned to a dynamic group and to all users.
			compType + "/p1.yaml": {
				ResourceId: "p1", DisplayName: "P1", PresentInTenant: true, Platforms: "windows",
				AssignmentTargets: []interface{}{groupTarget("G1"), allUsersTarget()},
			},
			// p2: present but configured-but-unassigned.
			compType + "/p2.yaml": {ResourceId: "p2", DisplayName: "P2", PresentInTenant: true, Platforms: "macOS"},
			// p3: gone from the tenant, retained.
			compType + "/p3.yaml": {ResourceId: "p3", DisplayName: "P3", PresentInTenant: false},
			// G1: a present dynamic group (counts everything present).
			groupsType + "/g1.yaml": {
				ResourceId: "G1", DisplayName: "Group One", PresentInTenant: true,
				GroupTypes: []string{"DynamicMembership"},
			},
			// Autopilot record is counted too under "count everything present".
			autopilotIdentitiesType + "/dev1.yaml": {ResourceId: "dev1", PresentInTenant: true},
		},
		NotListed: NotListedMeta{
			Types:   []string{"Microsoft.Graph/notlisted", "Microsoft.Graph/alsonotlisted"},
			Empty:   []string{"Microsoft.Graph/empty"},
			Reasons: map[string]string{"Microsoft.Graph/notlisted": "HTTP 503: failed to list notlisted: service unavailable"},
		},
	}

	out := renderSummaryFacts(m, buildGroupInfo(m))

	wants := []string{
		"Export generated at: `2026-01-02T03:04:05Z`",
		"Export complete: `true`",
		"| " + compType + " | 2 | macOS, windows | yes |",
		"| " + groupsType + " | 1 | — | no |",
		"| " + autopilotIdentitiesType + " | 1 | — | no |",
		"_4 resource(s) present across 3 type(s)._",
		"- Assigned: 1 of 2 resources",
		"- Configured but unassigned: 1",
		"- Targets: All users ×1 · All devices ×0 · group targets ×1 (dynamic ×1 · assigned ×0 · dangling ×0)",
		"Retained but no longer in tenant: 1",
		"Types not listed: Microsoft.Graph/alsonotlisted, Microsoft.Graph/notlisted (HTTP 503: failed to list notlisted: service unavailable)\n",
		"Types that listed to zero: Microsoft.Graph/empty",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("summary-facts missing %q in:\n%s", w, out)
		}
	}

	// Deterministic across calls.
	if out != renderSummaryFacts(m, buildGroupInfo(m)) {
		t.Error("renderSummaryFacts must be deterministic")
	}
}

func TestRenderSummaryFactsDanglingGroup(t *testing.T) {
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{compType: {HasAssignments: true}},
		Resources: map[string]ResourceMeta{
			compType + "/p1.yaml": {
				ResourceId: "p1", PresentInTenant: true,
				// G-missing has no group entry -> dangling target.
				AssignmentTargets: []interface{}{groupTarget("G-missing")},
			},
		},
	}
	out := renderSummaryFacts(m, buildGroupInfo(m))
	if !strings.Contains(out, "group targets ×1 (dynamic ×0 · assigned ×0 · dangling ×1)") {
		t.Errorf("dangling group must be counted, got:\n%s", out)
	}
}

func TestGeneratePromptWritesSummaryFacts(t *testing.T) {
	tenantDir := t.TempDir()
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{compType: {PromptSha256: "p-comp"}},
		Resources: map[string]ResourceMeta{
			compType + "/a.yaml": {ResourceId: "a", SourceSha256: "sa", PresentInTenant: true, Platforms: "windows"},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, filepath.Join(tenantDir, models.ResourcesDirName), compType)

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate()})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	out, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	body := string(out)
	// The block markers survive and the facts are spliced between them.
	if !strings.Contains(body, "<!-- summary-facts:start -->") || !strings.Contains(body, "<!-- summary-facts:end -->") {
		t.Error("summary-facts markers must survive splicing")
	}
	if !strings.Contains(body, "| "+compType+" | 1 | windows | no |") {
		t.Errorf("generate.md must carry the spliced summary facts:\n%s", body)
	}
}

func TestValidateMarkers(t *testing.T) {
	if err := ValidateMarkers(DefaultGeneratePromptTemplate(), requiredMarkers); err != nil {
		t.Fatalf("default template must validate: %v", err)
	}

	broken := []byte("no markers here")
	err := ValidateMarkers(broken, []string{"worklist"})
	if err == nil || !strings.Contains(err.Error(), "worklist") {
		t.Fatalf("expected error naming worklist, got %v", err)
	}
}

func TestSpliceMarker(t *testing.T) {
	tmpl := []byte("before\n<!-- x:start -->\nOLD\n<!-- x:end -->\nafter\n")
	out, err := SpliceMarker(tmpl, "x", "NEW")
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

func TestRenderWorklistIncludesAllHashColumns(t *testing.T) {
	items := []WorkItem{
		{
			ResourceType:        compType,
			SourcePath:          "resources/" + compType + "/pol.yaml",
			DocPath:             "docs/" + compType + "/pol.md",
			Reason:              "resource changed",
			SourceSha256:        "src-hash",
			PromptSha256:        "prompt-hash",
			AssignmentsSha256:   "assign-hash",
			NotificationsSha256: "notif-hash",
		},
		{
			ResourceType: notificationMessageTemplatesType,
			SourcePath:   "resources/" + notificationMessageTemplatesType + "/t1.yaml",
			DocPath:      "docs/" + notificationMessageTemplatesType + "/t1.md",
			Reason:       "no document",
			SourceSha256: "src-t1",
			PromptSha256: "prompt-t1",
			UsedBySha256: "usedby-hash",
		},
		{
			ResourceType:     groupsType,
			SourcePath:       "resources/" + groupsType + "/g1.yaml",
			DocPath:          "docs/" + groupsType + "/g1.md",
			Reason:           "no document",
			SourceSha256:     "src-g1",
			PromptSha256:     "prompt-g1",
			TargetedBySha256: "targetedby-hash",
		},
	}
	out := renderWorklist(items)

	// Header must include all hash columns.
	if !strings.Contains(out, "| notificationsSha256 |") {
		t.Error("worklist table must have a notificationsSha256 column")
	}
	if !strings.Contains(out, "| usedBySha256 |") {
		t.Error("worklist table must have a usedBySha256 column")
	}
	if !strings.Contains(out, "| targetedBySha256 |") {
		t.Error("worklist table must have a targetedBySha256 column")
	}
	// The compliance policy row must render the notifications hash.
	if !strings.Contains(out, "`notif-hash`") {
		t.Errorf("compliance policy row must render notificationsSha256, got:\n%s", out)
	}
	// The template row must render the used-by hash.
	if !strings.Contains(out, "`usedby-hash`") {
		t.Errorf("template row must render usedBySha256, got:\n%s", out)
	}
	// The group row must render the targeted-by hash.
	if !strings.Contains(out, "`targetedby-hash`") {
		t.Errorf("group row must render targetedBySha256, got:\n%s", out)
	}
	// Empty hash columns must render as empty cells (not backticked).
	// The template row has no notificationsSha256 or assignmentsSha256.
	if strings.Contains(out, "`notif-hash`") && strings.Contains(out, "`usedby-hash`") {
		// Both hashes present — verify they are on different rows.
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.Contains(line, "`usedby-hash`") && strings.Contains(line, "`notif-hash`") {
				t.Error("usedBySha256 and notificationsSha256 must not be on the same row")
			}
		}
	}
}

func TestRenderExpected(t *testing.T) {
	// Empty work list renders a comment line, not an empty block, so
	// chunks/expected.txt is never ambiguous.
	if got := renderExpected(nil); !strings.HasPrefix(got, "#") {
		t.Errorf("empty work list must render a comment line, got %q", got)
	}

	items := []WorkItem{
		{SourcePath: "resources/b/two.yaml"},
		{SourcePath: "resources/a/one.yaml"},
	}
	got := renderExpected(items)
	// Sorted, one path per line, nothing else.
	if got != "resources/a/one.yaml\nresources/b/two.yaml" {
		t.Errorf("renderExpected = %q", got)
	}
}

func TestGeneratePromptGroupCarriesTargetedByHash(t *testing.T) {
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)

	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z",
		Tenant:      "example.com",
		Run:         RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:   {PromptSha256: "p-comp", HasAssignments: true},
			groupsType: {PromptSha256: "p-grp"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId:        "pol",
				SourceSha256:      "s-pol",
				PresentInTenant:   true,
				AssignmentTargets: []interface{}{groupTarget("G1")},
			},
			groupsType + "/g1.yaml": {ResourceId: "G1", DisplayName: "Group One", SourceSha256: "s-g1", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, groupsType)
	// policy has a current doc; referenced group G1 has none -> G1 is in list 1.
	writeDoc(t, tenantDir, compType+"/policy.yaml", "s-pol", "p-comp")

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate()})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}

	// The freshly generated group document must carry the reverse hash so its
	// Targeted by block and hash land in the run that creates it.
	var g1 *WorkItem
	for i := range res.ToGenerate {
		if res.ToGenerate[i].DocPath == "docs/"+groupsType+"/g1.md" {
			g1 = &res.ToGenerate[i]
		}
	}
	if g1 == nil {
		t.Fatalf("group G1 must be in the work list, got %+v", res.ToGenerate)
	}
	if want := reverseHash(m, "G1"); g1.TargetedBySha256 != want || want == "" {
		t.Errorf("group work item TargetedBySha256 = %q, want %q", g1.TargetedBySha256, want)
	}

	// The written prompt's expected block must list the group's source path so
	// the section-4 coverage check can diff against it.
	out, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, "<!-- expected:start -->") || !strings.Contains(body, "<!-- expected:end -->") {
		t.Error("prompt must contain the expected marker pair")
	}
	if !strings.Contains(body, srcRel(groupsType+"/g1.yaml")) {
		t.Errorf("expected block must list the group source path, got:\n%s", body)
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

	// A double-quoted summary keeps an inner ": " intact instead of breaking the frontmatter.
	withSummary := []byte("---\nsource: a.yaml\nsummary: \"Enforces X: requires Y\"\n---\n# body\n")
	fm, ok = parseFrontmatter(withSummary)
	if !ok {
		t.Fatal("expected valid frontmatter with a quoted summary")
	}
	if fm.Summary != "Enforces X: requires Y" {
		t.Errorf("Summary = %q, want %q", fm.Summary, "Enforces X: requires Y")
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

// TestDefaultGeneratePromptTemplateWording pins the run-wide wording of the
// embedded run prompt: it describes full and incremental runs alike, asks for
// the double-quoted summary frontmatter line and checks it, names every
// settings-section variant and bans bare URLs.
func TestDefaultGeneratePromptTemplateWording(t *testing.T) {
	text := string(DefaultGeneratePromptTemplate())

	if !strings.HasPrefix(text, "# Documentation generation prompt (template)\n") {
		t.Errorf("template title = %q", strings.SplitN(text, "\n", 2)[0])
	}
	for _, want := range []string{
		"It covers either every resource (a first run, or a run after a template change moved every",
		"summary: \"",
		"`summary` is required: one sentence taken from the document's summary paragraph",
		"always a double-quoted YAML string on one line",
		"a `Settings`, `Properties` or `Definition` section",
		"never a bare URL",
		"and a non-empty one-line double-quoted `summary`",
		`fail(doc, "frontmatter missing summary")`,
		`fail(doc, "frontmatter summary not a non-empty double-quoted line")`,
		`fail(doc, "frontmatter generatedAt is not the export timestamp")`,
		`fail(doc, "<details> block(s) without a data-setting path")`,
		`fail(doc, "assignments markers missing — the spec asks for an assignments block")`,
		`fail(doc, "assignments markers in a document whose spec has no assignments block")`,
		"The Conditional\nAccess `Conditions` table is not an assignments block",
		"what the settings do, and the\n  scope token in its name only where those don't decide it",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("template missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"This is an **incremental** run",
		"put the source YAML filename\nin backticks on its own line",
		"a `Properties` or `Settings` section",
	} {
		if strings.Contains(text, unwanted) {
			t.Errorf("template unexpectedly contains %q", unwanted)
		}
	}
}

func TestNotListedWithReasons(t *testing.T) {
	tests := []struct {
		name string
		in   NotListedMeta
		want string
	}{
		{name: "nothing failed", in: NotListedMeta{Reasons: map[string]string{}}, want: "none"},
		{name: "older file without reasons", in: NotListedMeta{Types: []string{"b", "a"}}, want: "a, b"},
		{
			name: "reasons sorted by type",
			in:   NotListedMeta{Types: []string{"b", "a"}, Reasons: map[string]string{"a": "HTTP 404 X: gone", "b": "no subscription available"}},
			want: "a (HTTP 404 X: gone), b (no subscription available)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := notListedWithReasons(tt.in); got != tt.want {
				t.Errorf("notListedWithReasons() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The docstrings that identify the run prompt's shipped Python scripts.
const (
	section4Docstring    = `"""Section 4 structural checks. Run from the tenant folder. Exit 1 if anything failed."""`
	section6Docstring    = `"""Section 6 reference checks. Run from the tenant folder after section 5. Exit 1 if anything failed."""`
	signalSweepDocstring = `"""Section 7 signal sweep. Run from the tenant folder before writing docs/summary.md. Prints each signal with the resources it names."""`
)

// embeddedScript cuts the Python script identified by docstring out of its
// four-backtick fence in text.
func embeddedScript(t *testing.T, text, docstring string) string {
	t.Helper()
	i := strings.Index(text, docstring)
	if i < 0 {
		t.Fatalf("no script with docstring %s", docstring)
	}
	const open = "````python\n"
	start := strings.LastIndex(text[:i], open)
	end := strings.Index(text[i:], "\n````")
	if start < 0 || end < 0 {
		t.Fatalf("script %s is not inside a four-backtick python fence", docstring)
	}
	return text[start+len(open) : i+end+1]
}

// pythonFunction returns a top-level Python function's source: its def line and
// every following line up to the next unindented one.
func pythonFunction(t *testing.T, script, name string) string {
	t.Helper()
	i := strings.Index(script, "def "+name+"(")
	if i < 0 {
		t.Fatalf("script has no function %s", name)
	}
	lines := strings.Split(script[i:], "\n")
	out := lines[:1]
	for _, l := range lines[1:] {
		if l != "" && !strings.HasPrefix(l, " ") {
			break
		}
		out = append(out, l)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n")
}

// requirePython skips the test when no python3 is on the PATH; CI's runner has one.
func requirePython(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	return p
}

// runScript runs a Python script with dir as its working directory and returns
// its combined output and exit code.
func runScript(t *testing.T, python, dir, script string) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "script.py")
	if err := os.WriteFile(path, []byte(script), 0644); err != nil {
		t.Fatalf("write script: %v", err)
	}
	cmd := exec.CommandContext(t.Context(), python, path)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exitErr):
		return string(out), exitErr.ExitCode()
	default:
		t.Fatalf("run script: %v", err)
		return "", -1
	}
}

// TestDefaultGeneratePromptTemplateShipsScripts pins the scripts the run prompt
// ships for sections 6 and 7, so no run has to write its own.
func TestDefaultGeneratePromptTemplateShipsScripts(t *testing.T) {
	text := string(DefaultGeneratePromptTemplate())
	for _, want := range []string{
		section6Docstring,
		signalSweepDocstring,
		"def marker_problems(",
		`column_links(doc, blk(doc, "assignments"), "Target")`,
		`column_links(group, blk(group, "targeted-by"), "Resource")`,
		`WORDS = ("password", "passwd", "pwd", "passphrase", "secret", "token", "apikey", "authkey", "accesskey", "privatekey", "sharedkey")`,
		`key.lower().endswith("commandline")`,
		`CREDENTIAL_TYPES_HEADING = "Expiry and renewal"`,
		"found by exactly five rules",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("template missing %q", want)
		}
	}

	// The scripts must not carry a literal tool-filled marker: rendering would
	// refuse the template, or splice into the script instead of the block.
	tenantDir := t.TempDir()
	writeMeta(t, tenantDir, &Metadata{GeneratedAt: "2026-10-02T00:00:00Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{}, Resources: map[string]ResourceMeta{}})
	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate()})
	if err != nil {
		t.Fatalf("GeneratePrompt with the default template: %v", err)
	}
	out, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	for _, docstring := range []string{section4Docstring, section6Docstring, signalSweepDocstring} {
		if !strings.Contains(string(out), docstring) {
			t.Errorf("rendered prompt lost the script %s", docstring)
		}
	}
}

// TestMarkerProblemsHelperIsShared keeps the section-6 copy of the marker-pair
// helper identical to section 4's: the agent pastes each script as its own file,
// so the helper is shared by copy, not import.
func TestMarkerProblemsHelperIsShared(t *testing.T) {
	text := string(DefaultGeneratePromptTemplate())
	s4 := pythonFunction(t, embeddedScript(t, text, section4Docstring), "marker_problems")
	s6 := pythonFunction(t, embeddedScript(t, text, section6Docstring), "marker_problems")
	if s4 != s6 {
		t.Errorf("marker_problems differs between sections 4 and 6:\n--- section 4\n%s\n--- section 6\n%s", s4, s6)
	}
}

// TestMarkerProblemsDetectsOrderAndNesting runs the section-4 helper over
// documents whose marked blocks are well formed, reversed and nested.
func TestMarkerProblemsDetectsOrderAndNesting(t *testing.T) {
	python := requirePython(t)
	helper := pythonFunction(t, embeddedScript(t, string(DefaultGeneratePromptTemplate()), section4Docstring), "marker_problems")
	script := "import re, sys\n\n" + helper + "\n\nprint(marker_problems(sys.stdin.read()))\n"
	tests := []struct {
		name string
		doc  string
		want string
	}{
		{"well formed", "<!-- assignments:start -->\nx\n<!-- assignments:end -->\n<!-- notifications:start -->\ny\n<!-- notifications:end -->\n", "[]"},
		{"end before start", "<!-- assignments:end -->\nx\n<!-- assignments:start -->\n", "assignments markers: end before start"},
		{"nested", "<!-- targeted-by:start -->\n<!-- assignments:start -->\n<!-- assignments:end -->\n<!-- targeted-by:end -->\n", "assignments markers nested inside targeted-by"},
		{"unbalanced", "<!-- used-by:start -->\n", "used-by markers unbalanced: 1 start / 0 end"},
		{"repeated", "<!-- used-by:start -->\n<!-- used-by:end -->\n<!-- used-by:start -->\n<!-- used-by:end -->\n", "used-by markers repeated (2 pairs)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "helper.py")
			if err := os.WriteFile(path, []byte(script), 0644); err != nil {
				t.Fatalf("write helper: %v", err)
			}
			cmd := exec.CommandContext(t.Context(), python, path)
			cmd.Stdin = strings.NewReader(tt.doc)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("run helper: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), tt.want) {
				t.Errorf("marker_problems = %s, want it to contain %q", out, tt.want)
			}
		})
	}
}

// Resource ids of the section-6 fixture: a compliance policy assigned to a group
// through an assignment filter and notifying through a template, plus a second
// filter whose document is current and so outside every list.
const (
	refPolicyID   = "11111111-1111-4111-8111-111111111111"
	refGroupID    = "22222222-2222-4222-8222-222222222222"
	refTemplateID = "33333333-3333-4333-8333-333333333333"
	refFilterID   = "44444444-4444-4444-8444-444444444444"
	refOtherID    = "55555555-5555-4555-8555-555555555555"
)

// referenceFixtureMtime is the section-4 snapshot time of every fixture document.
var referenceFixtureMtime = time.Date(2026, 10, 2, 8, 0, 0, 0, time.UTC)

// referenceFixture builds a tenant whose documents section 6 must pass: the
// prompt is rendered by GeneratePrompt so the script reads the real table
// shapes, the documents carry the work list's hashes, and chunks/mtimes.json
// records their mtimes. It returns the tenant directory and the work-list rows
// by document path.
func referenceFixture(t *testing.T) (string, map[string]WorkItem) {
	t.Helper()
	return referenceFixtureMigrating(t, false)
}

// referenceFixtureMigrating is referenceFixture with the option of a policy
// document that is current but carries neither marker when the prompt is
// rendered, so it lands in the migrate table as one merged row; the policy is
// then rewritten with the fixture body and both hashes from its Migrate items.
func referenceFixtureMigrating(t *testing.T, policyMigrates bool) (string, map[string]WorkItem) {
	t.Helper()
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-10-02T00:00:00Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:                         {PromptSha256: "p-comp", HasAssignments: true},
			groupsType:                       {PromptSha256: "p-grp"},
			notificationMessageTemplatesType: {PromptSha256: "p-tmpl"},
			assignmentFiltersType:            {PromptSha256: "p-flt"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: refPolicyID, DisplayName: "Policy", SourceSha256: "s-pol", PresentInTenant: true,
				AssignmentTargets:        []interface{}{groupTargetWithFilter(refGroupID, refFilterID, "include")},
				NotificationTemplateRefs: []string{refTemplateID},
			},
			groupsType + "/group.yaml": {
				ResourceId: refGroupID, DisplayName: "Group", SourceSha256: "s-grp", PresentInTenant: true,
				SecurityEnabled: boolPtr(true),
			},
			notificationMessageTemplatesType + "/template.yaml": {ResourceId: refTemplateID, DisplayName: "Template", SourceSha256: "s-tmpl", PresentInTenant: true},
			assignmentFiltersType + "/filter.yaml":              {ResourceId: refFilterID, DisplayName: "Filter", SourceSha256: "s-flt", PresentInTenant: true},
			assignmentFiltersType + "/other.yaml":               {ResourceId: refOtherID, DisplayName: "Other", SourceSha256: "s-oth", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	for _, rtype := range []string{compType, groupsType, notificationMessageTemplatesType, assignmentFiltersType} {
		writePromptFile(t, resourcesDir, rtype)
	}
	// Current before the prompt is rendered, so it is in none of the three lists.
	writeDoc(t, tenantDir, assignmentFiltersType+"/other.yaml", "s-oth", "p-flt")
	if policyMigrates {
		writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp"})
	}

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate()})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	items := map[string]WorkItem{}
	for _, it := range res.ToGenerate {
		items[it.DocPath] = it
	}
	// A document missing both markers has one Migrate item per marker; the
	// hashes to write are the union.
	for _, it := range res.Migrate {
		merged := items[it.DocPath]
		merged.ResourceType, merged.SourcePath, merged.DocPath = it.ResourceType, it.SourcePath, it.DocPath
		merged.SourceSha256, merged.PromptSha256 = it.SourceSha256, it.PromptSha256
		if it.AssignmentsSha256 != "" {
			merged.AssignmentsSha256 = it.AssignmentsSha256
		}
		if it.NotificationsSha256 != "" {
			merged.NotificationsSha256 = it.NotificationsSha256
		}
		items[it.DocPath] = merged
	}
	if len(items) != 4 {
		t.Fatalf("fixture work list = %+v, want policy, group, template and filter", res.ToGenerate)
	}

	bodies := map[string]string{
		"docs/" + compType + "/policy.md": "# Policy\n\n## Assignments\n\n<!-- assignments:start -->\n\n" +
			"| Direction | Target | Filter |\n|---|---|---|\n" +
			"| Include | [[PROD] Group](../groups/group.md) · assigned security group · `" + refGroupID + "` | include [Filter](../assignmentFilters/filter.md) |\n\n" +
			"<!-- assignments:end -->\n\n## Settings\n\n<!-- notifications:start -->\n" +
			"Noncompliance actions notify through [Template](../notificationMessageTemplates/template.md).\n" +
			"<!-- notifications:end -->\n",
		"docs/" + groupsType + "/group.md": "# Group\n\n## Usage as assignment target\n\n<!-- targeted-by:start -->\n## Targeted by\n\n" +
			"1 resource assigns this group.\n\n| Resource | Type | Direction | Filter |\n|---|---|---|---|\n" +
			"| [\\[PROD\\] Policy](../deviceCompliancePolicies/policy.md) | deviceCompliancePolicies | Include | include [Filter](../assignmentFilters/filter.md) |\n" +
			"<!-- targeted-by:end -->\n",
		"docs/" + notificationMessageTemplatesType + "/template.md": "# Template\n\n## Usage and references\n\n<!-- used-by:start -->\n## Used by\n\n" +
			"1 resource references this template in a noncompliance action.\n\n| Resource | Type |\n|---|---|\n" +
			"| [[PROD] Policy](../deviceCompliancePolicies/policy.md) | deviceCompliancePolicies |\n<!-- used-by:end -->\n",
		"docs/" + assignmentFiltersType + "/filter.md": "# Filter\n",
	}
	for doc, body := range bodies {
		it, ok := items[doc]
		if !ok {
			t.Fatalf("%s is not in the work list", doc)
		}
		var b strings.Builder
		fmt.Fprintf(&b, "---\nsource: %s\nsourceSha256: %s\npromptSha256: %s\n", it.SourcePath, it.SourceSha256, it.PromptSha256)
		for _, h := range [][2]string{
			{"assignmentsSha256", it.AssignmentsSha256}, {"notificationsSha256", it.NotificationsSha256},
			{"usedBySha256", it.UsedBySha256}, {"targetedBySha256", it.TargetedBySha256},
		} {
			if h[1] != "" {
				fmt.Fprintf(&b, "%s: %s\n", h[0], h[1])
			}
		}
		b.WriteString("generatedAt: 2026-10-02T00:00:00Z\n---\n" + body)
		path := filepath.Join(tenantDir, filepath.FromSlash(doc))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", doc, err)
		}
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatalf("write %s: %v", doc, err)
		}
	}

	// The section-4 snapshot, on whole seconds so Python reads back the same value.
	snapshot := map[string]int64{}
	for doc := range bodies {
		snapshot[doc] = referenceFixtureMtime.Unix()
	}
	snapshot["docs/"+assignmentFiltersType+"/other.md"] = referenceFixtureMtime.Unix()
	for doc := range snapshot {
		if err := os.Chtimes(filepath.Join(tenantDir, filepath.FromSlash(doc)), referenceFixtureMtime, referenceFixtureMtime); err != nil {
			t.Fatalf("chtimes %s: %v", doc, err)
		}
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tenantDir, "chunks"), 0755); err != nil {
		t.Fatalf("mkdir chunks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tenantDir, "chunks", "mtimes.json"), data, 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	return tenantDir, items
}

// editFixtureDoc replaces old with replacement in one fixture document.
func editFixtureDoc(t *testing.T, tenantDir, doc, old, replacement string) {
	t.Helper()
	path := filepath.Join(tenantDir, filepath.FromSlash(doc))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", doc, err)
	}
	if !strings.Contains(string(data), old) {
		t.Fatalf("%s does not contain %q", doc, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0644); err != nil {
		t.Fatalf("write %s: %v", doc, err)
	}
}

// TestSectionSixReferenceScript runs the shipped section-6 script, taken from
// the rendered prompt, over a clean fixture and over one planted defect per
// check.
func TestSectionSixReferenceScript(t *testing.T) {
	python := requirePython(t)
	policyDoc := "docs/" + compType + "/policy.md"
	groupDoc := "docs/" + groupsType + "/group.md"
	templateDoc := "docs/" + notificationMessageTemplatesType + "/template.md"

	tests := []struct {
		name    string
		plant   func(t *testing.T, dir string, items map[string]WorkItem)
		message string
	}{
		{name: "clean, with the filter link in the Targeted by Filter column"},
		{
			name: "policy row removed from the group's Targeted by",
			plant: func(t *testing.T, dir string, _ map[string]WorkItem) {
				editFixtureDoc(t, dir, groupDoc, "| [\\[PROD\\] Policy](../deviceCompliancePolicies/policy.md) | deviceCompliancePolicies | Include | include [Filter](../assignmentFilters/filter.md) |\n", "")
			},
			message: "assigns a group whose Targeted by does not list this document",
		},
		{
			name: "bare group GUID in an assignments block",
			plant: func(t *testing.T, dir string, _ map[string]WorkItem) {
				editFixtureDoc(t, dir, policyDoc, "[[PROD] Group](../groups/group.md) · assigned security group · ", "")
			},
			message: "bare GUID in a marked block",
		},
		{
			name: "wrong targetedBySha256",
			plant: func(t *testing.T, dir string, items map[string]WorkItem) {
				editFixtureDoc(t, dir, groupDoc, "targetedBySha256: "+items[groupDoc].TargetedBySha256, "targetedBySha256: 0000")
			},
			message: "targetedBySha256 missing or not the value the prompt gives",
		},
		{
			name: "document outside the three lists touched",
			plant: func(t *testing.T, dir string, _ map[string]WorkItem) {
				later := referenceFixtureMtime.Add(time.Hour)
				if err := os.Chtimes(filepath.Join(dir, "docs", filepath.FromSlash(assignmentFiltersType), "other.md"), later, later); err != nil {
					t.Fatalf("chtimes: %v", err)
				}
			},
			message: "changed since section 4 but not in the work-list, re-splice or migrate lists",
		},
		{
			name: "notifications link with no Used by answer",
			plant: func(t *testing.T, dir string, _ map[string]WorkItem) {
				editFixtureDoc(t, dir, templateDoc, "| [[PROD] Policy](../deviceCompliancePolicies/policy.md) | deviceCompliancePolicies |\n", "")
			},
			message: "notifies through a template whose Used by does not list this document",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, items := referenceFixture(t)
			if tt.plant != nil {
				tt.plant(t, dir, items)
			}
			prompt, err := os.ReadFile(filepath.Join(dir, DocsDirName, GenerateFileName))
			if err != nil {
				t.Fatalf("read rendered prompt: %v", err)
			}
			out, code := runScript(t, python, dir, embeddedScript(t, string(prompt), section6Docstring))
			if tt.message == "" {
				if code != 0 {
					t.Fatalf("clean fixture: exit %d\n%s", code, out)
				}
				return
			}
			if code != 1 || !strings.Contains(out, tt.message) {
				t.Errorf("exit %d, want 1 with %q:\n%s", code, tt.message, out)
			}
		})
	}
}

// TestSectionSixReferenceScriptMigratedPolicy runs the section-6 script over a
// tree whose policy was migrated: it was listed in the migrate table with both
// hashes, and was rewritten carrying them. The script must read both hash
// columns by header, pass, and fail when the notifications hash is dropped.
func TestSectionSixReferenceScriptMigratedPolicy(t *testing.T) {
	python := requirePython(t)
	policyDoc := "docs/" + compType + "/policy.md"
	dir, items := referenceFixtureMigrating(t, true)
	prompt, err := os.ReadFile(filepath.Join(dir, DocsDirName, GenerateFileName))
	if err != nil {
		t.Fatalf("read rendered prompt: %v", err)
	}
	if !strings.Contains(string(prompt), "| Document | Type | Reason | assignmentsSha256 | notificationsSha256 |") {
		t.Fatalf("migrate table missing from the rendered prompt")
	}
	script := embeddedScript(t, string(prompt), section6Docstring)
	if out, code := runScript(t, python, dir, script); code != 0 {
		t.Fatalf("migrated policy with both hashes: exit %d\n%s", code, out)
	}

	editFixtureDoc(t, dir, policyDoc, "notificationsSha256: "+items[policyDoc].NotificationsSha256+"\n", "")
	out, code := runScript(t, python, dir, script)
	if code != 1 || !strings.Contains(out, "notificationsSha256 missing or not the value the prompt gives") {
		t.Errorf("exit %d, want 1 with the missing notificationsSha256 message:\n%s", code, out)
	}
}

// Credentials planted in the signal-sweep fixture; the sweep must report where
// they are and never print them.
const (
	plantedAPIToken = "Xk9#mQ2vLp7zRt"
	plantedAuthKey  = "Rk7!pW3nZq9Lm2"
	danglingGroupID = "99999999-9999-4999-8999-999999999999"

	plantedQuotedArg = "Xy9!kLm2pQ"
	plantedWifiKey   = "Zt5@bN8cYh3d"
	plantedNote      = "Hn4$vT8wQe1x"
	plantedOMA       = "Qw3$eR6tYu9i"
)

// writeResourceYAML writes a resource the way the pipeline does
// (pipeline.MarshalResourceYAML), so the sweep reads the real yaml.v3 shape.
func writeResourceYAML(t *testing.T, resourcesDir, key string, data map[string]interface{}) {
	t.Helper()
	b, err := pipeline.MarshalResourceYAML(data)
	if err != nil {
		t.Fatalf("marshal %s: %v", key, err)
	}
	path := filepath.Join(resourcesDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", key, err)
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
}

// TestSectionSevenSignalSweep runs the shipped signal sweep over a resources/
// tree holding a command-line token, a plist credential, a VPP token near
// expiry, non-credential expiries, a credential-looking non-credential key, an
// unassigned resource and a dangling group target.
func TestSectionSevenSignalSweep(t *testing.T) {
	python := requirePython(t)
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	const (
		appsType   = "Microsoft.Graph/mobileApps"
		configType = "Microsoft.Graph/deviceConfigurations"
		vppType    = "Microsoft.Graph/vppTokens"
	)
	writeMeta(t, tenantDir, &Metadata{
		GeneratedAt: "2026-10-02T00:00:00Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			appsType: {HasAssignments: true}, configType: {HasAssignments: true}, groupsType: {}, vppType: {},
		},
		Resources: map[string]ResourceMeta{
			appsType + "/agent.yaml": {ResourceId: "app", DisplayName: "Agent", PresentInTenant: true,
				AssignmentTargets: []interface{}{allUsersTarget(), groupTarget(danglingGroupID)}},
			configType + "/ios_custom.yaml":  {ResourceId: "ios", DisplayName: "iOS custom", PresentInTenant: true, AssignmentTargets: []interface{}{allUsersTarget()}},
			configType + "/update_ring.yaml": {ResourceId: "ring", DisplayName: "Update ring", PresentInTenant: true},
			configType + "/wifi.yaml":        {ResourceId: "wifi", DisplayName: "Wi-Fi", PresentInTenant: true, AssignmentTargets: []interface{}{allUsersTarget()}},
			groupsType + "/m365.yaml":        {ResourceId: "grp", DisplayName: "M365 group", PresentInTenant: true},
			vppType + "/vpp.yaml":            {ResourceId: "vpp", DisplayName: "VPP", PresentInTenant: true},
		},
	})

	plist := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\">\n<dict>\n" +
		"  <key>RemoteOfficeAuthKey</key>\n  <string>" + plantedAuthKey + "</string>\n" +
		"  <key>PayloadDisplayName</key>\n  <string>Remote office</string>\n</dict>\n</plist>\n"
	writeResourceYAML(t, resourcesDir, appsType+"/agent.yaml", map[string]interface{}{
		"@odata.type":          "#microsoft.graph.win32LobApp",
		"displayName":          "Agent",
		"installCommandLine":   "setup.exe /quiet APITOKEN=" + plantedAPIToken,
		"uninstallCommandLine": "msiexec /x {11111111-2222-3333-4444-555555555555} /quiet",
		"repairCommandLine":    "msiexec /qn \"PASSWORD=" + plantedQuotedArg + "\"",
	})
	writeResourceYAML(t, resourcesDir, configType+"/ios_custom.yaml", map[string]interface{}{
		"@odata.type":     "#microsoft.graph.iosCustomConfiguration",
		"displayName":     "iOS custom",
		"payload":         plist,
		"payloadFileName": "remote.mobileconfig",
	})
	if err := os.WriteFile(filepath.Join(resourcesDir, filepath.FromSlash(configType), "ios_custom.mobileconfig"), []byte(plist), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
	writeResourceYAML(t, resourcesDir, configType+"/update_ring.yaml", map[string]interface{}{
		"@odata.type":                       "#microsoft.graph.windowsUpdateForBusinessConfiguration",
		"displayName":                       "Update ring",
		"featureUpdatesPauseExpiryDateTime": "2026-10-20T00:00:00Z",
		"qualityUpdatesPauseExpiryDateTime": "2026-11-01T00:00:00Z",
	})
	writeResourceYAML(t, resourcesDir, configType+"/wifi.yaml", map[string]interface{}{
		"@odata.type":  "#microsoft.graph.windows10CustomConfiguration",
		"displayName":  "Wi-Fi",
		"state":        "disabled",
		"wifiPassword": plantedWifiKey,
		"description":  "Setup note password: " + plantedNote,
		"accessToken":  "2026-10-20T00:00:00Z",
		"omaSettings": []interface{}{
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/A", "isEncrypted": true, "value": plantedOMA},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/B", "isEncrypted": true, "value": "*****"},
		},
	})
	writeResourceYAML(t, resourcesDir, groupsType+"/m365.yaml", map[string]interface{}{
		"displayName":        "M365 group",
		"expirationDateTime": "2026-12-01T00:00:00Z",
		"groupTypes":         []interface{}{"Unified"},
	})
	writeResourceYAML(t, resourcesDir, vppType+"/vpp.yaml", map[string]interface{}{
		"expirationDateTime": "2026-12-15T00:00:00Z",
		"tokenName":          "Contoso-VPP-2026a",
	})
	for rtype, headings := range map[string]string{
		appsType:   "References | Lifecycle and operations | Security | Properties",
		configType: "References | Lifecycle and operations | Security | Settings",
		groupsType: "References | Membership | Usage as assignment target | Lifecycle and operations | Security | Properties",
		vppType:    "References | Expiry and renewal | Lifecycle and operations | Security | Properties",
	} {
		spec := "spec\n<!-- doc-headings: " + headings + " -->\n"
		if err := os.WriteFile(filepath.Join(resourcesDir, filepath.FromSlash(rtype), docPromptFileName), []byte(spec), 0644); err != nil {
			t.Fatalf("write spec: %v", err)
		}
	}

	out, code := runScript(t, python, tenantDir, embeddedScript(t, string(DefaultGeneratePromptTemplate()), signalSweepDocstring))
	if code != 0 {
		t.Fatalf("sweep exit %d:\n%s", code, out)
	}
	for _, want := range []string{
		"resources/" + appsType + "/agent.yaml  rule (e)  installCommandLine argument APITOKEN",
		"resources/" + appsType + "/agent.yaml  rule (e)  repairCommandLine argument PASSWORD",
		"resources/" + configType + "/wifi.yaml  state: disabled",
		"resources/" + configType + "/wifi.yaml  rule (b)  wifiPassword  value length " + strconv.Itoa(len(plantedWifiKey)),
		"resources/" + configType + "/wifi.yaml  rule (c)  description after 'password'  value length " + strconv.Itoa(len(plantedNote)),
		"resources/" + configType + "/wifi.yaml  rule (d)  omaSettings[0].value  value length " + strconv.Itoa(len(plantedOMA)),
		"resources/" + configType + "/ios_custom.yaml  rule (a)  payload plist key RemoteOfficeAuthKey",
		"resources/" + configType + "/ios_custom.mobileconfig  rule (a)  plist key RemoteOfficeAuthKey",
		"resources/" + vppType + "/vpp.yaml  expirationDateTime 2026-12-15T00:00:00Z  74 day(s) left",
		"Update ring  docs/" + configType + "/update_ring.md",
		"## Configured but unassigned: 1",
		danglingGroupID + "  assigned by 1 resource(s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("sweep output missing %q:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{
		"PauseExpiryDateTime",
		"m365.yaml",
		"tokenName",
		"uninstallCommandLine",
		plantedAPIToken,
		plantedAuthKey,
		plantedQuotedArg,
		plantedWifiKey,
		plantedNote,
		plantedOMA,
		"omaSettings[1]",
		"accessToken",
	} {
		if strings.Contains(out, unwanted) {
			t.Errorf("sweep output must not contain %q:\n%s", unwanted, out)
		}
	}
}
