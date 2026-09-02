package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure-resource-downloader/internal/models"
)

const templateType = notificationMessageTemplatesType

func TestUsedBySha256DetectsChanges(t *testing.T) {
	rows := []usedByRow{
		{resourceType: compType, sourceKey: compType + "/a.yaml", resourceName: "Alpha"},
	}
	base := usedBySha256(rows)

	// The empty set has a stable, non-empty hash distinct from a populated one.
	if usedBySha256(nil) == base {
		t.Error("empty used-by set must not hash like a populated one")
	}
	if usedBySha256(nil) != usedBySha256([]usedByRow{}) {
		t.Error("empty used-by hash must be stable")
	}

	// Adding a referencing resource changes the hash.
	more := append([]usedByRow{}, rows...)
	more = append(more, usedByRow{resourceType: compType, sourceKey: compType + "/b.yaml", resourceName: "Beta"})
	if usedBySha256(more) == base {
		t.Error("adding a referencing resource must change the used-by hash")
	}

	// Renaming a referencing resource changes the hash.
	renamed := []usedByRow{{resourceType: compType, sourceKey: compType + "/a.yaml", resourceName: "Alpha RENAMED"}}
	if usedBySha256(renamed) == base {
		t.Error("renaming a referencing resource must change the used-by hash")
	}

	// Order independence.
	orderA := usedBySha256(more)
	swapped := []usedByRow{more[1], more[0]}
	if usedBySha256(swapped) != orderA {
		t.Error("used-by hash must be independent of row order")
	}
}

func TestBuildUsedByTemplateAndDangling(t *testing.T) {
	m := &Metadata{
		Resources: map[string]ResourceMeta{
			// A present policy references T1 and a missing template.
			compType + "/p1.yaml": {
				ResourceId: "p1", DisplayName: "P1", PresentInTenant: true,
				NotificationTemplateRefs: []string{"T1", "T-missing"},
			},
			// A gone policy referencing T1 must not count.
			compType + "/p2.yaml": {
				ResourceId: "p2", DisplayName: "P2", PresentInTenant: false,
				NotificationTemplateRefs: []string{"T1"},
			},
			templateType + "/t1.yaml": {ResourceId: "T1", DisplayName: "Compliance email", PresentInTenant: true},
		},
	}

	usedBy := buildUsedByTemplate(m)
	if len(usedBy["T1"]) != 1 || usedBy["T1"][0].sourceKey != compType+"/p1.yaml" {
		t.Errorf("T1 used-by = %+v, want only the present policy", usedBy["T1"])
	}
	// A template must never be a source of a reference.
	if len(usedBy["T-missing"]) != 1 {
		t.Errorf("dangling template still tracked as referenced: %+v", usedBy["T-missing"])
	}

	dangling := danglingTemplateIDs(m)
	if len(dangling) != 1 || dangling[0] != "T-missing" {
		t.Errorf("danglingTemplateIDs = %v, want [T-missing]", dangling)
	}
}

// templateScenario builds a metadata + tree with one present compliance policy
// referencing a present template T1, both with current source/prompt docs, and
// the template document correctly used-by-hashed so it never lands in a list
// unless a test intends it to.
func templateScenario(t *testing.T) (string, *Metadata) {
	t.Helper()
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:     {PromptSha256: "p-comp"},
			templateType: {PromptSha256: "p-tmpl"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: "pol", DisplayName: "Policy", SourceSha256: "s-pol", PresentInTenant: true,
				NotificationTemplateRefs: []string{"T1"},
			},
			templateType + "/t1.yaml": {
				ResourceId: "T1", DisplayName: "Compliance email", SourceSha256: "s-t1", PresentInTenant: true,
			},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, templateType)
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp"})
	writeDocFM(t, tenantDir, templateType+"/t1.yaml", docFM{srcSha: "s-t1", promptSha: "p-tmpl", usedBySha: usedByHash(m, "T1")})
	return tenantDir, m
}

func usedByHash(m *Metadata, templateID string) string {
	return usedBySha256(buildUsedByTemplate(m)[templateID])
}

func TestGeneratePromptUsedByResplice(t *testing.T) {
	tenantDir, m := templateScenario(t)
	// The template document's used-by hash is stale.
	writeDocFM(t, tenantDir, templateType+"/t1.yaml", docFM{srcSha: "s-t1", promptSha: "p-tmpl", usedBySha: "STALE"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.ToGenerate) != 0 || len(res.ForwardResplice) != 0 || len(res.ReverseResplice) != 0 {
		t.Errorf("only the template's used-by block is stale: gen=%+v fwd=%+v rev=%+v", res.ToGenerate, res.ForwardResplice, res.ReverseResplice)
	}
	usedby := docPaths(res.UsedByResplice)
	it, ok := usedby["docs/"+templateType+"/t1.md"]
	if !ok {
		t.Fatalf("template must be in UsedByResplice, got %+v", res.UsedByResplice)
	}
	if it.Hash != usedByHash(m, "T1") {
		t.Errorf("used-by hash = %q, want %q", it.Hash, usedByHash(m, "T1"))
	}

	// Rewriting the doc with the correct hash clears the used-by re-splice.
	writeDocFM(t, tenantDir, templateType+"/t1.yaml", docFM{srcSha: "s-t1", promptSha: "p-tmpl", usedBySha: usedByHash(m, "T1")})
	res, err = GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt (rerun): %v", err)
	}
	if len(res.UsedByResplice) != 0 {
		t.Errorf("matching hash must clear UsedByResplice: %+v", res.UsedByResplice)
	}
}

func TestNotificationRefsSha256DetectsRename(t *testing.T) {
	names := map[string]string{"T1": "Compliance email"}
	base := notificationRefsSha256(names, []string{"T1"})

	// Renaming the referenced template moves the hash.
	if notificationRefsSha256(map[string]string{"T1": "Renamed"}, []string{"T1"}) == base {
		t.Error("renaming a referenced template must change the forward hash")
	}
	// A dangling reference (template absent) differs from a resolved one.
	if notificationRefsSha256(map[string]string{}, []string{"T1"}) == base {
		t.Error("a dangling reference must hash differently from a resolved one")
	}
	// Order independence and empty-set stability.
	multi := notificationRefsSha256(names, []string{"T1", "T2"})
	if notificationRefsSha256(names, []string{"T2", "T1"}) != multi {
		t.Error("forward hash must be independent of ref order")
	}
	if notificationRefsSha256(names, nil) != notificationRefsSha256(names, []string{}) {
		t.Error("empty ref hash must be stable")
	}
}

func notifHash(m *Metadata, refs []string) string {
	return notificationRefsSha256(templateNames(m), refs)
}

// policyTemplateScenario builds a compliance policy referencing template T1,
// both present with current source/prompt docs and the template's used-by hash
// correct, so nothing lands in a list unless a test dirties it.
func policyTemplateScenario(t *testing.T) (string, *Metadata) {
	t.Helper()
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:     {PromptSha256: "p-comp"},
			templateType: {PromptSha256: "p-tmpl"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: "pol", DisplayName: "Policy", SourceSha256: "s-pol", PresentInTenant: true,
				NotificationTemplateRefs: []string{"T1"},
			},
			templateType + "/t1.yaml": {
				ResourceId: "T1", DisplayName: "Compliance email", SourceSha256: "s-t1", PresentInTenant: true,
			},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, templateType)
	writeDocFM(t, tenantDir, templateType+"/t1.yaml", docFM{srcSha: "s-t1", promptSha: "p-tmpl", usedBySha: usedByHash(m, "T1")})
	return tenantDir, m
}

func TestGeneratePromptNotificationsResplice(t *testing.T) {
	tenantDir, m := policyTemplateScenario(t)
	// Policy doc is current and carries the marker, but its notifications hash
	// is stale (the referenced template was renamed since it was written).
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", withNotificationsMarker: true, notificationsSha: "STALE"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.ToGenerate) != 0 || len(res.Migrate) != 0 || len(res.ForwardResplice) != 0 {
		t.Errorf("only the notifications block is stale: gen=%+v migrate=%+v fwd=%+v", res.ToGenerate, res.Migrate, res.ForwardResplice)
	}
	it, ok := docPaths(res.NotificationsResplice)["docs/"+compType+"/policy.md"]
	if !ok {
		t.Fatalf("policy must be in NotificationsResplice, got %+v", res.NotificationsResplice)
	}
	if it.Hash != notifHash(m, []string{"T1"}) {
		t.Errorf("notifications hash = %q, want %q", it.Hash, notifHash(m, []string{"T1"}))
	}

	// Writing the correct hash clears the re-splice.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp", withNotificationsMarker: true, notificationsSha: notifHash(m, []string{"T1"})})
	res, err = GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt (rerun): %v", err)
	}
	if len(res.NotificationsResplice) != 0 {
		t.Errorf("matching hash must clear NotificationsResplice: %+v", res.NotificationsResplice)
	}
}

func TestGeneratePromptNotificationsMigrate(t *testing.T) {
	tenantDir, m := policyTemplateScenario(t)
	// Policy doc is current but predates the notification markers.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.NotificationsResplice) != 0 {
		t.Errorf("a document without the marker migrates, not re-splices: %+v", res.NotificationsResplice)
	}
	var found *WorkItem
	for i := range res.Migrate {
		if res.Migrate[i].DocPath == "docs/"+compType+"/policy.md" {
			found = &res.Migrate[i]
		}
	}
	if found == nil {
		t.Fatalf("policy must be in Migrate, got %+v", res.Migrate)
	}
	if found.NotificationsSha256 != notifHash(m, []string{"T1"}) {
		t.Errorf("migrate item must carry the forward hash, got %q", found.NotificationsSha256)
	}
}

func TestGeneratePromptList1CarriesUsedBySha256(t *testing.T) {
	// Build a scenario where the template has no document (list 1) but the
	// referencing policy does, so only the template lands in ToGenerate.
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:     {PromptSha256: "p-comp"},
			templateType: {PromptSha256: "p-tmpl"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: "pol", DisplayName: "Policy", SourceSha256: "s-pol", PresentInTenant: true,
				NotificationTemplateRefs: []string{"T1"},
			},
			templateType + "/t1.yaml": {
				ResourceId: "T1", DisplayName: "Compliance email", SourceSha256: "s-t1", PresentInTenant: true,
			},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, templateType)
	// Policy has a current doc; template has none → template lands in list 1.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	var found *WorkItem
	for i := range res.ToGenerate {
		if res.ToGenerate[i].DocPath == "docs/"+templateType+"/t1.md" {
			found = &res.ToGenerate[i]
		}
	}
	if found == nil {
		t.Fatalf("template must be in ToGenerate (no doc), got %+v", res.ToGenerate)
	}
	want := usedByHash(m, "T1")
	if found.UsedBySha256 != want {
		t.Errorf("UsedBySha256 = %q, want %q", found.UsedBySha256, want)
	}
}

func TestGeneratePromptList1CarriesNotificationsSha256(t *testing.T) {
	tenantDir, m := policyTemplateScenario(t)
	// Policy document is missing — it lands in list 1 (no doc).
	// The work item must carry NotificationsSha256.
	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: true})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	var found *WorkItem
	for i := range res.ToGenerate {
		if res.ToGenerate[i].DocPath == "docs/"+compType+"/policy.md" {
			found = &res.ToGenerate[i]
		}
	}
	if found == nil {
		t.Fatalf("policy must be in ToGenerate (no doc), got %+v", res.ToGenerate)
	}
	want := notifHash(m, []string{"T1"})
	if found.NotificationsSha256 != want {
		t.Errorf("NotificationsSha256 = %q, want %q", found.NotificationsSha256, want)
	}
}

func TestGeneratePromptUsedByMap(t *testing.T) {
	tenantDir := t.TempDir()
	resourcesDir := filepath.Join(tenantDir, models.ResourcesDirName)
	m := &Metadata{
		GeneratedAt: "2026-01-02T03:04:05Z", Tenant: "example.com", Run: RunMeta{Complete: true},
		Types: map[string]TypeMeta{
			compType:     {PromptSha256: "p-comp"},
			templateType: {PromptSha256: "p-tmpl"},
		},
		Resources: map[string]ResourceMeta{
			compType + "/policy.yaml": {
				ResourceId: "pol", DisplayName: "OS validation", SourceSha256: "s-pol", PresentInTenant: true,
				NotificationTemplateRefs: []string{"T1", "T-missing"},
			},
			templateType + "/t1.yaml": {ResourceId: "T1", DisplayName: "Compliance email", SourceSha256: "s-t1", PresentInTenant: true},
			// An unreferenced template is still documented and listed as such.
			templateType + "/t2.yaml": {ResourceId: "T2", DisplayName: "Unused email", SourceSha256: "s-t2", PresentInTenant: true},
		},
	}
	writeMeta(t, tenantDir, m)
	writePromptFile(t, resourcesDir, compType)
	writePromptFile(t, resourcesDir, templateType)
	// Keep the referencing policy current so it does not clutter list 1.
	writeDocFM(t, tenantDir, compType+"/policy.yaml", docFM{srcSha: "s-pol", promptSha: "p-comp"})

	res, err := GeneratePrompt(GeneratePromptOptions{TenantDir: tenantDir, Template: DefaultGeneratePromptTemplate(), DryRun: false})
	if err != nil {
		t.Fatalf("GeneratePrompt: %v", err)
	}
	if len(res.DanglingTemplateIDs) != 1 || res.DanglingTemplateIDs[0] != "T-missing" {
		t.Errorf("DanglingTemplateIDs = %v, want [T-missing]", res.DanglingTemplateIDs)
	}

	body, err := os.ReadFile(res.OutPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, "[Compliance email](docs/"+templateType+"/t1.md) — used by 1 resource(s)") {
		t.Errorf("usedbymap must resolve the referenced template and its user:\n%s", s)
	}
	if !strings.Contains(s, "[OS validation](docs/"+compType+"/policy.md)") {
		t.Error("usedbymap must link the referencing resource")
	}
	if !strings.Contains(s, "[Unused email](docs/"+templateType+"/t2.md) — not referenced by any resource") {
		t.Error("usedbymap must list an unreferenced template as such")
	}
	if !strings.Contains(s, "T-missing` → ⚠️ not in export") {
		t.Error("usedbymap must flag the dangling template")
	}
}
