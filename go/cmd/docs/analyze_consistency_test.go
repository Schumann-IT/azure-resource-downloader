package docs

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/consistency"
	docsengine "azure-resource-downloader/internal/docs"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const testDomain = "contoso.example.com"

// runConsistency runs `docs analyze-consistency --domain <testDomain>` against
// output, with the given consistency section (nil: none).
func runConsistency(t *testing.T, output string, section interface{}) error {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("output", output)
	if section != nil {
		viper.Set("consistency", section)
	}
	cmd := NewAnalyzeConsistencyCommand()
	cmd.SetContext(context.Background())
	if err := cmd.Flags().Set("domain", testDomain); err != nil {
		t.Fatal(err)
	}
	return runAnalyzeConsistency(cmd, nil)
}

// writeEmptyExport writes an export with no resources for testDomain.
func writeEmptyExport(t *testing.T, output string) string {
	t.Helper()
	tenantDir := filepath.Join(output, testDomain)
	m := docsengine.Metadata{
		GeneratedAt: "2026-01-01T00:00:00Z",
		Tenant:      testDomain,
		Run:         docsengine.RunMeta{Complete: true},
		Types:       map[string]docsengine.TypeMeta{},
		Resources:   map[string]docsengine.ResourceMeta{},
	}
	data, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tenantDir, "resources")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return tenantDir
}

// TestAnalyzeConsistencyRefusesAnInvalidCatalog: a typo in the catalog fails
// with exit 2 before the export is resolved, so nothing is read or written.
func TestAnalyzeConsistencyRefusesAnInvalidCatalog(t *testing.T) {
	output := t.TempDir()
	tenantDir := writeEmptyExport(t, output)

	err := runConsistency(t, output, map[string]interface{}{
		"version":      1,
		"equivalences": []interface{}{map[string]interface{}{"id": "e", "members": []interface{}{"a_one", "a_two"}, "relation": "~"}},
	})
	if err == nil {
		t.Fatal("want an error for an invalid catalog")
	}
	if got := cmdutil.ExitCode(err); got != exitCannotAnswer {
		t.Errorf("exit code = %d, want %d", got, exitCannotAnswer)
	}
	if !strings.Contains(err.Error(), "invalid 'consistency' config section") || !strings.Contains(err.Error(), "unknown relation") {
		t.Errorf("error %q should name the section and the problem", err)
	}
	if _, statErr := os.Stat(filepath.Join(tenantDir, consistency.DirName)); !os.IsNotExist(statErr) {
		t.Errorf("consistency/ exists after a refused catalog (stat: %v)", statErr)
	}
}

// TestAnalyzeConsistencyRefusesAnUnknownField: a misspelt optional field is
// refused instead of silently dropped.
func TestAnalyzeConsistencyRefusesAnUnknownField(t *testing.T) {
	output := t.TempDir()
	writeEmptyExport(t, output)

	err := runConsistency(t, output, map[string]interface{}{
		"version": 1,
		"equivalences": []interface{}{map[string]interface{}{
			"id": "e", "members": []interface{}{"a_one", "a_two"}, "relation": "same",
			"enforce": map[string]interface{}{"macos": true},
		}},
	})
	if err == nil {
		t.Fatal("want an error for an unknown field")
	}
	if got := cmdutil.ExitCode(err); got != exitCannotAnswer {
		t.Errorf("exit code = %d, want %d", got, exitCannotAnswer)
	}
	if !strings.Contains(err.Error(), "invalid 'consistency' config section") {
		t.Errorf("error %q should name the section", err)
	}
}

// TestAnalyzeConsistencyRefusesBeforeResolving: with no export at all, the
// catalog error is still the one reported — compilation comes first.
func TestAnalyzeConsistencyRefusesBeforeResolving(t *testing.T) {
	err := runConsistency(t, filepath.Join(t.TempDir(), "missing"), map[string]interface{}{"version": 0})
	if err == nil || !strings.Contains(err.Error(), "invalid 'consistency' config section") {
		t.Errorf("error = %v, want the catalog refusal before any export lookup", err)
	}
}

// TestAnalyzeConsistencyRecordsTheCatalog: a valid catalog reaches the
// analysis and lands as the catalog: block of consistency/metadata.yaml.
func TestAnalyzeConsistencyRecordsTheCatalog(t *testing.T) {
	output := t.TempDir()
	tenantDir := writeEmptyExport(t, output)

	err := runConsistency(t, output, map[string]interface{}{
		"version": 1,
		"equivalences": []interface{}{map[string]interface{}{
			"id": "e", "members": []interface{}{"a_one", "a_two"}, "relation": "same",
			"reference": "https://learn.microsoft.com/en-us/mem/intune/", "status": "verify",
		}},
	})
	if err != nil {
		t.Fatalf("runAnalyzeConsistency() = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tenantDir, consistency.DirName, consistency.MetadataFileName))
	if err != nil {
		t.Fatal(err)
	}
	var meta consistency.MetadataFile
	if err := yaml.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Catalog == nil || meta.Catalog.Counts.Equivalences != 1 || len(meta.Catalog.UnmatchedMembers) != 2 {
		t.Errorf("catalog block = %+v", meta.Catalog)
	}
}

// loadTailoredCatalog decodes and compiles the consistency section of the
// tracked worked example, config-tailored-intune.yaml.
func loadTailoredCatalog(t *testing.T) (consistency.CatalogConfig, *consistency.Compiled) {
	t.Helper()
	v := viper.New()
	v.SetConfigFile(filepath.Join("..", "..", "config-tailored-intune.yaml"))
	if err := v.ReadInConfig(); err != nil {
		t.Fatalf("reading config-tailored-intune.yaml: %v", err)
	}
	if !v.IsSet("consistency") {
		t.Fatal("config-tailored-intune.yaml has no 'consistency' section")
	}
	var cfg consistency.CatalogConfig
	if err := v.UnmarshalKey("consistency", &cfg); err != nil {
		t.Fatal(err)
	}
	c, err := consistency.CompileCatalog(cfg)
	if err != nil {
		t.Fatalf("the tracked catalog does not compile: %v", err)
	}
	return cfg, c
}

// TestTailoredConfigCatalogCompiles keeps the tracked catalog shipped in the
// worked example valid and pins its reviewed shape: the counts, the entries
// that left it, and exactly which entries are verified.
func TestTailoredConfigCatalogCompiles(t *testing.T) {
	cfg, c := loadTailoredCatalog(t)
	counts := c.Counts()
	if counts.Equivalences != 37 || counts.Topics != 13 || counts.Rules != 29 {
		t.Errorf("tracked catalog counts = %+v, want 37 equivalences / 13 topics / 29 rules", counts)
	}
	for _, e := range cfg.Equivalences {
		if len(e.Members) < 2 || e.Relation == "" {
			t.Errorf("equivalence %q decoded incompletely: %+v", e.ID, e)
		}
		if strings.Contains(e.ID, "minimum-os") {
			t.Errorf("equivalence %q: the minimum-OS comparison is blocked on indexing", e.ID)
		}
	}
	for _, topic := range cfg.Topics {
		if topic.ID == "assignment-filters" {
			t.Errorf("topic %q belongs to the scope model, not the catalog", topic.ID)
		}
	}
	for _, r := range cfg.Rules {
		if strings.HasPrefix(r.ID, "r6-") || strings.HasPrefix(r.ID, "r7-") {
			t.Errorf("rule %q belongs to the scope model, not the catalog", r.ID)
		}
	}

	var gotEq, gotRules []string
	for _, e := range cfg.Equivalences {
		if e.Status == "verified" {
			gotEq = append(gotEq, e.ID)
		}
	}
	for _, r := range cfg.Rules {
		if r.Status == "verified" {
			gotRules = append(gotRules, r.ID)
		}
	}
	wantEq := []string{"windows-defender-cloud-protection-maps", "windows-defender-realtime-admx"}
	wantRules := []string{
		"r-whfb-user-over-device-enable", "r-whfb-user-over-device-minpinlength",
		"r5-deferral-with-feature-profile", "r5b-ring-feature-pause-with-feature-profile",
		"r5c-ring-driver-exclusion-with-driver-policy", "r5d-overlapping-feature-update-policies",
		"r5e-apple-enforcement-ignores-update-settings",
	}
	sort.Strings(gotEq)
	sort.Strings(gotRules)
	if strings.Join(gotEq, "|") != strings.Join(wantEq, "|") {
		t.Errorf("verified equivalences = %v, want %v", gotEq, wantEq)
	}
	if strings.Join(gotRules, "|") != strings.Join(wantRules, "|") {
		t.Errorf("verified rules = %v, want %v", gotRules, wantRules)
	}
}

// TestTailoredConfigCatalogUsesTheBridgedFirewallForm guards the Firewall CSP
// bridge: custom OMA-URIs index as vendor_msft_firewall_..., so no member,
// topic key regex or rule selector of the tracked catalog may name the refused
// device_vendor_msft_firewall_... form.
func TestTailoredConfigCatalogUsesTheBridgedFirewallForm(t *testing.T) {
	cfg, _ := loadTailoredCatalog(t)
	const refused = "device_vendor_msft_firewall_"
	check := func(where, key string) {
		if strings.HasPrefix(strings.TrimPrefix(key, "^"), refused) {
			t.Errorf("%s uses the refused %s form: %q", where, refused, key)
		}
	}
	// A topic key is a regex, so the refused form may hide behind a flag group
	// or inside an alternation: match it anywhere, case-insensitively.
	checkRegex := func(where, key string) {
		if strings.Contains(strings.ToLower(key), refused) {
			t.Errorf("%s uses the refused %s form: %q", where, refused, key)
		}
	}
	for _, e := range cfg.Equivalences {
		for _, m := range e.Members {
			check("equivalence "+e.ID, m)
		}
	}
	for _, topic := range cfg.Topics {
		for _, m := range topic.Match {
			checkRegex("topic "+topic.ID, m.Key)
		}
	}
	for _, r := range cfg.Rules {
		for _, k := range r.Left.Keys {
			check("rule "+r.ID+" left", k)
		}
		for _, k := range r.Right.Keys {
			check("rule "+r.ID+" right", k)
		}
	}
}

// TestTailoredConfigSeedsThePasswordLengthEquivalence pins what the
// contradiction analysis relies on in the tracked catalog: the macOS password
// length members and relation.
func TestTailoredConfigSeedsThePasswordLengthEquivalence(t *testing.T) {
	_, c := loadTailoredCatalog(t)
	want := []string{
		"#microsoft.graph.macOSCompliancePolicy#passwordMinimumLength",
		"#microsoft.graph.macOSGeneralDeviceConfiguration#passwordMinimumLength",
		"com.apple.mobiledevice.passwordpolicy_minlength",
	}
	for _, e := range c.Equivalences() {
		if e.ID != "macos-password-minimum-length" {
			continue
		}
		got := append([]string(nil), e.Members...)
		sort.Strings(got)
		sort.Strings(want)
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("members = %v, want %v", got, want)
		}
		if string(e.Relation) != ">=" {
			t.Errorf("relation = %q, want >=", e.Relation)
		}
		return
	}
	t.Fatal("equivalence macos-password-minimum-length not found in the tracked catalog")
}
