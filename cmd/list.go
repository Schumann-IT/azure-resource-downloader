package cmd

import (
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"

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

	if sub == "" {
		return fmt.Errorf("subscription ID is required")
	}

	// Create Azure client
	azureClient, err := azure.NewClient(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}

	// Create handler registry and register handlers
	registry := handlers.NewRegistry()
	registerHandlers(registry, azureClient)

	// Get and display all registered types
	types := registry.GetAllTypes()

	fmt.Printf("📋 Supported Azure Resource Types (%d total):\n\n", len(types))
	for i, resourceType := range types {
		handler, _ := registry.Get(resourceType)
		terraformType := handler.GetTerraformResourceType()
		fmt.Printf("%2d. %-50s -> %s\n", i+1, resourceType, terraformType)
	}

	fmt.Println("\n💡 To add more resource types, implement a new handler and register it in cmd/download.go")

	return nil
}
