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
Graph/Intune types that need scopes the Azure CLI app cannot provide, sign in to
a dedicated app registration with --client-id/--tenant-id (device-code flow).`,
		// Cobra validates flag groups against the flag set of the command it
		// runs, so a required-together pairing declared on this parent would not
		// constrain the subcommand that actually executes. Enforce it here
		// instead, where the inherited flags are visible on cmd.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.RequireAuthFlagPair(cmd)
		},
	}

	// Shared flags live on the group, not on each subcommand. Only groups every
	// subcommand honours belong here: the authentication flags serve download's,
	// drift's and list's sign-in and types' (lazy, non-interactive) credential;
	// the selection flags scope what download and drift fetch, what list
	// enumerates and what types shows and counts; --workers bounds the listing
	// concurrency all four share. --timeout stays on download and drift: it
	// wraps each resource fetch, which only those two perform, so a sibling
	// advertising it would ignore it.
	//
	// The auth group is deliberately a second registration: root declares the
	// same flags locally for `azure-rd --debug` (see root.go's init). Both are
	// needed — root's copy serves --debug, this one serves the group's
	// subcommands — and neither is redundant.
	cmdutil.AddPersistentAzureAuthFlags(cmd)
	cmdutil.AddPersistentSelectionFlags(cmd)
	cmdutil.AddPersistentWorkersFlag(cmd)

	cmd.AddCommand(resource.NewDownloadCommand())
	cmd.AddCommand(resource.NewDriftCommand())
	cmd.AddCommand(resource.NewTypesCommand())
	cmd.AddCommand(resource.NewListCommand())
	return cmd
}
