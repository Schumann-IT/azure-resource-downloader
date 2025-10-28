package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	resourceIDs   []string
	resourceType  string
	resourceGroup string
	timeout       int
)

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download Azure resources",
	Long: `Download Azure resources and transform them into YAML format with Terraform import statements.

You can specify resources in multiple ways:
  - By resource ID: --resource-id "/subscriptions/.../resourceGroups/my-rg"
  - By type: --type "Microsoft.Storage/storageAccounts"
  - By resource group: --resource-group "my-rg"

Examples:
  # Download a specific resource
  azure-rd download --subscription "sub-id" --resource-id "/subscriptions/.../resourceGroups/my-rg"
  
  # Download all resources in a resource group
  azure-rd download --subscription "sub-id" --resource-group "my-rg"
  
  # Dry run to see what would be downloaded
  azure-rd download --subscription "sub-id" --resource-group "my-rg" --dry-run`,
	RunE: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	// Download-specific flags
	downloadCmd.Flags().StringSliceVar(&resourceIDs, "resource-id", []string{}, "Azure resource IDs to download (can be specified multiple times)")
	downloadCmd.Flags().StringVar(&resourceType, "type", "", "Azure resource type to download")
	downloadCmd.Flags().StringVar(&resourceGroup, "resource-group", "", "Resource group name")
	downloadCmd.Flags().IntVar(&timeout, "timeout", 300, "timeout in seconds for the download operation")
}

func runDownload(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration
	sub := viper.GetString("subscription")
	output := viper.GetString("output")
	workers := viper.GetInt("workers")
	dryRun := viper.GetBool("dry-run")

	if sub == "" {
		return fmt.Errorf("subscription ID is required")
	}

	// Validate input
	if len(resourceIDs) == 0 && resourceGroup == "" {
		return fmt.Errorf("either --resource-id or --resource-group must be specified")
	}

	fmt.Printf("🚀 Azure Resource Downloader\n")
	fmt.Printf("   Subscription: %s\n", sub)
	fmt.Printf("   Output: %s\n", output)
	fmt.Printf("   Workers: %d\n", workers)
	fmt.Printf("   Dry Run: %v\n\n", dryRun)

	// Create Azure client
	fmt.Println("🔐 Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}
	fmt.Println("✅ Authentication successful\n")

	// Create handler registry and register handlers
	registry := handlers.NewRegistry()
	registerHandlers(registry, azureClient)

	fmt.Printf("📦 Registered %d resource type handlers\n\n", len(registry.GetAllTypes()))

	// Build fetch requests
	requests, err := buildFetchRequests(resourceIDs, resourceGroup, resourceType, sub)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		return fmt.Errorf("no resources to download")
	}

	fmt.Printf("📥 Preparing to download %d resource(s)...\n\n", len(requests))

	// Create and configure pipeline
	pipelineConfig := &models.PipelineConfig{
		OutputDir:      output,
		WorkerCount:    workers,
		Timeout:        time.Duration(timeout) * time.Second,
		DryRun:         dryRun,
		SubscriptionID: sub,
	}

	p := pipeline.NewPipeline(azureClient, registry, pipelineConfig)

	// Execute pipeline
	fmt.Println("⚡ Starting pipeline execution...")
	summary, err := p.Execute(ctx, requests)
	if err != nil {
		return fmt.Errorf("pipeline execution failed: %w", err)
	}

	// Print summary
	summary.PrintSummary()

	if summary.FailedResources > 0 {
		return fmt.Errorf("pipeline completed with %d errors", summary.FailedResources)
	}

	fmt.Println("\n✨ Download completed successfully!")
	return nil
}

// registerHandlers registers all available resource handlers
func registerHandlers(registry *handlers.Registry, azureClient *azure.Client) {
	cred := azureClient.GetCredential()
	sub := azureClient.GetSubscriptionID()

	// Register handlers for supported resource types
	registry.Register("Microsoft.Resources/resourceGroups", handlers.NewResourceGroupHandler(cred, sub))
	registry.Register("Microsoft.Storage/storageAccounts", handlers.NewStorageAccountHandler(cred, sub))
	registry.Register("Microsoft.Compute/virtualMachines", handlers.NewVirtualMachineHandler(cred, sub))

	// Add more handlers here as needed
	// registry.Register("Microsoft.Network/virtualNetworks", handlers.NewVirtualNetworkHandler(cred, sub))
	// registry.Register("Microsoft.Sql/servers", handlers.NewSqlServerHandler(cred, sub))
}

// buildFetchRequests creates fetch requests from command-line arguments
func buildFetchRequests(resourceIDs []string, resourceGroup, resourceType, subscriptionID string) ([]*models.FetchRequest, error) {
	var requests []*models.FetchRequest

	// If specific resource IDs are provided, use them
	if len(resourceIDs) > 0 {
		for _, id := range resourceIDs {
			requests = append(requests, &models.FetchRequest{
				ResourceID:   id,
				Subscription: subscriptionID,
			})
		}
		return requests, nil
	}

	// If resource group is specified, build resource ID
	if resourceGroup != "" {
		rgID := fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscriptionID, resourceGroup)
		requests = append(requests, &models.FetchRequest{
			ResourceID:    rgID,
			ResourceType:  "Microsoft.Resources/resourceGroups",
			ResourceGroup: resourceGroup,
			Subscription:  subscriptionID,
		})
		return requests, nil
	}

	return requests, nil
}

// parseResourceType extracts the resource type from a resource ID
func parseResourceType(resourceID string) string {
	parts := strings.Split(strings.Trim(resourceID, "/"), "/")

	for i, part := range parts {
		if strings.EqualFold(part, "providers") && i+2 < len(parts) {
			return parts[i+1] + "/" + parts[i+2]
		}
	}

	return ""
}
