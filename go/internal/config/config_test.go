package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// write creates a config file with the given contents.
func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// TestScopeOfPartitionsEveryKnownKey guards the table itself: a key that is in
// neither scope is rejected as unknown, so the partition cannot quietly acquire
// a hole. The three keys named here are the ones whose placement carries real
// risk, and they are asserted individually rather than counted.
func TestScopeOfPartitionsEveryKnownKey(t *testing.T) {
	cases := map[string]Scope{
		// Hashed into transformConfigSha256: a per-tenant value would make an
		// export non-comparable with its own baseline and with every other
		// tenant.
		"transformers": ScopeGeneral,
		// The export root; the tenant is already a subdirectory of it.
		"output": ScopeGeneral,
		// Hashed into filtersSha256, which gates drift comparability, and the
		// patterns encode one tenant's naming convention.
		"filters": ScopeTenant,
		// Identity: meaningless or harmful in another tenant.
		"client-id":    ScopeTenant,
		"tenant-id":    ScopeTenant,
		"subscription": ScopeTenant,
		// A workspace from another tenant returns no rows, which reads as
		// "nobody changed it".
		AuditWorkspaceKey: ScopeTenant,
		// Recorded in the export metadata where it gates drift comparability,
		// and which types an account can read is a property of one tenant.
		ExcludeTypeKey: ScopeTenant,
		// Moved to the command line.
		"dry-run":   ScopeFlagOnly,
		"log-level": ScopeFlagOnly,
	}

	for key, want := range cases {
		got, known := ScopeOf(key)
		if !known {
			t.Errorf("ScopeOf(%q) reports unknown; every recognised key must be in the table", key)
			continue
		}
		if got != want {
			t.Errorf("ScopeOf(%q) = %v, want %v", key, got, want)
		}
	}

	if _, known := ScopeOf("not-a-setting"); known {
		t.Error("ScopeOf() accepted an unknown key; a typo must not be silently ignored")
	}
}

// TestKeysInScopeIsSortedAndDisjoint: the example files, the documentation and
// the tests enumerate the partition from this one function, so its output must
// be deterministic and each key must appear in exactly one scope.
func TestKeysInScopeIsSortedAndDisjoint(t *testing.T) {
	seen := map[string]Scope{}
	for _, scope := range []Scope{ScopeGeneral, ScopeTenant, ScopeFlagOnly} {
		keys := KeysInScope(scope)
		sorted := append([]string(nil), keys...)
		if !reflect.DeepEqual(keys, sortedCopy(sorted)) {
			t.Errorf("KeysInScope(%v) = %v, want sorted", scope, keys)
		}
		for _, key := range keys {
			if other, dup := seen[key]; dup {
				t.Errorf("key %q appears in scope %v and %v", key, other, scope)
			}
			seen[key] = scope
		}
	}
	if len(seen) == 0 {
		t.Fatal("the partition table is empty")
	}
}

// sortedCopy returns s sorted, for comparing against KeysInScope's output.
func sortedCopy(s []string) []string {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s
}

// TestValidateKeysNamesTheOffender: an invalid setting must be reported with the
// key and the file, because the alternative — ignoring it — is indistinguishable
// from a setting that had no effect.
func TestValidateKeysNamesTheOffender(t *testing.T) {
	tests := []struct {
		name      string
		keys      []string
		want      Scope
		wantInErr []string
	}{
		{
			name:      "tenant key in a base file",
			keys:      []string{"output", "subscription"},
			want:      ScopeGeneral,
			wantInErr: []string{"subscription", "tenant profile"},
		},
		{
			name:      "general key in a profile",
			keys:      []string{"client-id", "transformers"},
			want:      ScopeTenant,
			wantInErr: []string{"transformers", "base configuration file"},
		},
		{
			name:      "flag-only key names the flag",
			keys:      []string{"prune", "dry-run"},
			want:      ScopeGeneral,
			wantInErr: []string{"--dry-run"},
		},
		{
			name:      "unknown key",
			keys:      []string{"outpt"},
			want:      ScopeGeneral,
			wantInErr: []string{"outpt", "not a known setting"},
		},
		{
			name:      "case is irrelevant",
			keys:      []string{"Subscription"},
			want:      ScopeGeneral,
			wantInErr: []string{"Subscription"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateKeys("cfg.yaml", tt.keys, tt.want)
			if err == nil {
				t.Fatalf("ValidateKeys() = nil, want an error mentioning %v", tt.wantInErr)
			}
			if !strings.Contains(err.Error(), "cfg.yaml") {
				t.Errorf("error %q should name the file", err)
			}
			for _, want := range tt.wantInErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q should mention %q", err, want)
				}
			}
		})
	}

	if err := ValidateKeys("cfg.yaml", []string{"output", "timeout", "prune"}, ScopeGeneral); err != nil {
		t.Errorf("ValidateKeys() on valid general keys = %v, want nil", err)
	}
}

// TestLoadMergesProfileOverBase pins the layering: the base supplies the general
// settings, the profile the tenant's own, and the profile is read last.
func TestLoadMergesProfileOverBase(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, BaseFileName), "timeout: 42\nprune: true\n")
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "subscription: sub-123\n")

	v := viper.New()
	res, err := Load(v, Options{ConfigDir: dir, Domain: "contoso.example.com"})
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}

	if got := v.GetInt("timeout"); got != 42 {
		t.Errorf("timeout = %d, want 42 from the base file", got)
	}
	if !v.GetBool("prune") {
		t.Error("prune = false, want true from the base file")
	}
	if got := v.GetString("subscription"); got != "sub-123" {
		t.Errorf("subscription = %q, want the profile's value", got)
	}
	if res.Domain != "contoso.example.com" || res.ProfileFile == "" || res.BaseFile == "" {
		t.Errorf("Result = %+v, want both files and the domain recorded", res)
	}
}

// TestLoadAcceptsAWorkspaceGUID: the one accepted shape of audit-workspace-id
// is a workspace id, read back unchanged.
func TestLoadAcceptsAWorkspaceGUID(t *testing.T) {
	const id = "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b"
	dir := t.TempDir()
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), AuditWorkspaceKey+": "+id+"\n")

	v := viper.New()
	if _, err := Load(v, Options{ConfigDir: dir, Domain: "contoso.example.com"}); err != nil {
		t.Fatalf("Load() = %v, want a GUID accepted", err)
	}
	if got := v.GetString(AuditWorkspaceKey); got != id {
		t.Errorf("%s = %q, want %q", AuditWorkspaceKey, got, id)
	}
}

// TestLoadAcceptsAnExcludeTypeList: the accepted shape is a list of type
// names, read back unchanged — normalising the names against the registry is
// runprep's job, not the loader's.
func TestLoadAcceptsAnExcludeTypeList(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "contoso.example.com.yaml"),
		ExcludeTypeKey+":\n  - Microsoft.Compute/virtualMachines\n  - microsoft.storage/storageaccounts\n")

	v := viper.New()
	if _, err := Load(v, Options{ConfigDir: dir, Domain: "contoso.example.com"}); err != nil {
		t.Fatalf("Load() = %v, want a list of type names accepted", err)
	}
	want := []string{"Microsoft.Compute/virtualMachines", "microsoft.storage/storageaccounts"}
	if got := v.GetStringSlice(ExcludeTypeKey); !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %v, want %v", ExcludeTypeKey, got, want)
	}

	empty := t.TempDir()
	write(t, filepath.Join(empty, "contoso.example.com.yaml"), ExcludeTypeKey+": []\n")
	if _, err := Load(viper.New(), Options{ConfigDir: empty, Domain: "contoso.example.com"}); err != nil {
		t.Errorf("Load() = %v, want an empty list accepted", err)
	}
}

// TestLoadWithoutConfigDirAppliesDefaults: the zero-config path must keep
// working, and the settings that lost their flag must still get their built-in
// default from somewhere.
func TestLoadWithoutConfigDirAppliesDefaults(t *testing.T) {
	v := viper.New()
	res, err := Load(v, Options{})
	if err != nil {
		t.Fatalf("Load() = %v, want nil for a run with no configuration at all", err)
	}
	if res.BaseFile != "" || res.ProfileFile != "" {
		t.Errorf("Result = %+v, want no files recorded", res)
	}
	if got := v.GetInt("timeout"); got != DefaultTimeoutSeconds {
		t.Errorf("timeout = %d, want the built-in default %d", got, DefaultTimeoutSeconds)
	}
}

// TestLoadLeavesWorkersUnset is the trap this design has to avoid: the per-API
// worker counts apply unless the operator chose a single count deliberately, and
// that choice is detected with IsSet. Registering a default would make IsSet
// always true and silently flatten Microsoft Graph's 5 and ARM's 20 into one
// number.
func TestLoadLeavesWorkersUnset(t *testing.T) {
	v := viper.New()
	if _, err := Load(v, Options{}); err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if v.IsSet("workers") {
		t.Error("workers is set after Load(); it must have no default, or the per-API counts are lost")
	}
}

// TestLoadRefusals covers every way the configuration can be wrong, because each
// alternative is a silent misconfiguration: running with the wrong (empty)
// settings, or reaching outside the config directory.
func TestLoadRefusals(t *testing.T) {
	t.Run("config dir without a domain", func(t *testing.T) {
		dir := t.TempDir()
		_, err := Load(viper.New(), Options{ConfigDir: dir})
		if !errors.Is(err, ErrDomainRequired) {
			t.Fatalf("Load() error = %v, want ErrDomainRequired", err)
		}
	})

	t.Run("missing profile lists the available domains", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, BaseFileName), "")
		write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")

		_, err := Load(viper.New(), Options{ConfigDir: dir, Domain: "typo.example.com"})
		if !errors.Is(err, ErrProfileMissing) {
			t.Fatalf("Load() error = %v, want ErrProfileMissing", err)
		}
		if !strings.Contains(err.Error(), "contoso.example.com") {
			t.Errorf("error %q should list the available profiles", err)
		}
	})

	t.Run("mistyped explicit config file", func(t *testing.T) {
		_, err := Load(viper.New(), Options{ConfigFile: filepath.Join(t.TempDir(), "nope.yaml")})
		if err == nil {
			t.Fatal("Load() = nil, want an error for a config file that cannot be read")
		}
	})

	t.Run("audit workspace that is not a GUID", func(t *testing.T) {
		for _, value := range []string{
			"my-workspace",
			"/subscriptions/0000/resourceGroups/rg/providers/Microsoft.OperationalInsights/workspaces/ws",
			"1234",
		} {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "contoso.example.com.yaml"), AuditWorkspaceKey+": \""+value+"\"\n")
			_, err := Load(viper.New(), Options{ConfigDir: dir, Domain: "contoso.example.com"})
			if err == nil {
				t.Errorf("Load() accepted %s %q", AuditWorkspaceKey, value)
				continue
			}
			if !strings.Contains(err.Error(), AuditWorkspaceKey) || !strings.Contains(err.Error(), "contoso.example.com.yaml") {
				t.Errorf("error %q should name the key and the profile file", err)
			}
		}
	})

	t.Run("exclude-type that is not a list of non-empty strings", func(t *testing.T) {
		for _, value := range []string{
			`"Microsoft.Compute/virtualMachines"`,
			`{Microsoft.Compute/virtualMachines: true}`,
			`[""]`,
			`["  "]`,
			`[Microsoft.Compute/virtualMachines, 42]`,
			`[[nested]]`,
		} {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "contoso.example.com.yaml"), ExcludeTypeKey+": "+value+"\n")
			_, err := Load(viper.New(), Options{ConfigDir: dir, Domain: "contoso.example.com"})
			if err == nil {
				t.Errorf("Load() accepted %s %s", ExcludeTypeKey, value)
				continue
			}
			if !strings.Contains(err.Error(), ExcludeTypeKey) || !strings.Contains(err.Error(), "contoso.example.com.yaml") {
				t.Errorf("error %q should name the key and the profile file", err)
			}
		}
	})

	t.Run("exclude-type in the base file", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, BaseFileName), ExcludeTypeKey+": [Microsoft.Compute/virtualMachines]\n")
		write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")
		_, err := Load(viper.New(), Options{ConfigDir: dir, Domain: "contoso.example.com"})
		if err == nil || !strings.Contains(err.Error(), ExcludeTypeKey) {
			t.Fatalf("Load() error = %v, want the base file refused naming %q", err, ExcludeTypeKey)
		}
	})

	t.Run("domain escaping the config directory", func(t *testing.T) {
		dir := t.TempDir()
		for _, domain := range []string{"../escape", "sub/dir", "..", BaseFileName, "base"} {
			if _, err := Load(viper.New(), Options{ConfigDir: dir, Domain: domain}); err == nil {
				t.Errorf("Load() accepted domain %q", domain)
			}
		}
	})
}

// TestExplicitConfigWinsOverBaseConvention: --config names the base explicitly,
// so a one-off base file never requires moving files in the config directory.
func TestExplicitConfigWinsOverBaseConvention(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, BaseFileName), "timeout: 42\n")
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")

	explicit := filepath.Join(t.TempDir(), "base.yaml")
	write(t, explicit, "timeout: 99\n")

	v := viper.New()
	res, err := Load(v, Options{ConfigFile: explicit, ConfigDir: dir, Domain: "contoso.example.com"})
	if err != nil {
		t.Fatalf("Load() = %v", err)
	}
	if res.BaseFile != explicit {
		t.Errorf("BaseFile = %q, want the explicit %q", res.BaseFile, explicit)
	}
	if got := v.GetInt("timeout"); got != 99 {
		t.Errorf("timeout = %d, want 99 from the explicit base file", got)
	}
}

// TestProfileCandidatesExcludesTheBaseFile: base.yaml is configuration, never a
// tenant, and the exclusion is explicit rather than incidental.
func TestProfileCandidatesExcludesTheBaseFile(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, BaseFileName), "")
	write(t, filepath.Join(dir, "fabrikam.example.com.yaml"), "")
	write(t, filepath.Join(dir, "contoso.example.com.yaml"), "")
	write(t, filepath.Join(dir, "notes.txt"), "")

	got := ProfileCandidates(dir)
	want := []string{"contoso.example.com", "fabrikam.example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ProfileCandidates() = %v, want %v", got, want)
	}

	if got := ProfileCandidates(filepath.Join(dir, "does-not-exist")); got != nil {
		t.Errorf("ProfileCandidates() on a missing directory = %v, want nil", got)
	}
}

// ExampleLoad shows the everyday layering: a config directory holding the shared
// base file and one profile per tenant, selected by domain.
func ExampleLoad() {
	v := viper.New()
	res, err := Load(v, Options{ConfigDir: "/etc/azure-rd", Domain: "contoso.onmicrosoft.com"})
	if err != nil {
		return
	}
	println(res.BaseFile, res.ProfileFile)
}
