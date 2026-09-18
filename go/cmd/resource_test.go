package cmd

import (
	"strings"
	"testing"

	"azure-resource-downloader/internal/cmdutil"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// subcommand returns the named subcommand of the resource group, failing the
// test when it is absent.
func subcommand(t *testing.T, parent *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("no %q subcommand on %q", name, parent.Name())
	return nil
}

// TestResourceGroupSharesAuthFlags guards the point of the grouping: the
// authentication and selection flags and --workers are declared once on the
// parent and visible to every subcommand (all three honour them), while flags
// only download honours stay off the siblings. A command must never advertise a
// flag it ignores.
func TestResourceGroupSharesAuthFlags(t *testing.T) {
	resourceCmd := newResourceCommand()

	for _, name := range []string{"download", "drift", "types", "list"} {
		sub := subcommand(t, resourceCmd, name)
		for _, flag := range []string{"subscription", "client-id", "tenant-id", "type", "resource-id", "resource-group", "workers"} {
			// InheritedFlags is what a subcommand gets from its parents, and
			// asking for it is also what merges those flags into Flags() before
			// execution.
			if sub.InheritedFlags().Lookup(flag) == nil {
				t.Errorf("resource %s does not inherit --%s from the group", name, flag)
			}
			if sub.LocalFlags().Lookup(flag) != nil {
				t.Errorf("resource %s declares its own --%s; it must inherit the group's", name, flag)
			}
		}
	}

	// --timeout wraps each resource fetch (download and drift only) and the
	// remaining switches are download-only concerns, so types and list must not
	// offer them.
	for _, name := range []string{"types", "list"} {
		sub := subcommand(t, resourceCmd, name)
		for _, flag := range []string{"timeout", "prune", "no-prompt", "resolve-secrets"} {
			if sub.Flags().Lookup(flag) != nil {
				t.Errorf("resource %s offers --%s but ignores it", name, flag)
			}
		}
	}

	download := subcommand(t, resourceCmd, "download")
	for _, flag := range []string{"type", "resource-id", "resource-group", "workers", "timeout", "prune", "no-prompt", "resolve-secrets"} {
		if download.Flags().Lookup(flag) == nil {
			t.Errorf("resource download is missing --%s", flag)
		}
	}

	// Drift fetches and transforms like a download, so it honours --timeout and
	// --resolve-secrets; its own switches are --domain and --exit-code. It must
	// not offer the write-path switches it ignores (prune, no-prompt).
	driftCmd := subcommand(t, resourceCmd, "drift")
	for _, flag := range []string{"timeout", "resolve-secrets", "domain", "exit-code"} {
		if driftCmd.Flags().Lookup(flag) == nil {
			t.Errorf("resource drift is missing --%s", flag)
		}
	}
	for _, flag := range []string{"prune", "no-prompt"} {
		if driftCmd.Flags().Lookup(flag) != nil {
			t.Errorf("resource drift offers --%s but ignores it", flag)
		}
	}
}

// TestResourceAuthFlagPairEnforced guards the client-id/tenant-id contract
// across the parent/child boundary. Cobra validates flag groups against the
// flag set of the command it runs, so MarkFlagsRequiredTogether on the parent
// would not constrain a subcommand: the group enforces the pairing in its
// persistent pre-run instead, and this test is what keeps that from being
// dropped as redundant.
func TestResourceAuthFlagPairEnforced(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "neither flag", args: []string{"list"}, wantErr: false},
		{name: "client-id without tenant-id", args: []string{"list", "--client-id", "app-id"}, wantErr: true},
		{name: "tenant-id without client-id", args: []string{"list", "--tenant-id", "tenant"}, wantErr: true},
		{name: "both flags", args: []string{"list", "--client-id", "app-id", "--tenant-id", "tenant"}, wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)

			resourceCmd := newResourceCommand()
			// Replace the subcommand's action: this test is about flag
			// validation, which runs before RunE, not about listing resources.
			list := subcommand(t, resourceCmd, "list")
			list.RunE = func(cmd *cobra.Command, args []string) error { return nil }

			resourceCmd.SetArgs(tt.args)
			resourceCmd.SetOut(nil)
			resourceCmd.SilenceUsage = true
			resourceCmd.SilenceErrors = true

			err := resourceCmd.Execute()
			if tt.wantErr && err == nil {
				t.Error("expected an error for a half-specified device-code pair, got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestBindFlagsBindsInheritedFlags guards the binding helper against the
// failure the grouping introduces: a flag declared once on a command group is
// inherited by its subcommands rather than local to them, so binding only local
// flags would drop it from the flag > env > config > default chain. The symptom
// is one-directional and easy to miss — the flag keeps working on the command
// line while the config file and AZURE_RD_* silently stop applying — so this
// asserts both directions through a real execution.
func TestBindFlagsBindsInheritedFlags(t *testing.T) {
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		viper.Reset()
		t.Cleanup(viper.Reset)

		// A config file supplies the value the flag must be able to override.
		viper.SetConfigType("yaml")
		if err := viper.ReadConfig(strings.NewReader("subscription: sub-from-config\n")); err != nil {
			t.Fatalf("reading test config: %v", err)
		}

		resourceCmd := newResourceCommand()
		list := subcommand(t, resourceCmd, "list")

		var got string
		list.RunE = func(cmd *cobra.Command, args []string) error {
			cmdutil.BindFlags(cmd)
			got = viper.GetString("subscription")
			return nil
		}

		resourceCmd.SetArgs(append([]string{"list"}, args...))
		resourceCmd.SilenceUsage = true
		resourceCmd.SilenceErrors = true
		if err := resourceCmd.Execute(); err != nil {
			t.Fatalf("executing resource list: %v", err)
		}
		return got
	}

	t.Run("config applies when the flag is not set", func(t *testing.T) {
		if got := run(t); got != "sub-from-config" {
			t.Errorf("subscription = %q, want %q; a group-declared flag must still resolve from the config file", got, "sub-from-config")
		}
	})

	t.Run("flag wins over config", func(t *testing.T) {
		if got := run(t, "--subscription", "sub-from-flag"); got != "sub-from-flag" {
			t.Errorf("subscription = %q, want %q; the flag must outrank the config file", got, "sub-from-flag")
		}
	})
}
