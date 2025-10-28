package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
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
  - By resource type: --type "Microsoft.Storage/storageAccounts" (downloads all resources of that type)
  - By resource group: --resource-group "my-rg" (downloads the resource group itself)

The subscription ID is optional. If not specified, the default subscription from your 'az login' session will be used.

Examples:
  # Download a specific resource (uses default subscription from az login)
  azure-rd download --resource-id "/subscriptions/.../resourceGroups/my-rg"
  
  # Download all resources of a specific type
  azure-rd download --type "Microsoft.Resources/resourceGroups"
  azure-rd download --type "Microsoft.Storage/storageAccounts"
  
  # Download all resources in a resource group with explicit subscription
  azure-rd download --subscription "sub-id" --resource-group "my-rg"
  
  # Dry run to see what would be downloaded
  azure-rd download --type "Microsoft.Compute/virtualMachines" --dry-run`,
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
	excludeKeys := viper.GetStringSlice("exclude-keys")

	// Get resource-type-specific exclusions
	excludeKeysByType := make(map[string][]string)
	if viper.IsSet("exclude-keys-by-type") {
		excludeKeysByTypeConfig := viper.GetStringMap("exclude-keys-by-type")
		for resourceType, keys := range excludeKeysByTypeConfig {
			if keyList, ok := keys.([]interface{}); ok {
				strKeys := make([]string, 0, len(keyList))
				for _, k := range keyList {
					if strKey, ok := k.(string); ok {
						strKeys = append(strKeys, strKey)
					}
				}
				excludeKeysByType[resourceType] = strKeys
			}
		}
	}

	// Validate input
	if len(resourceIDs) == 0 && resourceGroup == "" && resourceType == "" {
		return fmt.Errorf("at least one of --resource-id, --resource-group, or --type must be specified")
	}

	log := logger.Default

	if sub == "" {
		log.Info("No subscription specified, will use default from Azure CLI session")
	}

	log.Info("Azure Resource Downloader",
		"subscription", func() string {
			if sub == "" {
				return "<default>"
			}
			return sub
		}(),
		"output", output,
		"workers", workers,
		"dry_run", dryRun)

	// Create Azure client (will auto-detect subscription if not provided)
	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}

	// Get the actual subscription ID being used (may have been auto-detected)
	sub = azureClient.GetSubscriptionID()
	log.Info("Authentication successful", "subscription", sub)

	// Create handler registry and register handlers
	registry := handlers.NewRegistry()
	registerHandlers(registry, azureClient)

	log.Info("Registered resource type handlers", "count", len(registry.GetAllTypes()))

	// Build fetch requests
	requests, err := buildFetchRequests(ctx, azureClient, resourceIDs, resourceGroup, resourceType, sub)
	if err != nil {
		return err
	}

	if len(requests) == 0 {
		return fmt.Errorf("no resources to download")
	}

	log.Info("Preparing to download resources", "count", len(requests))

	// Create and configure pipeline
	pipelineConfig := &models.PipelineConfig{
		OutputDir:         output,
		WorkerCount:       workers,
		Timeout:           time.Duration(timeout) * time.Second,
		DryRun:            dryRun,
		SubscriptionID:    sub,
		ExcludeKeys:       excludeKeys,
		ExcludeKeysByType: excludeKeysByType,
	}

	p := pipeline.NewPipeline(azureClient, registry, pipelineConfig)

	// Execute pipeline
	log.Info("Starting pipeline execution...")
	summary, err := p.Execute(ctx, requests)
	if err != nil {
		return fmt.Errorf("pipeline execution failed: %w", err)
	}

	// Print summary
	summary.PrintSummary()

	if summary.FailedResources > 0 {
		return fmt.Errorf("pipeline completed with %d errors", summary.FailedResources)
	}

	log.Info("Download completed successfully")
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

	// Register Microsoft Graph handlers (tenant-level resources)
	if capHandler, err := handlers.NewConditionalAccessPolicyHandler(cred); err == nil {
		registry.Register("Microsoft.Graph/conditionalAccessPolicies", capHandler)
	}

	// Add more handlers here as needed
	// registry.Register("Microsoft.Network/virtualNetworks", handlers.NewVirtualNetworkHandler(cred, sub))
	// registry.Register("Microsoft.Sql/servers", handlers.NewSqlServerHandler(cred, sub))
}

// buildFetchRequests creates fetch requests from command-line arguments
func buildFetchRequests(ctx context.Context, azureClient *azure.Client, resourceIDs []string, resourceGroup, resourceType, subscriptionID string) ([]*models.FetchRequest, error) {
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

	// If resource type is specified, list all resources of that type
	if resourceType != "" {
		log := logger.Default
		log.Info("Listing all resources of type", "type", resourceType)

		resourceList, err := azureClient.ListResourcesByType(ctx, resourceType)
		if err != nil {
			log.Error("Failed to list resources", "type", resourceType, "error", err)
			return nil, fmt.Errorf("failed to list resources of type %s: %w", resourceType, err)
		}

		log.Info("Found resources", "count", len(resourceList))

		if len(resourceList) == 0 {
			log.Warn("No resources found",
				"type", resourceType,
				"note", "This could be due to: (1) No resources of this type exist, (2) Insufficient permissions, or (3) Resources exist in a different scope (e.g., tenant vs subscription)")
		}

		for _, resourceID := range resourceList {
			requests = append(requests, &models.FetchRequest{
				ResourceID:   resourceID,
				ResourceType: resourceType,
				Subscription: subscriptionID,
			})
		}
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
