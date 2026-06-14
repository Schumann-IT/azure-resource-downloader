package cmd

import (
	"context"
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
	
This command shows which resource types have handlers registered and can be downloaded.`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	sub := viper.GetString("subscription")
	log := logger.Default

	// Create Azure client (will auto-detect subscription if not provided)
	// Note: list command doesn't really need a subscription, but we create the client
	// to ensure authentication works and to maintain consistency
	azureClient, err := azure.NewClient(ctx, sub, viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		// Runtime error - print and exit without showing help
		log.Error("Failed to create Azure client", "error", err)
		os.Exit(1)
	}

	// Get the actual subscription ID being used
	sub = azureClient.GetSubscriptionID()
	log.Info("Using subscription", "subscription", sub)

	// Create handler registry pre-populated with all supported resource types.
	// Secret resolution is a download-only concern, so it is always disabled here.
	registry := handlers.NewRegistry(azureClient.GetCredential(), azureClient.GetSubscriptionID(), false)

	// Get and display all registered types
	types := registry.GetAllTypes()

	log.Info("Supported Azure Resource Types", "count", len(types))

	// List each type
	for i, resourceType := range types {
		handler, _ := registry.Get(resourceType)
		terraformType := handler.GetTerraformResourceType()
		log.Info("",
			"number", i+1,
			"azure_type", resourceType,
			"terraform_type", terraformType)
	}

	log.Info("To add more resource types, implement a new handler and register it in internal/handlers/defaults.go")

	return nil
}
