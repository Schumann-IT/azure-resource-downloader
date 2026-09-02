package docs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// allUsersTarget builds a raw built-in "all licensed users" assignment target,
// which carries no groupId.
func allUsersTarget() map[string]interface{} {
	return map[string]interface{}{
		"target": map[string]interface{}{
			"@odata.type": "#microsoft.graph.allLicensedUsersAssignmentTarget",
		},
	}
}

// indexScenario builds a metadata + doc tree exercising every index bucket: a
// documented policy (assignments to a group + all users), a pending policy (no
// document yet), a referenced group (in scope, targeted-by), an unreferenced
// group and an autopilot identity (both excluded), and an orphan.
func indexScenario(t *testing.T) string {
	t.Helper()
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
			compType + "/gbl_c_prd_d_win_os.yaml": {
				ResourceId: "pol", DisplayName: "Win OS Validation", SourceSha256: "s-doc",
				ODataType: "#microsoft.graph.windows10CompliancePolicy", Platforms: "windows",
				PresentInTenant:   true,
				AssignmentTargets: []interface{}{groupTarget("G1"), allUsersTarget()},
			},
			compType + "/gbl_c_prd_u_win_pending.yaml": {
				ResourceId: "pend", DisplayName: "Pending Policy", SourceSha256: "s-pend",
				PresentInTenant: true,
			},
			compType + "/gbl_c_prd_d_win_orphan.yaml": {
				ResourceId: "orph", DisplayName: "Orphan", SourceSha256: "s-orph",
				PresentInTenant: false,
			},
			groupsType + "/g1.yaml": {
				ResourceId: "G1", DisplayName: "Group One", SourceSha256: "s-g1",
				GroupTypes: []string{"DynamicMembership"}, SecurityEnabled: boolPtr(true),
				PresentInTenant: true,
			},
			groupsType + "/g2.yaml": {
				ResourceId: "G2", DisplayName: "Unreferenced", SourceSha256: "s-g2",
				PresentInTenant: true,
			},
			autopilotIdentitiesType + "/dev1.yaml": {
				ResourceId: "dev1", PresentInTenant: true,
			},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, groupsType)

	// Documented policy and its referenced group carry LLM frontmatter; the
	// pending policy has no document.
	writeDocFM(t, tenantDir, compType+"/gbl_c_prd_d_win_os.yaml", docFM{
		srcSha: "s-doc", promptSha: "p-comp", assignmentsSha: "a", withMarkers: true,
		summary: "Requires Windows 10 22H2 and BitLocker", platformGroup: "Windows", functionGroup: "Compliance",
	})
	writeDocFM(t, tenantDir, groupsType+"/g1.yaml", docFM{
		srcSha: "s-g1", promptSha: "p-grp", targetedBySha: "tb",
		summary: "Dynamic group of managed Windows devices", platformGroup: "Assignment targets", functionGroup: "Groups",
	})

	return tenantDir
}

func loadIndex(t *testing.T, outPath string) IndexFile {
	t.Helper()
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	var idx IndexFile
	if err := yaml.Unmarshal(data, &idx); err != nil {
		t.Fatalf("unmarshal index: %v", err)
	}
	return idx
}

func resourceByDoc(idx IndexFile, doc string) *IndexResource {
	for i := range idx.Resources {
		if idx.Resources[i].Doc == doc {
			return &idx.Resources[i]
		}
	}
	return nil
}

func TestGenerateIndexBucketsAndEnrichment(t *testing.T) {
	tenantDir := indexScenario(t)

	res, err := GenerateIndex(GenerateIndexOptions{TenantDir: tenantDir})
	if err != nil {
		t.Fatalf("GenerateIndex: %v", err)
	}
	if !res.Written {
		t.Fatal("expected the index to be written")
	}

	// Result counts: two in-scope documents (policy + referenced group), one
	// pending (the second policy), one orphan, two excluded types.
	if res.Documented != 2 || res.Pending != 1 || res.Orphans != 1 {
		t.Errorf("counts: documented=%d pending=%d orphans=%d, want 2/1/1", res.Documented, res.Pending, res.Orphans)
	}
	if res.Excluded[autopilotIdentitiesType] != 1 || res.Excluded[groupsType] != 1 {
		t.Errorf("excluded = %v, want autopilot:1 groups:1", res.Excluded)
	}

	idx := loadIndex(t, res.OutPath)
	if idx.Version != indexVersion {
		t.Errorf("version = %d, want %d", idx.Version, indexVersion)
	}
	if idx.Tenant != "example.com" || idx.GeneratedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("header = %q/%q", idx.Tenant, idx.GeneratedAt)
	}
	if !idx.Complete {
		t.Error("index must report the export as complete")
	}
	if idx.Counts.Documented != 2 || idx.Counts.Pending != 1 {
		t.Errorf("index counts = %+v", idx.Counts)
	}
	// The orphan and both excluded resources must not appear in the list.
	if len(idx.Resources) != 3 {
		t.Fatalf("expected 3 listed resources, got %d: %+v", len(idx.Resources), idx.Resources)
	}

	// Documented policy: enriched from frontmatter, scope from filename, facts
	// from metadata, and a count-only forward assignment summary.
	doc := resourceByDoc(idx, "Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os.md")
	if doc == nil {
		t.Fatal("documented policy missing from index")
	}
	if !doc.Documented {
		t.Error("policy with a document must be documented")
	}
	if doc.Summary != "Requires Windows 10 22H2 and BitLocker" {
		t.Errorf("summary = %q", doc.Summary)
	}
	if doc.PlatformGroup != "Windows" || doc.FunctionGroup != "Compliance" {
		t.Errorf("grouping = %q/%q", doc.PlatformGroup, doc.FunctionGroup)
	}
	if doc.Scope != "device" {
		t.Errorf("scope = %q, want device", doc.Scope)
	}
	if doc.ODataType != "#microsoft.graph.windows10CompliancePolicy" || doc.Platforms != "windows" {
		t.Errorf("facts = %q/%q", doc.ODataType, doc.Platforms)
	}
	if doc.Assignments == nil || doc.Assignments.Groups != 1 || !doc.Assignments.AllUsers {
		t.Errorf("assignments = %+v, want groups:1 allUsers:true", doc.Assignments)
	}

	// Pending policy: listed but not documented, blank summary, and — being
	// assignment-capable but unassigned — a present but empty assignments block.
	pend := resourceByDoc(idx, "Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_u_win_pending.md")
	if pend == nil {
		t.Fatal("pending policy missing from index")
	}
	if pend.Documented || pend.Summary != "" {
		t.Errorf("pending must be undocumented with no summary: %+v", pend)
	}
	if pend.Scope != "user" {
		t.Errorf("pending scope = %q, want user", pend.Scope)
	}
	if pend.Assignments == nil || pend.Assignments.Groups != 0 || pend.Assignments.AllUsers {
		t.Errorf("unassigned capable policy must have an empty assignments block, got %+v", pend.Assignments)
	}

	// Referenced group: in scope, documented, with a targeted-by count.
	grp := resourceByDoc(idx, "Microsoft.Graph/groups/g1.md")
	if grp == nil {
		t.Fatal("referenced group missing from index")
	}
	if !grp.Documented {
		t.Error("referenced group with a document must be documented")
	}
	if grp.Assignments == nil || grp.Assignments.TargetedBy != 1 {
		t.Errorf("group assignments = %+v, want targetedBy:1", grp.Assignments)
	}
}

func TestGenerateIndexWithTaxonomy(t *testing.T) {
	tenantDir := indexScenario(t)

	tax := &TaxonomyConfig{
		Version: 1,
		Programmes: []TaxonomyProgramme{
			{ID: "windows-device", Label: "Windows device config", Match: []TaxonomyRule{{Platforms: "windows", Scope: "device"}}},
			{ID: "groups", Label: "Assignment groups", Match: []TaxonomyRule{{Type: groupsType}}},
			{ID: "empty-prog", Label: "Never matches", Match: []TaxonomyRule{{Name: "zzzznomatch"}}},
		},
	}

	res, err := GenerateIndex(GenerateIndexOptions{TenantDir: tenantDir, Taxonomy: tax})
	if err != nil {
		t.Fatalf("GenerateIndex: %v", err)
	}
	// The pending policy (no platforms) matches nothing.
	if res.Uncategorised != 1 {
		t.Errorf("uncategorised = %d, want 1", res.Uncategorised)
	}

	idx := loadIndex(t, res.OutPath)

	// Vocabularies are always emitted from the model constants, in order.
	if len(idx.Vocabularies.Platform) != len(models.PlatformGroups) || idx.Vocabularies.Platform[0] != models.PlatformGroups[0] {
		t.Errorf("platform vocabulary = %v, want %v", idx.Vocabularies.Platform, models.PlatformGroups)
	}
	if len(idx.Vocabularies.Function) != len(models.FunctionGroups) {
		t.Errorf("function vocabulary length = %d, want %d", len(idx.Vocabularies.Function), len(models.FunctionGroups))
	}

	// The full registry is emitted in display order, zero-count programme kept.
	wantReg := []IndexProgramme{
		{ID: "windows-device", Label: "Windows device config", Count: 1},
		{ID: "groups", Label: "Assignment groups", Count: 1},
		{ID: "empty-prog", Label: "Never matches", Count: 0},
	}
	if len(idx.Programmes) != len(wantReg) {
		t.Fatalf("programmes = %+v, want %+v", idx.Programmes, wantReg)
	}
	for i, w := range wantReg {
		if idx.Programmes[i] != w {
			t.Errorf("programmes[%d] = %+v, want %+v", i, idx.Programmes[i], w)
		}
	}

	// The documented Windows device policy carries the windows-device group.
	doc := resourceByDoc(idx, "Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_d_win_os.md")
	if doc == nil {
		t.Fatal("documented policy missing from index")
	}
	if len(doc.Groups) != 1 || doc.Groups[0].ID != "windows-device" || doc.Groups[0].Label != "Windows device config" {
		t.Errorf("policy groups = %+v, want [windows-device]", doc.Groups)
	}

	// The referenced group carries the groups programme.
	grp := resourceByDoc(idx, "Microsoft.Graph/groups/g1.md")
	if grp == nil {
		t.Fatal("referenced group missing from index")
	}
	if len(grp.Groups) != 1 || grp.Groups[0].ID != "groups" {
		t.Errorf("group groups = %+v, want [groups]", grp.Groups)
	}

	// The pending policy matched nothing, so it carries no groups.
	pend := resourceByDoc(idx, "Microsoft.Graph/deviceCompliancePolicies/gbl_c_prd_u_win_pending.md")
	if pend == nil {
		t.Fatal("pending policy missing from index")
	}
	if len(pend.Groups) != 0 {
		t.Errorf("pending groups = %+v, want none", pend.Groups)
	}
}

func TestGenerateIndexInvalidTaxonomy(t *testing.T) {
	tenantDir := indexScenario(t)

	// Duplicate id is rejected by the compiler.
	bad := &TaxonomyConfig{
		Version: 1,
		Programmes: []TaxonomyProgramme{
			{ID: "a", Label: "A", Match: []TaxonomyRule{{Name: "x"}}},
			{ID: "a", Label: "B", Match: []TaxonomyRule{{Name: "y"}}},
		},
	}

	if _, err := GenerateIndex(GenerateIndexOptions{TenantDir: tenantDir, Taxonomy: bad}); err == nil {
		t.Fatal("expected GenerateIndex to fail on an invalid taxonomy")
	}
}

func TestGenerateIndexDeterministicAndDryRun(t *testing.T) {
	tenantDir := indexScenario(t)

	// Dry run assembles the index but writes nothing.
	res, err := GenerateIndex(GenerateIndexOptions{TenantDir: tenantDir, DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Written {
		t.Error("dry run must not write")
	}
	if _, statErr := os.Stat(res.OutPath); !os.IsNotExist(statErr) {
		t.Error("dry run must not create index.yaml")
	}

	// Two real runs over the unchanged export are byte-identical.
	run := func() []byte {
		r, e := GenerateIndex(GenerateIndexOptions{TenantDir: tenantDir})
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
		t.Error("index.yaml must be deterministic across runs")
	}
}

func TestGenerateIndexTenantMismatch(t *testing.T) {
	tenantDir := t.TempDir()
	writeMeta(t, tenantDir, &Metadata{
		Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{}, Resources: map[string]ResourceMeta{},
	})

	_, err := GenerateIndex(GenerateIndexOptions{TenantDir: tenantDir, ExpectDomain: "other.com"})
	if !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("expected ErrTenantMismatch, got %v", err)
	}
}

func TestGenerateIndexNoMetadata(t *testing.T) {
	_, err := GenerateIndex(GenerateIndexOptions{TenantDir: t.TempDir()})
	if !errors.Is(err, ErrNoMetadata) {
		t.Fatalf("expected ErrNoMetadata, got %v", err)
	}
}
