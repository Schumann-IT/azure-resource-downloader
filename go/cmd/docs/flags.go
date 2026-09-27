package docs

import (
	"azure-resource-downloader/internal/cmdutil"

	"github.com/spf13/cobra"
)

// addExportFlags declares the flags every docs subcommand shares: the Azure
// auth group (the commands need nothing beyond the tenant domain, so the plain
// CLI credential is enough — the group is reused for --subscription/
// --client-id/--tenant-id parity with the other commands), --domain to run
// offline against a named export folder, and --out to override the output
// path. Only --out's usage differs per command (each writes a different file
// with a different default), so it is the one parameter; the flags stay local
// to each subcommand rather than persistent on the docs parent for the same
// reason.
func addExportFlags(cmd *cobra.Command, outUsage string) {
	cmdutil.AddAzureAuthFlags(cmd)

	f := cmd.Flags()
	f.String("domain", "", "export tenant domain (folder name under --output); skips authentication and runs offline")
	f.String("out", "", outUsage)
}
