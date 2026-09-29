// The `resource` parent command lives here in package cmd (like docs), while
// each subcommand is a file in its own directory under cmd/resource/. Those
// subcommand packages expose an exported constructor (e.g.
// resource.NewDownloadCommand) that this parent attaches; they cannot import
// package cmd (that would be an import cycle), so shared flag helpers come from
// internal/cmdutil instead.
package cmd

import (
	"azure-resource-downloader/cmd/resource"
	"azure-resource-downloader/internal/cmdutil"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// newResourceCommand builds the `resource` parent command with its subcommands
// attached. The root command registers it via rootCmd.AddCommand.
func newResourceCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource",
		Short: "Work with a tenant's Azure resources",
		Long: `Commands that act on the Azure resources of a tenant: see which resource
types this build supports (types), see what the tenant contains (list),
download it as clean YAML (download), and detect drift between the tenant and
the export on disk (drift).

The flags these commands share are declared here, on the group, so every
subcommand resolves them identically. All are optional: with no selection, a
command covers every registered resource type.

Authentication reuses your 'az login' session by default. To reach Microsoft
Graph/Intune types that need scopes the Azure CLI app cannot provide, put a
dedicated app registration's client-id and tenant-id in the tenant's
configuration profile (see config.example.domain.yaml).`,
	}

	// Shared flags live on the group, not on each subcommand. Only groups every
	// subcommand honours belong here: --domain names the tenant all four act on
	// (and selects its configuration profile), and the selection flags scope
	// what download and drift fetch, what list enumerates and what types shows
	// and counts. Everything else these commands need — credentials, worker
	// counts, timeouts, the write-path switches — comes from the configuration
	// file, which is the single source of truth for it.
	cmdutil.AddPersistentSelectionFlags(cmd)
	cmd.PersistentFlags().String("domain", "", "tenant domain to act on: selects <config-dir>/<domain>.yaml and the export under <output>; refused when it differs from the signed-in tenant")
	cmdutil.RegisterDomainCompletion(cmd)

	// Bind the group's config-backed and flag-only selection flags to viper
	// once. A single global binding is safe because each of these flags is
	// declared exactly once in the whole command tree, so no sibling command can
	// steal the binding — which is why the former per-execution re-binding is
	// gone. --domain is deliberately NOT bound: root and every docs subcommand
	// declare their own, so the value must be read from the running command.
	_ = viper.BindPFlag("type", cmd.PersistentFlags().Lookup("type"))
	_ = viper.BindPFlag("resource-id", cmd.PersistentFlags().Lookup("resource-id"))
	_ = viper.BindPFlag("resource-group", cmd.PersistentFlags().Lookup("resource-group"))

	cmd.AddCommand(resource.NewDownloadCommand())
	cmd.AddCommand(resource.NewDriftCommand())
	cmd.AddCommand(resource.NewTypesCommand())
	cmd.AddCommand(resource.NewListCommand())
	return cmd
}
