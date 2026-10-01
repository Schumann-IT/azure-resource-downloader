package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/runprep"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// loadConfig runs the real config loading for a command invocation, with the
// package-level flag variables set as Cobra would set them.
func loadConfig(t *testing.T, cmd *cobra.Command, configFile, configDir string) error {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)

	prevFile, prevDir := flagConfigFile, flagConfigDir
	t.Cleanup(func() { flagConfigFile, flagConfigDir = prevFile, prevDir })
	flagConfigFile, flagConfigDir = configFile, configDir

	if cmd == nil {
		cmd = &cobra.Command{Use: "probe"}
	}
	return initConfig(cmd)
}

// domainCmd builds a throwaway command carrying --domain, optionally marked as
// explicitly passed, which is the only thing that may select a profile.
func domainCmd(t *testing.T, domain string, passed bool) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "probe"}
	cmd.Flags().String("domain", "", "")
	if passed {
		if err := cmd.Flags().Set("domain", domain); err != nil {
			t.Fatalf("setting --domain: %v", err)
		}
	}
	return cmd
}

// TestEnvironmentIsNotRead guards the removal of the environment layer. An
// AZURE_RD_* variable left over from another tenant's run must have no effect
// whatsoever: it used to outrank the config file, so a forgotten export could
// silently redirect a download or point an audit at the wrong workspace.
func TestEnvironmentIsNotRead(t *testing.T) {
	t.Setenv("AZURE_RD_OUTPUT", "/tmp/from-env")
	t.Setenv("AZURE_RD_SUBSCRIPTION", "sub-from-env")
	t.Setenv("AZURE_RD_LOG_LEVEL", "warn")

	if err := loadConfig(t, nil, "", ""); err != nil {
		t.Fatalf("initConfig() = %v, want nil", err)
	}

	if got := viper.GetString("output"); got == "/tmp/from-env" {
		t.Error("AZURE_RD_OUTPUT still applies; the environment layer must be gone")
	}
	if got := viper.GetString("subscription"); got != "" {
		t.Errorf("subscription = %q, want empty; AZURE_RD_SUBSCRIPTION must be ignored", got)
	}
}

// TestBaseFileConventionInConfigDir guards the convention that keeps the
// everyday invocation at two flags: a config directory's base.yaml is the base
// configuration, and the profile is merged over it.
func TestBaseFileConventionInConfigDir(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, config.BaseFileName), "timeout: 42\n")
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "subscription: sub-from-profile\n")

	if err := loadConfig(t, domainCmd(t, "contoso.example.com", true), "", dir); err != nil {
		t.Fatalf("initConfig() = %v, want nil", err)
	}

	if got := viper.GetInt("timeout"); got != 42 {
		t.Errorf("timeout = %d, want 42 from %s", got, config.BaseFileName)
	}
	if got := viper.GetString("subscription"); got != "sub-from-profile" {
		t.Errorf("subscription = %q, want the profile's value", got)
	}
	if configLoaded.ProfileFile == "" || configLoaded.Domain != "contoso.example.com" {
		t.Errorf("loaded = %+v, want the profile and its domain recorded", configLoaded)
	}
}

// TestExplicitConfigOverridesBaseConvention: --config names the base file
// explicitly and must win over the directory's base.yaml, so a one-off base is
// always possible without moving files around.
func TestExplicitConfigOverridesBaseConvention(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, config.BaseFileName), "timeout: 42\n")
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")

	explicit := filepath.Join(t.TempDir(), "other.yaml")
	write(t, explicit, "timeout: 99\n")

	if err := loadConfig(t, domainCmd(t, "contoso.example.com", true), explicit, dir); err != nil {
		t.Fatalf("initConfig() = %v, want nil", err)
	}
	if got := viper.GetInt("timeout"); got != 99 {
		t.Errorf("timeout = %d, want 99 from the explicit --config", got)
	}
}

// TestConfigDirWithoutDomainFails: the profile cannot be chosen by the resolved
// tenant (configuration is read before authentication, and supplies the
// credentials it needs), so the rule is stated rather than guessed.
func TestConfigDirWithoutDomainFails(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")

	err := loadConfig(t, domainCmd(t, "contoso.example.com", false), "", dir)
	if err == nil {
		t.Fatal("initConfig() = nil, want an error when --config-dir has no --domain")
	}
	if !strings.Contains(err.Error(), "--domain") {
		t.Errorf("error %q should name --domain", err)
	}
}

// TestMissingProfileIsFatalAndListsCandidates: falling back to the defaults
// would run with the wrong (empty) configuration while looking like it worked,
// so a missing profile fails — and says which tenants are available, which is
// also the answer to "which tenants do I have?".
func TestMissingProfileIsFatalAndListsCandidates(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, config.BaseFileName), "")
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")
	write(t, filepath.Join(dir, "fabrikam.example.com.yaml"), "")

	err := loadConfig(t, domainCmd(t, "typo.example.com", true), "", dir)
	if err == nil {
		t.Fatal("initConfig() = nil, want an error for a domain with no profile")
	}
	for _, want := range []string{"contoso.example.com", "fabrikam.example.com"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should list the available profile %q", err, want)
		}
	}
	if strings.Contains(err.Error(), config.BaseFileName) {
		t.Errorf("error %q must not offer %s as a tenant", err, config.BaseFileName)
	}
}

// TestPartitionIsEnforced guards the whole point of the base/profile split: a
// key on the wrong side is a fatal error naming it, in both directions, and a
// setting that became a flag is reported as such rather than silently ignored.
func TestPartitionIsEnforced(t *testing.T) {
	tests := []struct {
		name      string
		base      string
		profile   string
		wantInErr string
	}{
		{
			name:      "tenant-scoped key in the base file",
			base:      "subscription: sub-123\n",
			wantInErr: "subscription",
		},
		{
			name:      "general key in a profile",
			profile:   "timeout: 42\n",
			wantInErr: "timeout",
		},
		{
			name:      "transformers in a profile would break comparability",
			profile:   "transformers: []\n",
			wantInErr: "transformers",
		},
		{
			name:      "a setting that is now a flag",
			base:      "dry-run: true\n",
			wantInErr: "--dry-run",
		},
		{
			name:      "audit-workspace-id in the base file",
			base:      "audit-workspace-id: 0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b\n",
			wantInErr: "audit-workspace-id",
		},
		{
			name:      "audit-workspace-id that is not a GUID",
			profile:   "audit-workspace-id: my-workspace\n",
			wantInErr: "audit-workspace-id",
		},
		{
			name:      "exclude-type in the base file",
			base:      "exclude-type: [Microsoft.Compute/virtualMachines]\n",
			wantInErr: "exclude-type",
		},
		{
			name:      "exclude-type that is not a list",
			profile:   "exclude-type: Microsoft.Compute/virtualMachines\n",
			wantInErr: "exclude-type",
		},
		{
			name:      "an unknown key is a typo, not a no-op",
			base:      "timeoutt: 42\n",
			wantInErr: "not a known setting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, config.BaseFileName), tt.base)
			write(t, filepath.Join(dir, "contoso.example.com.yaml"), tt.profile)

			err := loadConfig(t, domainCmd(t, "contoso.example.com", true), "", dir)
			if err == nil {
				t.Fatalf("initConfig() = nil, want an error mentioning %q", tt.wantInErr)
			}
			if !strings.Contains(err.Error(), tt.wantInErr) {
				t.Errorf("error %q should mention %q", err, tt.wantInErr)
			}
		})
	}
}

// TestDomainMustBeASinglePathSegment: the domain is joined to the config
// directory and to the output directory, so a value that is a path must never
// be accepted.
func TestDomainMustBeASinglePathSegment(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")

	for _, domain := range []string{"../escape", "sub/dir", ".."} {
		t.Run(domain, func(t *testing.T) {
			if err := loadConfig(t, domainCmd(t, domain, true), "", dir); err == nil {
				t.Errorf("initConfig() = nil for --domain %q, want a refusal", domain)
			}
		})
	}
}

// TestConfigExampleIsNoOp guards the promise made in config.example.yaml's own
// header: loading the file unmodified must behave exactly like running with no
// config file at all. Every effective value it produces must equal the built-in
// default, INCLUDING the transform-config hash recorded in
// resources/metadata.yaml — otherwise a run that loads the example would
// silently rewrite every resource's recorded hash.
func TestConfigExampleIsNoOp(t *testing.T) {
	if err := loadConfig(t, nil, filepath.Join("..", "config.example.yaml"), ""); err != nil {
		t.Fatalf("initConfig() = %v, want nil", err)
	}

	// Scalar and slice keys must equal the built-in defaults. --output's default
	// is read from the flag so this test cannot drift from the definition.
	if got, want := viper.GetString("output"), rootCmd.PersistentFlags().Lookup("output").DefValue; got != want {
		t.Errorf("config.example.yaml output = %q, want built-in default %q", got, want)
	}
	if got := viper.GetInt("timeout"); got != config.DefaultTimeoutSeconds {
		t.Errorf("config.example.yaml timeout = %d, want built-in default %d", got, config.DefaultTimeoutSeconds)
	}
	for _, k := range []string{"resolve-secrets", "no-prompt", "prune"} {
		if viper.GetBool(k) {
			t.Errorf("config.example.yaml %q = true, want the built-in default false", k)
		}
	}
	if got := viper.GetStringSlice("type"); len(got) != 0 {
		t.Errorf("config.example.yaml type = %v, want empty (built-in default)", got)
	}

	// The general worker count must stay COMMENTED OUT: its presence is the
	// signal that the operator chose one count for every API, which would
	// silently override the per-API defaults.
	if viper.IsSet("workers") {
		t.Error("config.example.yaml sets 'workers'; it must stay commented out, because presence overrides the per-API defaults")
	}
	if got, want := runprep.BuildWorkerConfig(), models.DefaultWorkerConfig(); !reflect.DeepEqual(got, want) {
		t.Errorf("BuildWorkerConfig() from config.example.yaml = %+v, want default %+v", got, want)
	}

	gotTransformers := runprep.BuildTransformerConfigs()
	wantTransformers := models.DefaultTransformerConfigs()
	if !reflect.DeepEqual(gotTransformers, wantTransformers) {
		t.Errorf("BuildTransformerConfigs() from config.example.yaml = %+v, want default %+v", gotTransformers, wantTransformers)
	}

	// Filters are tenant-scoped now, so the base example must define none — and
	// the recorded filter hash must be identical, or a drift check against an
	// example-loaded export would refuse to compare.
	if got := runprep.BuildResourceFilters(); len(got) != 0 {
		t.Errorf("BuildResourceFilters() from config.example.yaml = %+v, want none", got)
	}
	if got, want := docs.HashResourceFilters(runprep.BuildResourceFilters()), docs.HashResourceFilters(nil); got != want {
		t.Errorf("filtersSha256 from config.example.yaml = %s, want default %s", got, want)
	}

	// The taxonomy section changes docs/index.yaml when active, so it must stay
	// commented out to preserve the no-op guarantee.
	if viper.IsSet("taxonomy") {
		t.Errorf("config.example.yaml sets 'taxonomy' = %v, want unset", viper.Get("taxonomy"))
	}

	// The transform-config hash written to resources/metadata.yaml must be
	// byte-for-byte identical, or a run loading the example would report every
	// resource as changed even though the output bytes are unchanged.
	gotHash := docs.HashTransformConfig(gotTransformers, viper.GetBool("resolve-secrets"))
	wantHash := docs.HashTransformConfig(wantTransformers, false)
	if gotHash != wantHash {
		t.Errorf("transformConfigSha256 from config.example.yaml = %s, want default %s", gotHash, wantHash)
	}
}

// TestConfigExampleDomainIsNoOp holds the profile stub to the same promise:
// copying it to <config-dir>/<domain>.yaml unmodified must behave exactly like
// having no profile at all.
func TestConfigExampleDomainIsNoOp(t *testing.T) {
	stub, err := os.ReadFile(filepath.Join("..", "config.example.domain.yaml"))
	if err != nil {
		t.Fatalf("reading the profile stub: %v", err)
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), string(stub))

	if err := loadConfig(t, domainCmd(t, "contoso.example.com", true), "", dir); err != nil {
		t.Fatalf("initConfig() = %v, want nil (the stub must be a valid profile)", err)
	}

	for _, k := range []string{"subscription", "client-id", "tenant-id"} {
		if got := viper.GetString(k); got != "" {
			t.Errorf("config.example.domain.yaml %q = %q, want empty", k, got)
		}
	}
	// Attribution is enabled by the key's presence, so the stub must leave it
	// unset: copying the stub must not switch on audit queries.
	if viper.IsSet(config.AuditWorkspaceKey) {
		t.Errorf("config.example.domain.yaml sets %q = %v, want unset (it must stay commented out)",
			config.AuditWorkspaceKey, viper.Get(config.AuditWorkspaceKey))
	}
	if viper.IsSet("filters") {
		t.Errorf("config.example.domain.yaml sets 'filters' = %v, want unset (it must stay commented out)", viper.Get("filters"))
	}
	// An exclusion is recorded in the export metadata and gates drift, so the
	// stub must exclude nothing.
	if viper.IsSet(config.ExcludeTypeKey) {
		t.Errorf("config.example.domain.yaml sets %q = %v, want unset (it must stay commented out)",
			config.ExcludeTypeKey, viper.Get(config.ExcludeTypeKey))
	}
	if got := runprep.BuildResourceFilters(); len(got) != 0 {
		t.Errorf("BuildResourceFilters() from the profile stub = %+v, want none", got)
	}
}

// TestConfigExamplesCoverThePartition keeps the two example files honest against
// the one table that defines the partition: every recognised setting must be
// mentioned in the file it belongs to, so a newly added option cannot ship
// undocumented.
func TestConfigExamplesCoverThePartition(t *testing.T) {
	for _, tc := range []struct {
		file  string
		scope config.Scope
	}{
		{file: "config.example.yaml", scope: config.ScopeGeneral},
		{file: "config.example.domain.yaml", scope: config.ScopeTenant},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", tc.file))
			if err != nil {
				t.Fatalf("reading %s: %v", tc.file, err)
			}
			for _, key := range config.KeysInScope(tc.scope) {
				if !strings.Contains(string(data), key) {
					t.Errorf("%s does not mention the %s setting %q", tc.file, scopeName(tc.scope), key)
				}
			}
		})
	}
}

// scopeName renders a scope for test messages.
func scopeName(scope config.Scope) string {
	if scope == config.ScopeTenant {
		return "tenant-scoped"
	}
	return "general"
}

// write creates a file with the given contents, failing the test on error.
func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// mkdirAll creates a directory tree, failing the test on error.
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("creating %s: %v", path, err)
	}
}
