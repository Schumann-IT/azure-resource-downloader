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
types this build supports, and download them as clean YAML.

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
	// subcommand honours belong here: the authentication flags are used by
	// download to sign in and by list to construct its (lazy) credential.
	// Selection and pipeline tuning stay on download until a sibling honours
	// them too, so no command ever advertises a flag it ignores.
	//
	// This is deliberately a second registration of the auth group: root
	// declares the same flags locally for `azure-rd --debug` (see root.go's
	// init). Both are needed — root's copy serves --debug, this one serves the
	// group's subcommands — and neither is redundant.
	cmdutil.AddPersistentAzureAuthFlags(cmd)

	cmd.AddCommand(resource.NewDownloadCommand())
	cmd.AddCommand(resource.NewListCommand())
	return cmd
}
