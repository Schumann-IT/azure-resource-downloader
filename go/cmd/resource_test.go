package cmd

import (
	"path/filepath"
	"testing"

	"azure-resource-downloader/internal/config"

	"github.com/spf13/cobra"
)

// subcommand returns the named subcommand of the given parent, failing the test
// when it is absent.
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

// removedFlags are the flags that became configuration settings when the config
// file became the single source of truth. Each was a second way to say the same
// thing, and the dangerous way: a flag outranks the tenant's profile, so one
// tenant's credentials could be paired with another tenant's export, and a
// destructive or secret-writing switch could be typed instead of reviewed.
var removedFlags = []string{
	"subscription", "client-id", "tenant-id",
	"workers", "timeout", "resolve-secrets", "no-prompt", "prune",
}

// TestResourceGroupSharesFlags guards the grouping in both directions: the flags
// every subcommand honours are declared once on the parent and visible to all of
// them, and no subcommand offers a flag it would ignore.
func TestResourceGroupSharesFlags(t *testing.T) {
	resourceCmd := newResourceCommand()

	for _, name := range []string{"download", "drift", "types", "list"} {
		sub := subcommand(t, resourceCmd, name)
		for _, flag := range []string{"type", "resource-id", "resource-group", "domain"} {
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

	// --exit-code gates CI on drift being found, which only drift decides.
	driftCmd := subcommand(t, resourceCmd, "drift")
	if driftCmd.Flags().Lookup("exit-code") == nil {
		t.Error("resource drift is missing --exit-code")
	}
	for _, name := range []string{"download", "types", "list"} {
		sub := subcommand(t, resourceCmd, name)
		if sub.Flags().Lookup("exit-code") != nil {
			t.Errorf("resource %s offers --exit-code but ignores it", name)
		}
	}
}

// TestRemovedFlagsAreGone is the other half of making configuration the single
// source of truth: the settings that moved into the config file must not be
// offered anywhere on the command line. A leftover flag would keep outranking
// the tenant profile, which is exactly the defect the move removes.
func TestRemovedFlagsAreGone(t *testing.T) {
	resourceCmd := newResourceCommand()
	docsCmd := NewCommand()

	commands := map[string]*cobra.Command{
		"root":     rootCmd,
		"resource": resourceCmd,
		"docs":     docsCmd,
	}
	for _, name := range []string{"download", "drift", "types", "list"} {
		commands["resource "+name] = subcommand(t, resourceCmd, name)
	}
	for _, name := range []string{"generate-prompt", "generate-index", "analyze-drift"} {
		commands["docs "+name] = subcommand(t, docsCmd, name)
	}

	for label, cmd := range commands {
		for _, flag := range removedFlags {
			if cmd.Flags().Lookup(flag) != nil {
				t.Errorf("%s still offers --%s; it is a configuration setting now", label, flag)
			}
		}
	}
}

// TestSurvivingGlobalFlags pins the flags that deliberately stayed on the
// command line: the two that locate the configuration, and the ones that change
// only this invocation's verbosity, side effects or destination.
func TestSurvivingGlobalFlags(t *testing.T) {
	for _, flag := range []string{"config", "config-dir", "output", "dry-run", "log-level"} {
		if rootCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("root is missing the global --%s", flag)
		}
	}
	// --debug and --domain serve root's own diagnostic report, so they must not
	// be inherited by every command.
	for _, flag := range []string{"debug", "domain"} {
		if rootCmd.Flags().Lookup(flag) == nil {
			t.Errorf("root is missing --%s", flag)
		}
		if rootCmd.PersistentFlags().Lookup(flag) != nil {
			t.Errorf("--%s must be local to root, not persistent", flag)
		}
	}
}

// TestDomainCompletionListsProfilesAndExports guards the discovery path: tab
// completion on --domain answers "which tenants do I have?" from the config
// directory and the existing exports, locally and with no network call. It also
// pins the base-file exclusion — base.yaml is configuration, never a tenant.
func TestDomainCompletionListsProfilesAndExports(t *testing.T) {
	configDir := t.TempDir()
	write(t, filepath.Join(configDir, config.BaseFileName), "")
	write(t, filepath.Join(configDir, "contoso.example.com.yaml"), "")

	outputDir := t.TempDir()
	exportMeta := filepath.Join(outputDir, "fabrikam.example.com", "resources")
	mkdirAll(t, exportMeta)
	write(t, filepath.Join(exportMeta, "metadata.yaml"), "tenant: fabrikam.example.com\n")

	resourceCmd := newResourceCommand()
	download := subcommand(t, resourceCmd, "download")
	// Make the flags resolvable on the command that is completing: the
	// completion function reads them directly, because the hidden __complete
	// invocation is not guaranteed to have loaded any configuration.
	download.Flags().String("config-dir", configDir, "")
	download.Flags().String("output", outputDir, "")

	complete, ok := download.GetFlagCompletionFunc("domain")
	if !ok {
		t.Fatal("no completion function registered for --domain")
	}
	got, directive := complete(download, nil, "")

	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("directive = %v, want NoFileComp (a tenant domain is not a file)", directive)
	}
	want := []string{"contoso.example.com", "fabrikam.example.com"}
	if len(got) != len(want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidates = %v, want %v (sorted, base.yaml excluded)", got, want)
		}
	}
}
