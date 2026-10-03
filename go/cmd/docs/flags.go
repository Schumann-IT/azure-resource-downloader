package docs

import (
	"azure-resource-downloader/internal/cmdutil"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"

	"github.com/spf13/cobra"
)

// addExportFlags declares the flags the docs subcommands that write one file
// share: --domain to name the export folder (and select that tenant's
// configuration profile), and --out to override the output path. Only --out's
// usage differs per command (each writes a different file with a different
// default), so it is the one parameter; the flags stay local to each
// subcommand rather than persistent on the docs parent for the same reason.
//
// The credentials these commands may need to resolve the tenant domain come
// from that profile, not from flags, so there is exactly one place a tenant's
// identity is written down.
func addExportFlags(cmd *cobra.Command, outUsage string) {
	addDomainFlag(cmd)
	cmd.Flags().String("out", "", outUsage)
}

// addDomainFlag declares --domain and its completion alone, for a docs command
// that writes a fixed tree and so has no --out to honour.
func addDomainFlag(cmd *cobra.Command) {
	cmd.Flags().String("domain", "", "export tenant domain (folder name under --output); selects <config-dir>/<domain>.yaml and skips authentication, running offline")
	cmdutil.RegisterDomainCompletion(cmd)
}

// exportCredential returns the credential a docs command resolves the tenant
// domain with: none when --domain was passed (the run is offline and never
// signs in), otherwise the one the tenant's profile names.
func exportCredential(domain string) azcore.TokenCredential {
	if domain != "" {
		return nil
	}
	return cmdutil.ProfileCredential()
}
