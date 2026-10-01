// Package cmdutil holds CLI helpers shared by the root command package and its
// subcommand packages (e.g. cmd/docs). It exists so a subcommand living in its
// own directory can reuse the flag-group helpers, the exit-code plumbing and the
// interactive prompts without importing package cmd, which would create an
// import cycle (package cmd must import the subcommand packages to register
// them).
//
// It holds no viper-binding helper: with configuration as the single source of
// truth, each surviving flag is declared exactly once in the command tree and is
// bound (or read) at that one site, so there is nothing left to re-bind per
// execution.
package cmdutil

import (
	"os"
	"path/filepath"
	"sort"

	"azure-resource-downloader/internal/config"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/models"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Built-in defaults for settings that no longer have a flag. They are defined
// by the config package, which owns the configuration surface, and re-exported
// here so command code has one obvious place to reach for them.
const (
	DefaultWorkerCount    = config.DefaultWorkerCount
	DefaultTimeoutSeconds = config.DefaultTimeoutSeconds
)

// AddSelectionFlags registers the selection triad locally on cmd:
// --resource-id, --type and --resource-group. These stay on the command line
// because scoping a single run is the ad-hoc case: --type replaces the
// configured type list for this invocation (the tenant profile's exclude-type
// still applies, and naming an excluded type is refused), and the two ARM
// selectors have no config key at all, so a stale entry in a tenant profile
// can never silently scope every future run.
func AddSelectionFlags(cmd *cobra.Command) {
	defineSelectionFlags(cmd.Flags())
}

// AddPersistentSelectionFlags registers the selection triad on cmd's persistent
// flag set, for a command group whose every subcommand honours it.
func AddPersistentSelectionFlags(cmd *cobra.Command) {
	defineSelectionFlags(cmd.PersistentFlags())
}

// defineSelectionFlags is the single definition of the selection triad.
func defineSelectionFlags(f *pflag.FlagSet) {
	f.StringSlice("resource-id", []string{}, "explicit Azure resource ID to act on; repeatable")
	f.StringSlice("type", []string{}, "resource type to act on; repeatable, replaces the configured types for this run; the tenant profile's exclude-type still applies (default: all registered types)")
	f.String("resource-group", "", "act on resources in this resource group")
}

// DeclaredDomain returns cmd's --domain value only when it was actually passed
// on the command line. Only an explicit value may select a configuration
// profile or assert which tenant a run acts on: a flag's zero default would
// otherwise read as "the tenant named empty string", and for a download the
// real tenant is not known until after authentication.
//
// Every caller goes through this one reader rather than viper, because --domain
// is declared separately on root, on the resource group and on each docs
// subcommand, so a global binding could resolve to a sibling's copy.
func DeclaredDomain(cmd *cobra.Command) string {
	flag := cmd.Flags().Lookup("domain")
	if flag == nil || !flag.Changed {
		return ""
	}
	return flag.Value.String()
}

// RegisterDomainCompletion offers the tenant domains a run could act on as
// shell completions for cmd's --domain flag: the profiles in --config-dir and
// the export directories under --output. Every command that declares --domain
// registers it, so tab completion answers "which tenants do I have?" the same
// way everywhere.
//
// It stays local and cheap — never an Azure call on a Tab press — and reads the
// flags directly rather than through viper, because the hidden __complete
// invocation is not guaranteed to have loaded any configuration.
func RegisterDomainCompletion(cmd *cobra.Command) {
	_ = cmd.RegisterFlagCompletionFunc("domain",
		func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			dir, _ := cmd.Flags().GetString("config-dir")
			output, _ := cmd.Flags().GetString("output")

			seen := map[string]bool{}
			var candidates []string
			for _, domain := range append(config.ProfileCandidates(dir), ExportDomains(output)...) {
				if seen[domain] {
					continue
				}
				seen[domain] = true
				candidates = append(candidates, domain)
			}
			sort.Strings(candidates)
			return candidates, cobra.ShellCompDirectiveNoFileComp
		})
}

// ExportDomains lists the tenant directories that already hold an export under
// baseOutput. Like the profile listing it never fails: these are completion
// candidates, so an unreadable directory simply contributes none.
func ExportDomains(baseOutput string) []string {
	if baseOutput == "" {
		return nil
	}
	entries, err := os.ReadDir(baseOutput)
	if err != nil {
		return nil
	}
	var domains []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		metaPath := filepath.Join(baseOutput, entry.Name(), models.ResourcesDirName, docs.MetadataFileName)
		if info, err := os.Stat(metaPath); err == nil && !info.IsDir() {
			domains = append(domains, entry.Name())
		}
	}
	sort.Strings(domains)
	return domains
}
