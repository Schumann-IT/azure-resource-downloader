package consistency

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure-resource-downloader/internal/docs"
)

const (
	scType     = "Microsoft.Graph/deviceManagementConfigurationPolicies/"
	dcType     = "Microsoft.Graph/deviceConfigurations/"
	compType   = "Microsoft.Graph/deviceCompliancePolicies/"
	defenderID = "device_vendor_msft_policy_config_defender_allowrealtimemonitoring"
	admxID     = "device_vendor_msft_policy_config_admx_power_pw_promptpasswordonresume_dc_1"
	secretURI  = "./Device/Vendor/MSFT/Policy/Config/Secret/Token"
	secretText = "resolved-Plaintext-Secret-9f8e7d"
)

// syntheticTenant builds a small tenant covering every finding path.
func syntheticTenant(t *testing.T) *fixture {
	f := newFixture(t)
	f.addGroup("devices", grpDevices, `(device.deviceOSType -eq "Windows")`)
	f.addGroup("admins", grpAdmins, "")

	win := func(ts ...interface{}) docs.ResourceMeta {
		return docs.ResourceMeta{Platforms: "windows10", AssignmentTargets: ts}
	}

	// Two Settings Catalog policies on All devices: a conflict and a duplicate.
	f.add(scType+"a.yaml", win(allDevices()), catalogPolicy("windows10",
		catalogChoice(defenderID, defenderID+"_1"),
		catalogSimple("shared_value", 7),
	))
	f.add(scType+"b.yaml", win(allDevices()), catalogPolicy("windows10",
		catalogChoice(defenderID, defenderID+"_0"),
		catalogSimple("shared_value", 7),
	))

	// A custom OMA-URI profile on the bridge: the Defender value against both
	// catalog policies, an ADMX XML payload, and a secret.
	f.add(dcType+"custom.yaml", docs.ResourceMeta{
		ODataType:         "#microsoft.graph.windows10CustomConfiguration",
		AssignmentTargets: targets(include(grpDevices)),
	}, map[string]interface{}{
		"@odata.type": "#microsoft.graph.windows10CustomConfiguration",
		"omaSettings": []interface{}{
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/Defender/AllowRealtimeMonitoring", "value": 1},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/ADMX_Power/PW_PromptPasswordOnResume_DC_1", "value": "<enabled/>"},
			map[string]interface{}{"omaUri": secretURI, "value": secretText, "isEncrypted": true},
		},
	})
	f.add(scType+"admx.yaml", win(include(grpDevices)), catalogPolicy("windows10",
		catalogChoice(admxID, admxID+"_1"),
	))
	f.add(dcType+"secret_twin.yaml", docs.ResourceMeta{
		ODataType:         "#microsoft.graph.windows10CustomConfiguration",
		AssignmentTargets: targets(include(grpDevices)),
	}, map[string]interface{}{
		"@odata.type": "#microsoft.graph.windows10CustomConfiguration",
		"omaSettings": []interface{}{
			map[string]interface{}{"omaUri": secretURI, "value": secretText + "-other", "isEncrypted": true},
		},
	})

	// L1/Admin complement: one excludes the assigned admin group, its twin
	// includes only it. Different values, but they never meet.
	f.add(scType+"l1.yaml", win(allUsers(), exclude(grpAdmins)), catalogPolicy("windows10",
		catalogSimple("complement_value", 1),
	))
	f.add(scType+"admin.yaml", win(include(grpAdmins)), catalogPolicy("windows10",
		catalogSimple("complement_value", 2),
	))

	// Not indexed: absent from the tenant, filtered, skipped.
	f.addEntry(scType+"gone.yaml", docs.ResourceMeta{PresentInTenant: false, AssignmentTargets: targets(allDevices())})
	f.writeYAML(scType+"gone.yaml", catalogPolicy("windows10", catalogSimple("shared_value", 9)))
	f.addEntry(scType+"filtered.yaml", docs.ResourceMeta{PresentInTenant: true, Filtered: true, AssignmentTargets: targets(allDevices())})
	f.writeYAML(scType+"filtered.yaml", catalogPolicy("windows10", catalogSimple("shared_value", 9)))
	return f
}

func findingsByKey(fs []Finding) map[string]Finding {
	out := map[string]Finding{}
	for _, f := range fs {
		out[f.Kind+"|"+f.Key+"|"+f.A.Resource+"|"+f.B.Resource] = f
	}
	return out
}

func TestAnalyzeSyntheticTenant(t *testing.T) {
	dir := syntheticTenant(t).save()

	res, err := Analyze(Options{TenantDir: dir, ExpectDomain: "contoso.example.com", ToolVersion: "v-test"})
	if err != nil {
		t.Fatal(err)
	}
	got := findingsByKey(res.Mechanical.Findings)

	if f, ok := got["conflict|"+defenderID+"|"+scType+"a.yaml|"+scType+"b.yaml"]; !ok || f.Overlap != OverlapCertain || f.Confidence != ConfidenceFirm {
		t.Errorf("missing certain conflict between the catalog policies: %+v", res.Mechanical.Findings)
	}
	if _, ok := got["duplicate|shared_value|"+scType+"a.yaml|"+scType+"b.yaml"]; !ok {
		t.Error("missing duplicate on shared_value")
	}
	// The bridge: OMA value 1 equals option suffix _1 and differs from _0.
	if f, ok := got["duplicate|"+defenderID+"|"+dcType+"custom.yaml|"+scType+"a.yaml"]; !ok || f.Overlap != OverlapPossible {
		t.Errorf("missing bridge duplicate: %+v", res.Mechanical.Findings)
	}
	if _, ok := got["conflict|"+defenderID+"|"+dcType+"custom.yaml|"+scType+"b.yaml"]; !ok {
		t.Error("missing bridge conflict")
	}
	for k := range got {
		if strings.Contains(k, "complement_value") {
			t.Errorf("the L1/Admin complement pair must not overlap: %s", k)
		}
		if strings.Contains(k, "gone.yaml") || strings.Contains(k, "filtered.yaml") {
			t.Errorf("an absent or filtered resource must not be indexed: %s", k)
		}
	}

	unknownKeys := map[string]bool{}
	for _, u := range res.Mechanical.UnknownValues {
		unknownKeys[u.Key] = true
	}
	if !unknownKeys[admxID] {
		t.Errorf("a non-scalar bridge side belongs in unknownValues: %+v", res.Mechanical.UnknownValues)
	}
	if !unknownKeys[normaliseOMAURI(secretURI)] {
		t.Errorf("a secret pair belongs in unknownValues: %+v", res.Mechanical.UnknownValues)
	}

	c := res.Metadata.Counts
	if c.Indexed[typeSettingsCatalog].Resources != 5 || c.Indexed[typeDeviceConfigurations].Resources != 2 {
		t.Errorf("indexed counts: %+v", c.Indexed)
	}
	if c.Findings[KindConflict][OverlapCertain] != 1 || c.UnknownValues != len(res.Mechanical.UnknownValues) {
		t.Errorf("finding counts: %+v", c)
	}
	if res.Metadata.ExportGeneratedAt != "2026-01-01T00:00:00Z" || !res.Metadata.ExportComplete || res.Metadata.ToolVersion != "v-test" {
		t.Errorf("metadata: %+v", res.Metadata)
	}

	// Both files are on disk; a resolved secret never is.
	for _, name := range []string{MechanicalFileName, MetadataFileName} {
		data, err := os.ReadFile(filepath.Join(dir, DirName, name))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(secretText)) {
			t.Errorf("%s leaks a resolved secret", name)
		}
	}
}

func TestAnalyzeRerunIsByteEqual(t *testing.T) {
	dir := syntheticTenant(t).save()
	read := func() string {
		var b strings.Builder
		for _, name := range []string{MechanicalFileName, MetadataFileName} {
			data, err := os.ReadFile(filepath.Join(dir, DirName, name))
			if err != nil {
				t.Fatal(err)
			}
			b.Write(data)
		}
		return b.String()
	}
	if _, err := Analyze(Options{TenantDir: dir, ToolVersion: "v"}); err != nil {
		t.Fatal(err)
	}
	first := read()
	if _, err := Analyze(Options{TenantDir: dir, ToolVersion: "v"}); err != nil {
		t.Fatal(err)
	}
	if read() != first {
		t.Error("a rerun over an unchanged export must be byte-equal")
	}
}

func TestAnalyzeDryRunWritesNothing(t *testing.T) {
	dir := syntheticTenant(t).save()
	res, err := Analyze(Options{TenantDir: dir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Mechanical.Findings) == 0 {
		t.Error("a dry run still computes the findings")
	}
	if _, err := os.Stat(filepath.Join(dir, DirName)); !os.IsNotExist(err) {
		t.Error("a dry run must not create the consistency tree")
	}
}

func TestAnalyzeLeavesOtherFilesAlone(t *testing.T) {
	dir := syntheticTenant(t).save()
	other := filepath.Join(dir, DirName, "index.md")
	if err := os.MkdirAll(filepath.Dir(other), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("agent"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(Options{TenantDir: dir}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(other); err != nil || string(data) != "agent" {
		t.Error("other files in consistency/ must be left alone")
	}
}

func TestAnalyzeUnreadableResource(t *testing.T) {
	f := newFixture(t)
	f.add(scType+"ok.yaml", docs.ResourceMeta{Platforms: "windows10"}, catalogPolicy("windows10", catalogSimple("x", 1)))
	f.add(scType+"missing.yaml", docs.ResourceMeta{Platforms: "windows10"}, nil)
	f.add(scType+"broken.yaml", docs.ResourceMeta{Platforms: "windows10"}, nil)
	f.writeRaw(scType+"broken.yaml", []byte("settings: [unclosed\n  - : :"))
	dir := f.save()

	res, err := Analyze(Options{TenantDir: dir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata.Counts.Unreadable != 2 || len(res.Unreadable) != 2 {
		t.Errorf("want 2 unreadable, got %v", res.Unreadable)
	}
	if res.Metadata.Counts.Indexed[typeSettingsCatalog].Resources != 1 {
		t.Error("the readable resource is still indexed")
	}
}

func TestAnalyzeRefusals(t *testing.T) {
	t.Run("no metadata", func(t *testing.T) {
		_, err := Analyze(Options{TenantDir: t.TempDir()})
		if !errors.Is(err, docs.ErrNoMetadata) {
			t.Errorf("want ErrNoMetadata, got %v", err)
		}
	})
	t.Run("tenant mismatch", func(t *testing.T) {
		dir := newFixture(t).save()
		_, err := Analyze(Options{TenantDir: dir, ExpectDomain: "fabrikam.example.com"})
		if !errors.Is(err, docs.ErrTenantMismatch) {
			t.Errorf("want ErrTenantMismatch, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, DirName)); !os.IsNotExist(err) {
			t.Error("a refusal writes nothing")
		}
	})
	t.Run("write failure", func(t *testing.T) {
		dir := newFixture(t).save()
		// A file where the tree must go makes the directory impossible.
		if err := os.WriteFile(filepath.Join(dir, DirName), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := Analyze(Options{TenantDir: dir})
		if !errors.Is(err, ErrWrite) {
			t.Errorf("want ErrWrite, got %v", err)
		}
	})
}

// TestEquivalenceAliasHook joins a macOS compliance password length and a
// configuration profile's through a fixture equivalence table.
func TestEquivalenceAliasHook(t *testing.T) {
	const (
		complianceKey = "#microsoft.graph.macOSCompliancePolicy#passwordMinimumLength"
		configKey     = "com.apple.mobiledevice.passwordpolicy_minlength"
	)
	build := func(t *testing.T, configured int) string {
		f := newFixture(t)
		f.add(compType+"mac.yaml", docs.ResourceMeta{
			ODataType:         "#microsoft.graph.macOSCompliancePolicy",
			AssignmentTargets: targets(allDevices()),
		}, map[string]interface{}{"@odata.type": "#microsoft.graph.macOSCompliancePolicy", "passwordMinimumLength": 12})
		f.add(scType+"mac.yaml", docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(allDevices())},
			catalogPolicy("macOS", catalogSimple(configKey, configured)))
		return f.save()
	}
	equivalence := func(enforced bool, status string) []Equivalence {
		return []Equivalence{{
			ID:       "macos-password-length",
			Members:  []string{complianceKey, configKey},
			Relation: RelationAtLeast,
			Enforced: map[string]bool{PlatformMacOS: enforced},
			Status:   status,
		}}
	}

	tests := []struct {
		name       string
		configured int
		eqs        []Equivalence
		kind       string
		confidence string
	}{
		{"no catalog: exact keys only", 8, nil, "", ""},
		{"both enforce, values differ", 8, equivalence(true, StatusVerified), KindConflict, ConfidenceFirm},
		{"both enforce, values equal", 12, equivalence(true, StatusVerified), KindDuplicate, ConfidenceFirm},
		{"configuration fails the requirement", 8, equivalence(false, StatusVerified), KindContradiction, ConfidenceFirm},
		{"configuration meets the requirement", 14, equivalence(false, StatusVerified), "", ""},
		{"verify equivalence is never firm", 8, equivalence(false, StatusVerify), KindContradiction, ConfidencePossible},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Analyze(Options{TenantDir: build(t, tt.configured), Equivalences: tt.eqs, DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			fs := res.Mechanical.Findings
			if tt.kind == "" {
				if len(fs) != 0 {
					t.Errorf("want no finding, got %+v", fs)
				}
				return
			}
			if len(fs) != 1 {
				t.Fatalf("want one finding, got %+v", fs)
			}
			f := fs[0]
			if f.Kind != tt.kind || f.Confidence != tt.confidence || f.Key != "equivalence:macos-password-length" || f.Operator != ">=" || f.Overlap != OverlapCertain {
				t.Errorf("got %+v", f)
			}
		})
	}
}

func TestClearTree(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"resources", "docs", "drift", DirName} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := ClearTree(dir)
	if err != nil || !removed {
		t.Fatalf("want removed, got %v %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, DirName)); !os.IsNotExist(err) {
		t.Error("the consistency tree must be gone")
	}
	for _, sub := range []string{"resources", "docs", "drift"} {
		if _, err := os.Stat(filepath.Join(dir, sub)); err != nil {
			t.Errorf("sibling %s must survive", sub)
		}
	}
	removed, err = ClearTree(dir)
	if err != nil || removed {
		t.Errorf("no tree is a no-op, got %v %v", removed, err)
	}
}

// Example shows the analysis over a two-policy export.
func Example() {
	dir, _ := os.MkdirTemp("", "consistency-example")
	defer func() { _ = os.RemoveAll(dir) }()

	write := func(rel, content string) {
		p := filepath.Join(dir, "resources", filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0755)
		_ = os.WriteFile(p, []byte(content), 0644)
	}
	policy := func(v int) string {
		return fmt.Sprintf("settings:\n  - settingInstance:\n      settingDefinitionId: example_setting\n      simpleSettingValue:\n        value: %d\n", v)
	}
	write("Microsoft.Graph/deviceManagementConfigurationPolicies/a.yaml", policy(1))
	write("Microsoft.Graph/deviceManagementConfigurationPolicies/b.yaml", policy(2))
	entry := "    presentInTenant: true\n    platforms: macOS\n    assignmentTargets:\n      - target:\n          '@odata.type': '#microsoft.graph.allDevicesAssignmentTarget'\n"
	write("metadata.yaml", "tenant: contoso.example.com\nresources:\n"+
		"  Microsoft.Graph/deviceManagementConfigurationPolicies/a.yaml:\n"+entry+
		"  Microsoft.Graph/deviceManagementConfigurationPolicies/b.yaml:\n"+entry)

	res, err := Analyze(Options{TenantDir: dir, DryRun: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, f := range res.Mechanical.Findings {
		fmt.Println(f.Kind, f.Key, f.Overlap, f.A.Value, f.B.Value)
	}
	// Output: conflict example_setting certain 1 2
}

// analyzePair runs a dry analysis of two Settings Catalog policies on All
// devices with the given platforms.
func analyzePair(t *testing.T, platformA string, docA map[string]interface{}, platformB string, docB map[string]interface{}) *Result {
	t.Helper()
	f := newFixture(t)
	f.add(scType+"a.yaml", docs.ResourceMeta{Platforms: platformA, AssignmentTargets: targets(allDevices())}, docA)
	f.add(scType+"b.yaml", docs.ResourceMeta{Platforms: platformB, AssignmentTargets: targets(allDevices())}, docB)
	res, err := Analyze(Options{TenantDir: f.save(), DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestAnalyzeAppleCollectionsAreAdditive(t *testing.T) {
	sharedRule := `[{"` + ruleType + `":"TeamIdentifier","` + ruleValue + `":"TEAMS"}]`
	tests := []struct {
		name     string
		platform string
		a, b     []string
		kind     string
		value    string
	}{
		{"macOS different rules: no finding", "macOS", []string{"TEAMA"}, []string{"TEAMB"}, "", ""},
		{"macOS one shared rule: duplicate of it only", "macOS", []string{"TEAMA", "TEAMS"}, []string{"TEAMS", "TEAMB"}, KindDuplicate, sharedRule},
		{"iOS different rules: no finding", "iOS", []string{"TEAMA"}, []string{"TEAMB"}, "", ""},
		{"iOS one shared rule: duplicate", "iOS", []string{"TEAMS"}, []string{"TEAMS", "TEAMB"}, KindDuplicate, sharedRule},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := analyzePair(t, tt.platform, loginItemsPolicy(tt.platform, tt.a...), tt.platform, loginItemsPolicy(tt.platform, tt.b...))
			var got []Finding
			for _, f := range res.Mechanical.Findings {
				if f.Key == loginRules || strings.HasPrefix(f.Key, loginRules+"_") {
					got = append(got, f)
				}
			}
			if tt.kind == "" {
				if len(got) != 0 {
					t.Errorf("want no collection finding, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Kind != tt.kind || got[0].Key != loginRules || got[0].A.Value != tt.value || got[0].B.Value != tt.value {
				t.Errorf("want one %s on %s valued %s, got %+v", tt.kind, loginRules, tt.value, got)
			}
			if len(res.Mechanical.UnknownValues) != 0 {
				t.Errorf("no unknown values expected: %+v", res.Mechanical.UnknownValues)
			}
		})
	}
}

func TestAnalyzeApplePayloadKeyStillConflicts(t *testing.T) {
	const textKey = "com.apple.loginwindow_loginwindowtext"
	policy := func(text string) map[string]interface{} {
		return catalogPolicy("macOS", applePayload("com.apple.loginwindow", simpleInstance(textKey, text)))
	}
	res := analyzePair(t, "macOS", policy("one"), "macOS", policy("two"))
	got := findingsByKey(res.Mechanical.Findings)
	if _, ok := got["conflict|"+textKey+"|"+scType+"a.yaml|"+scType+"b.yaml"]; !ok {
		t.Errorf("a payload-level key set differently must still conflict: %+v", res.Mechanical.Findings)
	}
}

func TestAnalyzeWindowsNestedCollectionStillConflicts(t *testing.T) {
	res := analyzePair(t, "windows10", loginItemsPolicy("windows10", "TEAMA"), "windows10", loginItemsPolicy("windows10", "TEAMB"))
	got := findingsByKey(res.Mechanical.Findings)
	if _, ok := got["conflict|"+ruleValue+"|"+scType+"a.yaml|"+scType+"b.yaml"]; !ok {
		t.Errorf("a non-Apple nested collection keeps the folded comparison: %+v", res.Mechanical.Findings)
	}
}

func TestAnalyzeAppleUnknownMembers(t *testing.T) {
	const memberSecret = "resolved-Member-Secret-1a2b3c"
	secretRule := children(map[string]interface{}{
		"settingDefinitionId": ruleValue,
		"simpleSettingValue": map[string]interface{}{
			"@odata.type": "#microsoft.graph.deviceManagementConfigurationSecretSettingValue",
			"value":       memberSecret,
			"valueState":  "notEncrypted",
		},
	})
	withSecret := catalogPolicy("macOS", applePayload(loginPayload, groupCollection(loginRules, loginRule("TEAMA"), secretRule)))

	t.Run("disjoint pair with a secret member is listed, never written", func(t *testing.T) {
		f := newFixture(t)
		f.add(scType+"a.yaml", docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(allDevices())}, withSecret)
		f.add(scType+"b.yaml", docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(allDevices())}, loginItemsPolicy("macOS", "TEAMB"))
		dir := f.save()
		res, err := Analyze(Options{TenantDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Mechanical.Findings) != 0 {
			t.Errorf("want no finding, got %+v", res.Mechanical.Findings)
		}
		if len(res.Mechanical.UnknownValues) != 1 || res.Mechanical.UnknownValues[0].Key != loginRules {
			t.Errorf("want the pair under unknownValues, got %+v", res.Mechanical.UnknownValues)
		}
		for _, name := range []string{MechanicalFileName, MetadataFileName} {
			data, err := os.ReadFile(filepath.Join(dir, DirName, name))
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte(memberSecret)) {
				t.Errorf("%s leaks a secret member", name)
			}
		}
	})

	t.Run("a shared known member is still a duplicate", func(t *testing.T) {
		res := analyzePair(t, "macOS", withSecret, "macOS", loginItemsPolicy("macOS", "TEAMA"))
		if len(res.Mechanical.Findings) != 1 || res.Mechanical.Findings[0].Kind != KindDuplicate {
			t.Errorf("want one duplicate, got %+v", res.Mechanical.Findings)
		}
		if strings.Contains(res.Mechanical.Findings[0].A.Value, memberSecret) {
			t.Error("an unknown member is never written")
		}
	})

	t.Run("one member list against one folded value", func(t *testing.T) {
		res := analyzePair(t, "macOS", withSecret, "macOS, windows10", catalogPolicy("macOS, windows10",
			applePayload(loginPayload, map[string]interface{}{
				"settingDefinitionId": loginRules,
				"simpleSettingValue":  map[string]interface{}{"value": "x"},
			})))
		if len(res.Mechanical.Findings) != 0 || len(res.Mechanical.UnknownValues) != 1 {
			t.Errorf("a mixed pair is listed under unknownValues: %+v %+v", res.Mechanical.Findings, res.Mechanical.UnknownValues)
		}
	})
}

func TestAnalyzeRuledOutByScope(t *testing.T) {
	build := func(t *testing.T) string {
		f := newFixture(t)
		f.addGroup("admins", grpAdmins, "")
		win := func(ts ...interface{}) docs.ResourceMeta {
			return docs.ResourceMeta{Platforms: "windows10", AssignmentTargets: ts}
		}
		// The complement pair shares two keys: ruled out once.
		f.add(scType+"l1.yaml", win(allUsers(), exclude(grpAdmins)), catalogPolicy("windows10",
			catalogSimple("first_value", 1), catalogSimple("second_value", 1)))
		f.add(scType+"admin.yaml", win(include(grpAdmins)), catalogPolicy("windows10",
			catalogSimple("first_value", 2), catalogSimple("second_value", 2)))
		// A macOS policy never meets either and shares no key: not counted.
		f.add(scType+"mac.yaml", docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(allDevices())},
			catalogPolicy("macOS", catalogSimple("mac_only", 1)))
		return f.save()
	}

	dir := build(t)
	dry, err := Analyze(Options{TenantDir: dir, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, 1, dry.Metadata.Counts.RuledOutByScope, "a pair ruled out on two keys counts once")
	equal(t, 0, len(dry.Mechanical.Findings))

	read := func() string {
		data, err := os.ReadFile(filepath.Join(dir, DirName, MetadataFileName))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	res, err := Analyze(Options{TenantDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	equal(t, dry.Metadata.Counts.RuledOutByScope, res.Metadata.Counts.RuledOutByScope, "a dry run reports the same count")
	first := read()
	if !strings.Contains(first, "ruledOutByScope: 1\n") {
		t.Errorf("metadata.yaml must carry the count: %s", first)
	}
	if _, err := Analyze(Options{TenantDir: dir}); err != nil {
		t.Fatal(err)
	}
	equal(t, first, read(), "reruns stay byte-equal")

	t.Run("always written, zero included", func(t *testing.T) {
		f := newFixture(t)
		f.add(scType+"a.yaml", docs.ResourceMeta{Platforms: "windows10"}, catalogPolicy("windows10", catalogSimple("x", 1)))
		dir := f.save()
		if _, err := Analyze(Options{TenantDir: dir}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(dir, DirName, MetadataFileName))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "ruledOutByScope: 0\n") {
			t.Errorf("a zero count is written: %s", data)
		}
	})
}

func TestAnalyzeTypedEnumDefaultsAreNotConfigured(t *testing.T) {
	const odata = "#microsoft.graph.windows10CompliancePolicy"
	policy := func(pw string) map[string]interface{} {
		return map[string]interface{}{
			"@odata.type": odata,
			"deviceThreatProtectionRequiredSecurityLevel": "unavailable",
			"passwordRequiredType":                        pw,
		}
	}
	run := func(a, b string) *Result {
		f := newFixture(t)
		meta := docs.ResourceMeta{Platforms: "windows10", AssignmentTargets: targets(allDevices())}
		f.add(compType+"a.yaml", meta, policy(a))
		f.add(compType+"b.yaml", meta, policy(b))
		res, err := Analyze(Options{TenantDir: f.save(), DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	for _, tt := range []struct {
		name, a, b string
		want       []string
	}{
		{"both defaults: no finding", "deviceDefault", "deviceDefault", nil},
		{"one default side: no finding", "alphanumeric", "deviceDefault", nil},
		{"both alphanumeric: duplicate", "alphanumeric", "alphanumeric", []string{KindDuplicate}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res := run(tt.a, tt.b)
			var kinds []string
			for _, f := range res.Mechanical.Findings {
				kinds = append(kinds, f.Kind)
			}
			if strings.Join(kinds, ",") != strings.Join(tt.want, ",") {
				t.Errorf("want %v, got %+v", tt.want, res.Mechanical.Findings)
			}
			equal(t, 0, len(res.Mechanical.UnknownValues))
		})
	}
}
