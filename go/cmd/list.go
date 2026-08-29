package cmd

import (
	"azure-resource-downloader/internal/cmdutil"
	"os"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List supported resource types",
	Long: `List all Azure resource types that are currently supported by the tool.

This command shows which resource types have handlers registered and can be
downloaded.`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)

	// list only needs authentication to construct the client; selection and
	// pipeline-tuning flags would be misleading here (list ignores them).
	cmdutil.AddAzureAuthFlags(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	// Bind this command's local flags to viper before reading any values so the
	// flag > env > config > default precedence holds without a sibling command
	// stealing the binding.
	cmdutil.BindFlags(cmd)

	sub := viper.GetString("subscription")
	log := logger.Default

	// The supported types are baked into the binary, so enumerating them needs
	// neither a subscription nor a signed-in session. Build a lazy credential
	// (no network, no token fetch until first use) purely so the registry's
	// handler constructors succeed — the same offline path download uses for its
	// probe registry. This keeps `azure-rd list` usable as documentation, even
	// offline or without 'az login'.
	cred, err := azure.NewCredential(viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		// Runtime error - print and exit without showing help
		log.Error("Failed to prepare Azure credentials", "error", err)
		os.Exit(1)
	}

	// Create handler registry pre-populated with all supported resource types.
	// Secret resolution is a download-only concern, so it is always disabled here.
	registry := handlers.NewRegistry(cred, sub, false)

	// Get and display all registered types
	types := registry.GetAllTypes()

	log.Info("Supported Azure Resource Types", "count", len(types))

	// List each type
	for i, resourceType := range types {
		log.Info("",
			"number", i+1,
			"azure_type", resourceType)
	}

	log.Info("To add more resource types, implement a new handler and register it in internal/handlers/defaults.go")

	return nil
}
