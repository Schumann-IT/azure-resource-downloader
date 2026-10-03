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

// TestTailoredConfigCatalogCompiles keeps the seed catalog shipped in the
// worked example valid.
func TestTailoredConfigCatalogCompiles(t *testing.T) {
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
		t.Fatalf("the seed catalog does not compile: %v", err)
	}
	counts := c.Counts()
	if counts.Equivalences == 0 || counts.Topics == 0 || counts.Rules == 0 {
		t.Errorf("seed counts = %+v, want every section populated", counts)
	}
	for _, e := range cfg.Equivalences {
		if len(e.Members) < 2 || e.Relation == "" {
			t.Errorf("equivalence %q decoded incompletely: %+v", e.ID, e)
		}
	}
}

// TestTailoredConfigSeedsThePasswordLengthEquivalence pins the seed the
// contradiction analysis relies on: the macOS password length members and relation.
func TestTailoredConfigSeedsThePasswordLengthEquivalence(t *testing.T) {
	v := viper.New()
	v.SetConfigFile(filepath.Join("..", "..", "config-tailored-intune.yaml"))
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	var cfg consistency.CatalogConfig
	if err := v.UnmarshalKey("consistency", &cfg); err != nil {
		t.Fatal(err)
	}
	c, err := consistency.CompileCatalog(cfg)
	if err != nil {
		t.Fatal(err)
	}
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
	t.Fatal("equivalence macos-password-minimum-length not found in the seed")
}
