// Package docs implements the `azure-rd docs ...` command group: operations on
// an existing export's documentation tree. Commands here never download or
// fetch a resource — they read the export already on disk and act on it.
//
// The `docs` parent command lives here in package cmd (like download and list),
// while each subcommand is a file in its own directory under cmd/docs/. Those
// subcommand packages expose an exported constructor (e.g.
// docs.NewGeneratePromptCommand) that this parent attaches; they cannot import
// package cmd (that would be an import cycle), so shared flag helpers come from
// internal/cmdutil instead.
package cmd

import (
	"azure-resource-downloader/cmd/docs"

	"github.com/spf13/cobra"
)

// NewCommand builds the `docs` parent command with its subcommands attached.
// The root command registers it via rootCmd.AddCommand(NewCommand()).
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Work with the generated documentation for an export",
		Long: `Commands that operate on an existing export's documentation tree.

These commands never download or fetch a resource: they read the export already
on disk (resources/ and its metadata.yaml) and the documents under docs/. Run a
download first to refresh the export, then a docs command to act on it.`,
	}

	cmd.AddCommand(docs.NewGeneratePromptCommand())
	cmd.AddCommand(docs.NewGenerateIndexCommand())
	return cmd
}
