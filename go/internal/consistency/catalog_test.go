package consistency

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"azure-resource-downloader/internal/docs"

	"gopkg.in/yaml.v3"
)

const (
	learnRef       = "https://learn.microsoft.com/en-us/mem/intune/protect/compliance-policy-create-mac-os"
	macCompliance  = "#microsoft.graph.macOSCompliancePolicy#passwordMinimumLength"
	macCatalogKey  = "com.apple.mobiledevice.passwordpolicy_minlength"
	macRestriction = "#microsoft.graph.macOSGeneralDeviceConfiguration#passwordMinimumLength"
)

// validCatalog returns a small catalog that compiles; each test breaks one
// thing in its own copy.
func validCatalog() CatalogConfig {
	return CatalogConfig{
		Version: 1,
		Equivalences: []EquivalenceConfig{{
			ID:          "macos-password-minimum-length",
			Description: "Minimum password length on macOS",
			Members:     []string{macCompliance, macCatalogKey, macRestriction},
			Relation:    ">=",
			Enforced:    map[string]bool{"macos": false},
			Reference:   learnRef,
			Status:      StatusVerified,
		}},
		Topics: []TopicConfig{
			{ID: "encryption", Label: "Encryption", Match: []TopicRule{{Key: "(?:filevault|bitlocker)"}, {Name: "encrypt"}}},
			{ID: "macos-passwords", Label: "macOS passwords", Match: []TopicRule{{Platforms: "macos", Key: "password"}}},
		},
		Rules: []RuleConfig{{
			ID:          "r1-encryption",
			Description: "Compliance requires encryption that no configuration enables",
			Left:        Selector{Topic: "encryption", Class: "requirement"},
			Right:       Selector{Keys: []string{macCatalogKey}, Class: "configuration", Platforms: "macos"},
			Violation:   "required but not configured",
			Resolution:  "the configuration enables it",
			Reference:   learnRef,
			Status:      StatusVerify,
		}},
	}
}

func TestCompileCatalogValid(t *testing.T) {
	c, err := CompileCatalog(validCatalog())
	if err != nil {
		t.Fatalf("CompileCatalog() = %v, want nil", err)
	}
	eqs := c.Equivalences()
	if len(eqs) != 1 {
		t.Fatalf("want one equivalence, got %+v", eqs)
	}
	want := Equivalence{
		ID:       "macos-password-minimum-length",
		Members:  []string{macCompliance, macRestriction, macCatalogKey},
		Relation: RelationAtLeast,
		Enforced: map[string]bool{PlatformMacOS: false},
		Status:   StatusVerified,
	}
	if !reflect.DeepEqual(eqs[0], want) {
		t.Errorf("equivalence = %+v, want %+v", eqs[0], want)
	}
	if got := c.Counts(); got != (CatalogCounts{Equivalences: 1, Topics: 2, Rules: 1}) {
		t.Errorf("counts = %+v", got)
	}
	if got := c.VerifyCount(); got != 1 {
		t.Errorf("verify count = %d, want 1 (the rule)", got)
	}
	if len(c.SHA256()) != 64 {
		t.Errorf("sha256 = %q, want a hex digest", c.SHA256())
	}
}

func TestCompileCatalogValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *CatalogConfig)
		want   []string
	}{
		{"version below one", func(c *CatalogConfig) { c.Version = 0 }, []string{"version must be >= 1"}},
		{"unknown relation", func(c *CatalogConfig) { c.Equivalences[0].Relation = ">" },
			[]string{"equivalences", "macos-password-minimum-length", `unknown relation ">"`}},
		{"one-member equivalence", func(c *CatalogConfig) { c.Equivalences[0].Members = []string{macCatalogKey, macCatalogKey} },
			[]string{"equivalences", "macos-password-minimum-length", "at least two distinct members"}},
		{"raw OMA-URI member", func(c *CatalogConfig) {
			c.Equivalences[0].Members[1] = "./Device/Vendor/MSFT/Policy/Config/DeviceLock/MinDevicePasswordLength"
		}, []string{"equivalences", "macos-password-minimum-length", `use "device_vendor_msft_policy_config_devicelock_mindevicepasswordlength"`}},
		{"mixed-case Settings Catalog member", func(c *CatalogConfig) {
			c.Equivalences[0].Members[1] = "com.apple.MobileDevice.passwordpolicy_minlength"
		}, []string{`use "com.apple.mobiledevice.passwordpolicy_minlength"`}},
		{"typed member without a path", func(c *CatalogConfig) { c.Equivalences[0].Members[0] = "#microsoft.graph.macOSCompliancePolicy#" },
			[]string{"is not a canonical key"}},
		{"unknown enforced platform", func(c *CatalogConfig) { c.Equivalences[0].Enforced = map[string]bool{"chromeos": true} },
			[]string{"equivalences", `unknown platform "chromeos"`}},
		{"non-Learn reference", func(c *CatalogConfig) { c.Equivalences[0].Reference = "https://example.com/blog" },
			[]string{"equivalences", "must start with https://learn.microsoft.com/"}},
		{"unknown status", func(c *CatalogConfig) { c.Equivalences[0].Status = "done" },
			[]string{"equivalences", `unknown status "done"`}},
		{"duplicate equivalence id", func(c *CatalogConfig) { c.Equivalences = append(c.Equivalences, c.Equivalences[0]) },
			[]string{"equivalences", `duplicate id "macos-password-minimum-length"`}},
		{"invalid id", func(c *CatalogConfig) { c.Topics[0].ID = "Encryption" },
			[]string{"topics", `invalid id "Encryption"`}},
		{"duplicate topic id", func(c *CatalogConfig) { c.Topics[1].ID = "encryption" },
			[]string{"topics", `duplicate id "encryption"`}},
		{"topic without label", func(c *CatalogConfig) { c.Topics[0].Label = " " },
			[]string{"topics", `"encryption"`, "has no label"}},
		{"topic without rules", func(c *CatalogConfig) { c.Topics[0].Match = nil },
			[]string{"topics", "has no match rules"}},
		{"topic rule with no field", func(c *CatalogConfig) { c.Topics[0].Match = []TopicRule{{}} },
			[]string{"topics", "match rule 0", "sets no field"}},
		{"uncompilable topic regex", func(c *CatalogConfig) { c.Topics[0].Match[0].Key = "(" },
			[]string{"topics", `invalid key regex "("`}},
		{"rule naming a missing topic", func(c *CatalogConfig) { c.Rules[0].Left.Topic = "firewall" },
			[]string{"rules", `"r1-encryption"`, "left", `topic "firewall"`}},
		{"rule side with neither topic nor keys", func(c *CatalogConfig) { c.Rules[0].Right = Selector{} },
			[]string{"rules", "right", "neither a topic nor keys"}},
		{"rule side with both topic and keys", func(c *CatalogConfig) { c.Rules[0].Right.Topic = "encryption" },
			[]string{"rules", "right", "both a topic and keys"}},
		{"rule key not canonical", func(c *CatalogConfig) { c.Rules[0].Right.Keys = []string{"./Vendor/MSFT/Firewall/MdmStore/DomainProfile/EnableFirewall"} },
			[]string{"rules", `use "device_vendor_msft_firewall_mdmstore_domainprofile_enablefirewall"`}},
		{"rule unknown class", func(c *CatalogConfig) { c.Rules[0].Left.Class = "policy" },
			[]string{"rules", `unknown class "policy"`}},
		{"rule uncompilable platforms", func(c *CatalogConfig) { c.Rules[0].Right.Platforms = "[" },
			[]string{"rules", `invalid platforms regex "["`}},
		{"rule without description", func(c *CatalogConfig) { c.Rules[0].Description = "" },
			[]string{"rules", "has no description"}},
		{"rule without violation", func(c *CatalogConfig) { c.Rules[0].Violation = "" },
			[]string{"rules", "has no violation"}},
		{"rule without resolution", func(c *CatalogConfig) { c.Rules[0].Resolution = "" },
			[]string{"rules", "has no resolution"}},
		{"rule non-Learn reference", func(c *CatalogConfig) { c.Rules[0].Reference = "" },
			[]string{"rules", "must start with https://learn.microsoft.com/"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validCatalog()
			tt.mutate(&cfg)
			_, err := CompileCatalog(cfg)
			if err == nil {
				t.Fatal("CompileCatalog() = nil, want an error")
			}
			for _, want := range tt.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should contain %q", err, want)
				}
			}
		})
	}
}

func TestValidMemberKey(t *testing.T) {
	tests := []struct {
		key       string
		ok        bool
		canonical string
	}{
		{macCompliance, true, ""},
		{"#microsoft.graph.macOSCustomConfiguration#payload:com.example.profile", true, ""},
		{"#microsoft.graph.windows10GeneralConfiguration#defenderDetectedMalwareActions.highSeverity", true, ""},
		{macCatalogKey, true, ""},
		{"device_vendor_msft_bitlocker_requiredeviceencryption", true, ""},
		{"vendor_msft_firewall_mdmstore_domainprofile_enablefirewall", true, ""},
		{"deviceConfiguration--windows10EndpointProtectionConfiguration_firewallEnabled", true, ""},
		{"./Device/Vendor/MSFT/BitLocker/RequireDeviceEncryption", false, "device_vendor_msft_bitlocker_requiredeviceencryption"},
		{"./Vendor/MSFT/Firewall/MdmStore/DomainProfile/EnableFirewall", false, "device_vendor_msft_firewall_mdmstore_domainprofile_enablefirewall"},
		{"Com.Apple.MobileDevice.PasswordPolicy_MinLength", false, macCatalogKey},
		{"#microsoft.graph.macOSCompliancePolicy", false, ""},
		{"", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			ok, canonical := validMemberKey(tt.key)
			if ok != tt.ok || canonical != tt.canonical {
				t.Errorf("validMemberKey(%q) = (%v, %q), want (%v, %q)", tt.key, ok, canonical, tt.ok, tt.canonical)
			}
		})
	}
}

// TestIndexKeysAreValidMembers guards the shared key construction: every key
// the index emits, from every source type, passes member validation, so a
// member pasted from mechanical.yaml is always accepted.
func TestIndexKeysAreValidMembers(t *testing.T) {
	f := syntheticTenant(t)
	f.add(compType+"mac.yaml", docs.ResourceMeta{ODataType: "#microsoft.graph.macOSCompliancePolicy"},
		map[string]interface{}{"@odata.type": "#microsoft.graph.macOSCompliancePolicy", "passwordMinimumLength": 12,
			"nested": map[string]interface{}{"inner": true}})
	f.add(dcType+"mac_custom.yaml", docs.ResourceMeta{ODataType: odataMacOSCustom, Platforms: "macOS"},
		customProfile(odataMacOSCustom, plist(rootIDA, payloadMark)))
	f.add(dcType+"mac_app.yaml", docs.ResourceMeta{ODataType: odataMacOSCustomApp, Platforms: "macOS"},
		appConfig(bundleIDTest, "<dict><key>a</key><string>b</string></dict>"))
	f.add("Microsoft.Graph/groupPolicyConfigurations/g.yaml", docs.ResourceMeta{}, map[string]interface{}{
		"definitionValues": []interface{}{
			map[string]interface{}{"enabled": true, "definition": map[string]interface{}{"id": "AAAA-Def"}},
		}})
	f.add("Microsoft.Graph/deviceManagementIntents/i.yaml", docs.ResourceMeta{}, map[string]interface{}{
		"settings": []interface{}{
			map[string]interface{}{"definitionId": "deviceConfiguration--windows10EndpointProtectionConfiguration_firewallEnabled", "valueJson": `true`},
		}})
	f.add(scType+"apple.yaml", docs.ResourceMeta{Platforms: "macOS"}, loginItemsPolicy("macOS", "TEAM1"))
	dir := f.save()

	m, err := docs.LoadExportMetadata(dir)
	if err != nil {
		t.Fatal(err)
	}
	settings, _, _ := buildIndex(&m, filepath.Join(dir, "resources"))
	sourceTypes := map[string]bool{}
	for _, s := range settings {
		sourceTypes[s.SourceType] = true
		if ok, canonical := validMemberKey(s.Key); !ok {
			t.Errorf("index key %q (%s) fails member validation (canonical %q)", s.Key, s.SourceType, canonical)
		}
	}
	for _, typ := range []string{typeSettingsCatalog, typeDeviceConfigurations, typeDeviceCompliancePolicies, typeGroupPolicyConfigs, typeIntents} {
		if !sourceTypes[typ] {
			t.Errorf("fixture indexes nothing of %s", typ)
		}
	}
}

func TestCatalogHashStability(t *testing.T) {
	base, err := CompileCatalog(validCatalog())
	if err != nil {
		t.Fatal(err)
	}

	reordered := validCatalog()
	reordered.Topics[0], reordered.Topics[1] = reordered.Topics[1], reordered.Topics[0]
	reordered.Topics[1].Match[0], reordered.Topics[1].Match[1] = reordered.Topics[1].Match[1], reordered.Topics[1].Match[0]
	m := reordered.Equivalences[0].Members
	m[0], m[2] = m[2], m[0]
	reordered.Equivalences = append(reordered.Equivalences, EquivalenceConfig{
		ID: "a-first", Members: []string{"a_one", "a_two"}, Relation: "same", Reference: learnRef, Status: StatusVerify,
	})
	withExtra := validCatalog()
	withExtra.Equivalences = append([]EquivalenceConfig{reordered.Equivalences[1]}, withExtra.Equivalences...)

	a, err := CompileCatalog(reordered)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CompileCatalog(withExtra)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256() != b.SHA256() {
		t.Error("entry, member and match-rule order must not move the hash")
	}
	if a.SHA256() == base.SHA256() {
		t.Error("an added equivalence must move the hash")
	}

	changed := validCatalog()
	changed.Rules[0].Reference = learnRef + "#passwords"
	c, err := CompileCatalog(changed)
	if err != nil {
		t.Fatal(err)
	}
	if c.SHA256() == base.SHA256() {
		t.Error("a changed reference must move the hash: the LLM step judges against it")
	}
}

// TestCatalogHashIgnoresYAMLKeyOrder decodes the same catalog written with its
// keys in two orders.
func TestCatalogHashIgnoresYAMLKeyOrder(t *testing.T) {
	one := `
version: 1
equivalences:
  - id: e
    members: [a_one, a_two]
    relation: same
    reference: ` + learnRef + `
    status: verify
    enforced: {macos: true, ios: false}
`
	two := `
equivalences:
  - status: verify
    enforced: {ios: false, macos: true}
    reference: ` + learnRef + `
    relation: same
    members: [a_two, a_one]
    id: e
version: 1
`
	hash := func(text string) string {
		var cfg CatalogConfig
		if err := yaml.Unmarshal([]byte(text), &cfg); err != nil {
			t.Fatal(err)
		}
		c, err := CompileCatalog(cfg)
		if err != nil {
			t.Fatal(err)
		}
		return c.SHA256()
	}
	if hash(one) != hash(two) {
		t.Error("YAML key order must not move the hash")
	}
}

func TestTopics(t *testing.T) {
	c, err := CompileCatalog(validCatalog())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		facts TopicFacts
		want  []string
	}{
		{"several topics", TopicFacts{Name: "FileVault", Platforms: "macOS",
			Keys: []string{"com.apple.mcx.filevault2_enable", macCatalogKey}}, []string{"encryption", "macos-passwords"}},
		{"a key rule", TopicFacts{Keys: []string{"device_vendor_msft_bitlocker_requiredeviceencryption"}}, []string{"encryption"}},
		{"a name rule, case-insensitive", TopicFacts{Name: "Disk ENCRYPTION"}, []string{"encryption"}},
		{"AND within a rule", TopicFacts{Platforms: "windows10", Keys: []string{macCatalogKey}}, nil},
		{"no match", TopicFacts{Name: "Wi-Fi", Type: "Microsoft.Graph/deviceConfigurations"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := c.Topics(tt.facts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Topics() = %v, want %v", got, tt.want)
			}
		})
	}

	typed, err := CompileCatalog(CatalogConfig{Version: 1, Topics: []TopicConfig{{ID: "ca", Label: "CA",
		Match: []TopicRule{{Type: "Microsoft.Graph/conditionalAccessPolicies", ODataType: "conditionalaccess"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := typed.Topics(TopicFacts{Type: "Microsoft.Graph/conditionalAccessPolicies", ODataType: "#microsoft.graph.conditionalAccessPolicy"}); !reflect.DeepEqual(got, []string{"ca"}) {
		t.Errorf("type is exact and odataType a regex: got %v", got)
	}
	if got := typed.Topics(TopicFacts{Type: "microsoft.graph/conditionalaccesspolicies", ODataType: "#microsoft.graph.conditionalAccessPolicy"}); got != nil {
		t.Errorf("type must match exactly: got %v", got)
	}
}

// macPair builds a macOS compliance policy and a Settings Catalog policy on
// All devices: the compliance requires a minimum length of 12, the
// configuration sets configured.
func macPair(t *testing.T, configured int) string {
	f := newFixture(t)
	f.add(compType+"mac.yaml", docs.ResourceMeta{
		ODataType:         "#microsoft.graph.macOSCompliancePolicy",
		AssignmentTargets: targets(allDevices()),
	}, map[string]interface{}{"@odata.type": "#microsoft.graph.macOSCompliancePolicy", "passwordMinimumLength": 12})
	f.add(scType+"mac.yaml", docs.ResourceMeta{Platforms: "macOS", AssignmentTargets: targets(allDevices())},
		catalogPolicy("macOS", catalogSimple(macCatalogKey, configured)))
	return f.save()
}

func TestAnalyzeWithCatalog(t *testing.T) {
	for _, tt := range []struct {
		status     string
		confidence string
	}{
		{StatusVerified, ConfidenceFirm},
		{StatusVerify, ConfidencePossible},
	} {
		t.Run(tt.status, func(t *testing.T) {
			cfg := validCatalog()
			cfg.Equivalences[0].Status = tt.status
			c, err := CompileCatalog(cfg)
			if err != nil {
				t.Fatal(err)
			}
			res, err := Analyze(Options{TenantDir: macPair(t, 8), Catalog: c, DryRun: true})
			if err != nil {
				t.Fatal(err)
			}
			fs := res.Mechanical.Findings
			if len(fs) != 1 {
				t.Fatalf("want one finding, got %+v", fs)
			}
			if f := fs[0]; f.Kind != KindContradiction || f.Confidence != tt.confidence || f.Key != "equivalence:macos-password-minimum-length" {
				t.Errorf("got %+v", f)
			}
			meta := res.Metadata.Catalog
			if meta == nil {
				t.Fatal("metadata has no catalog block")
			}
			if meta.SHA256 != c.SHA256() || meta.Counts != c.Counts() {
				t.Errorf("catalog block = %+v", meta)
			}
			if want := []string{macRestriction}; !reflect.DeepEqual(meta.UnmatchedMembers, want) {
				t.Errorf("unmatchedMembers = %v, want %v", meta.UnmatchedMembers, want)
			}
		})
	}
}

func TestAnalyzeCatalogBlockOnDisk(t *testing.T) {
	read := func(t *testing.T, dir string) map[string]interface{} {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, DirName, MetadataFileName))
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]interface{}
		if err := yaml.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	t.Run("absent without a catalog", func(t *testing.T) {
		dir := macPair(t, 8)
		if _, err := Analyze(Options{TenantDir: dir}); err != nil {
			t.Fatal(err)
		}
		meta := read(t, dir)
		if _, ok := meta["catalog"]; ok {
			t.Errorf("catalog block written without a catalog: %v", meta["catalog"])
		}
		if meta["version"] != metadataVersion {
			t.Errorf("version = %v, want %d", meta["version"], metadataVersion)
		}
	})

	t.Run("present with a catalog", func(t *testing.T) {
		c, err := CompileCatalog(validCatalog())
		if err != nil {
			t.Fatal(err)
		}
		dir := macPair(t, 14)
		if _, err := Analyze(Options{TenantDir: dir, Catalog: c}); err != nil {
			t.Fatal(err)
		}
		meta := read(t, dir)
		block, ok := meta["catalog"].(map[string]interface{})
		if !ok {
			t.Fatalf("catalog block = %v", meta["catalog"])
		}
		if block["sha256"] != c.SHA256() {
			t.Errorf("sha256 = %v, want %s", block["sha256"], c.SHA256())
		}
		counts := fmt.Sprint(block["counts"])
		if counts != "map[equivalences:1 rules:1 topics:2]" {
			t.Errorf("counts = %s", counts)
		}
		if got := fmt.Sprint(block["unmatchedMembers"]); got != "["+macRestriction+"]" {
			t.Errorf("unmatchedMembers = %s", got)
		}
		if meta["version"] != metadataVersion {
			t.Errorf("version = %v, want %d unchanged", meta["version"], metadataVersion)
		}
	})
}
