package cmd

import (
	"context"
	"fmt"
	"os"
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
	resourceTypes []string
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
  - By resource type: --type "Microsoft.Storage/storageAccounts" (repeatable; downloads all resources of the given type(s))
  - By resource group: --resource-group "my-rg" (downloads the resource group itself)

The --type flag acts as a filter and may be specified multiple times. If no
--type (and no --resource-id/--resource-group) is given, all registered
resource types are downloaded.

The subscription ID is optional. If not specified, the default subscription from your 'az login' session will be used.

Examples:
  # Download a specific resource (uses default subscription from az login)
  azure-rd download --resource-id "/subscriptions/.../resourceGroups/my-rg"
  
  # Download all resources of one or more types
  azure-rd download --type "Microsoft.Resources/resourceGroups"
  azure-rd download --type "Microsoft.Storage/storageAccounts" --type "Microsoft.Compute/virtualMachines"

  # Download every registered resource type (no --type filter)
  azure-rd download
  
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
	downloadCmd.Flags().StringSliceVar(&resourceTypes, "type", []string{}, "Azure resource type(s) to download; repeatable. Acts as a filter \u2014 if omitted, all registered types are downloaded")
	downloadCmd.Flags().StringVar(&resourceGroup, "resource-group", "", "Resource group name")
	downloadCmd.Flags().IntVar(&timeout, "timeout", 300, "timeout in seconds for the download operation")
}

func runDownload(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration
	sub := viper.GetString("subscription")
	output := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	workersFlag := viper.GetInt("workers")

	// Build worker configuration
	workerConfig := buildWorkerConfig()

	log := logger.Default

	// Build transformer configurations
	transformerConfigs := buildTransformerConfigs()

	// Log which transformers will be used
	if len(transformerConfigs) == 0 {
		log.Info("No transformers enabled - raw Azure data will be output")
	} else {
		transformerNames := make([]string, len(transformerConfigs))
		for i, tc := range transformerConfigs {
			transformerNames[i] = tc.Name
		}
		log.Info("Active transformers", "transformers", transformerNames, "count", len(transformerConfigs))

		// Debug: show detailed config for each transformer
		for _, tc := range transformerConfigs {
			if len(tc.Config) > 0 {
				log.Debug("Transformer configuration",
					"name", tc.Name,
					"config", tc.Config)
			} else {
				log.Debug("Transformer configuration",
					"name", tc.Name,
					"config", "default")
			}
		}
	}

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
		"workers", workersFlag,
		"dry_run", dryRun)

	// Create Azure client (will auto-detect subscription if not provided)
	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub)
	if err != nil {
		// Runtime error - print and exit without showing help
		log.Error("Failed to create Azure client", "error", err)
		os.Exit(1)
	}

	// Get the actual subscription ID being used (may have been auto-detected)
	sub = azureClient.GetSubscriptionID()
	log.Info("Authentication successful", "subscription", sub)

	// Create handler registry and register handlers
	registry := handlers.NewRegistry()
	registerHandlers(registry, azureClient)

	log.Info("Registered resource type handlers", "count", len(registry.GetAllTypes()))

	// Build fetch requests
	requests, err := buildFetchRequests(ctx, registry, resourceIDs, resourceGroup, resourceTypes, sub)
	if err != nil {
		// Runtime error - print and exit without showing help
		log.Error("Failed to build fetch requests", "error", err)
		os.Exit(1)
	}

	if len(requests) == 0 {
		// Runtime error - print and exit without showing help
		log.Error("No resources to download")
		os.Exit(1)
	}

	log.Info("Preparing to download resources", "count", len(requests))

	// Worker tuning is API-specific and only meaningful when a single type is
	// targeted. With multiple types (or all registered types), treat as mixed.
	effectiveType := ""
	if len(resourceTypes) == 1 {
		effectiveType = resourceTypes[0]
	}

	// Determine worker count based on resource type and API
	workers := determineWorkerCount(workerConfig, effectiveType, requests, workersFlag)

	log.Info("Worker configuration",
		"workers", workers,
		"resource_type", func() string {
			if effectiveType != "" {
				return effectiveType
			}
			return "mixed"
		}(),
		"api", func() string {
			if effectiveType != "" {
				return string(models.DetectAPIType(effectiveType))
			}
			return "auto-detected"
		}())

	// Warn if using too many workers based on API type
	if effectiveType != "" {
		shouldWarn, rateLimitInfo := models.ShouldWarnAboutWorkerCount(effectiveType, workers)
		if shouldWarn {
			apiConfig := models.GetAPIConfig(effectiveType)
			log.Warn("Worker count exceeds recommendation for this API",
				"workers", workers,
				"resource_type", effectiveType,
				"api", apiConfig.Name,
				"recommended_workers", apiConfig.RecommendedWorkers,
				"max_recommended", apiConfig.MaxRecommendedWorkers,
				"rate_limit", rateLimitInfo,
				"note", "More workers can SLOW DOWN downloads due to rate limits and exponential backoff")
		}
	}

	// Log transformer configuration
	if len(transformerConfigs) > 0 {
		transformerNames := make([]string, len(transformerConfigs))
		for i, tc := range transformerConfigs {
			transformerNames[i] = tc.Name
		}
		defaultConfigs := models.DefaultTransformerConfigs()
		if len(transformerConfigs) != len(defaultConfigs) {
			log.Info("Custom transformers configured", "transformers", transformerNames)
		}
	}

	// Create and configure pipeline
	pipelineConfig := &models.PipelineConfig{
		OutputDir:          output,
		WorkerCount:        workers,
		Timeout:            time.Duration(timeout) * time.Second,
		DryRun:             dryRun,
		SubscriptionID:     sub,
		TransformerConfigs: transformerConfigs,
	}

	p := pipeline.NewPipeline(azureClient, registry, pipelineConfig)

	// Execute pipeline
	log.Info("Starting pipeline execution...")
	summary, err := p.Execute(ctx, requests)
	if err != nil {
		// Runtime error - print and exit without showing help
		log.Error("Pipeline execution failed", "error", err)
		os.Exit(1)
	}

	// Print summary
	summary.PrintSummary()

	if summary.FailedResources > 0 {
		// Runtime error - print and exit without showing help
		log.Error("Pipeline completed with errors", "failed", summary.FailedResources)
		os.Exit(1)
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
	if aspHandler, err := handlers.NewAuthenticationStrengthPolicyHandler(cred); err == nil {
		registry.Register("Microsoft.Graph/authenticationStrengthPolicies", aspHandler)
	}

	// Register Intune (Microsoft Graph beta) handlers
	if dmcpHandler, err := handlers.NewDeviceManagementConfigurationPolicyHandler(cred); err == nil {
		registry.Register("Microsoft.Graph/deviceManagementConfigurationPolicies", dmcpHandler)
	}
	secretOpts := handlers.SecretResolutionOptions{
		Enabled:  viper.GetBool("resolve-secrets"),
		ClientID: viper.GetString("secrets-client-id"),
		TenantID: viper.GetString("secrets-tenant-id"),
	}
	if secretOpts.Enabled {
		logger.Default.Warn("Secret resolution enabled - encrypted Intune OMA-URI values will be written to output in plaintext; you will be prompted for a delegated device-code sign-in as an Intune admin",
			"flag", "--resolve-secrets")
	}
	if dcHandler, err := handlers.NewDeviceConfigurationHandler(cred, secretOpts); err == nil {
		registry.Register("Microsoft.Graph/deviceConfigurations", dcHandler)
	}

	// Add more handlers here as needed
	// registry.Register("Microsoft.Network/virtualNetworks", handlers.NewVirtualNetworkHandler(cred, sub))
	// registry.Register("Microsoft.Sql/servers", handlers.NewSqlServerHandler(cred, sub))
}

// buildFetchRequests creates fetch requests from command-line arguments.
//
// Selection precedence: explicit --resource-id, then --resource-group, then
// type listing. The resourceTypes slice acts as a filter on the registered
// handlers: when empty, all registered types are considered.
func buildFetchRequests(ctx context.Context, registry *handlers.Registry, resourceIDs []string, resourceGroup string, resourceTypes []string, subscriptionID string) ([]*models.FetchRequest, error) {
	var requests []*models.FetchRequest
	log := logger.Default

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

	// Otherwise, list resources by type. --type acts as a filter; when no type
	// is given, every registered type is considered.
	types := resourceTypes
	if len(types) == 0 {
		types = registry.GetAllTypes()
		log.Info("No --type filter given, considering all registered types", "count", len(types))
	}

	for _, resourceType := range types {
		handler, err := registry.Get(resourceType)
		if err != nil {
			log.Error("No handler for resource type", "type", resourceType, "error", err)
			return nil, fmt.Errorf("no handler registered for resource type %s: %w", resourceType, err)
		}

		log.Info("Listing all resources of type", "type", resourceType)

		resourceList, err := handler.List(ctx)
		if err != nil {
			// Don't abort the whole run because one type fails (e.g. missing
			// permissions for a single Graph collection); log and continue so
			// the remaining types are still downloaded.
			log.Warn("Failed to list resources, skipping type", "type", resourceType, "error", err)
			continue
		}

		log.Info("Found resources", "type", resourceType, "count", len(resourceList))

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

// buildWorkerConfig constructs worker configuration from config file
func buildWorkerConfig() *models.WorkerConfig {
	config := models.DefaultWorkerConfig()

	// Read general workers setting from config (overrides defaults)
	if viper.IsSet("workers") {
		generalWorkers := viper.GetInt("workers")
		if generalWorkers > 0 {
			config.Default = generalWorkers
			// Don't override API-specific defaults yet - those come from workers-by-api
		}
	}

	// Read API-specific worker configuration (highest priority from config)
	if viper.IsSet("workers-by-api.microsoft-graph") {
		if graphWorkers := viper.GetInt("workers-by-api.microsoft-graph"); graphWorkers > 0 {
			config.MicrosoftGraph = graphWorkers
		}
	}
	if viper.IsSet("workers-by-api.azure-resource-manager") {
		if armWorkers := viper.GetInt("workers-by-api.azure-resource-manager"); armWorkers > 0 {
			config.AzureResourceManager = armWorkers
		}
	}

	return config
}

// determineWorkerCount determines the worker count based on resource type
func determineWorkerCount(workerConfig *models.WorkerConfig, resourceType string, requests []*models.FetchRequest, workersFlag int) int {
	// Priority 1: Check if --workers CLI flag was explicitly set (highest priority)
	// The flag value is passed in; if it's not the default (5), user set it explicitly
	if workersFlag != 5 {
		return workersFlag
	}

	// Priority 2: Use API-specific worker count based on resource type
	if resourceType != "" {
		return workerConfig.GetWorkerCount(resourceType)
	}

	// Priority 3: For mixed resource types, use safe default
	return workerConfig.Default
}

// buildTransformerConfigs constructs transformer configurations from viper
func buildTransformerConfigs() []models.TransformerConfig {
	log := logger.Default

	// Check if transformers key exists in config
	if !viper.IsSet("transformers") {
		// No transformers key at all - use defaults
		log.Debug("No 'transformers' key in config, using defaults")
		return models.DefaultTransformerConfigs()
	}

	// Get transformers configuration
	transformersConfig := viper.Get("transformers")

	// Debug: show what we got from viper
	log.Debug("Raw transformers config from viper",
		"type", fmt.Sprintf("%T", transformersConfig),
		"value", transformersConfig)

	// Handle different config formats
	switch v := transformersConfig.(type) {
	case []interface{}:
		// List of transformer configs (could be empty list)
		log.Debug("Transformers config is a list",
			"length", len(v))

		if len(v) == 0 {
			// Explicitly empty list - user wants NO transformers
			log.Info("Transformers explicitly disabled via empty list: transformers: []")
			return []models.TransformerConfig{}
		}

		var configs []models.TransformerConfig
		for _, item := range v {
			if itemMap, ok := item.(map[string]interface{}); ok {
				// Full transformer config with name and config
				name, _ := itemMap["name"].(string)
				if name == "" {
					log.Warn("Transformer config missing 'name' field, skipping", "item", itemMap)
					continue
				}

				config := make(map[string]interface{})
				for key, value := range itemMap {
					if key != "name" {
						config[key] = value
					}
				}

				configs = append(configs, models.TransformerConfig{
					Name:   name,
					Config: config,
				})

				log.Debug("Loaded transformer config",
					"name", name,
					"config", config)
			} else if name, ok := item.(string); ok {
				// Simple string name (no config)
				configs = append(configs, models.TransformerConfig{
					Name:   name,
					Config: map[string]interface{}{},
				})

				log.Debug("Loaded transformer (simple format)", "name", name)
			} else {
				log.Warn("Unexpected transformer item type",
					"type", fmt.Sprintf("%T", item),
					"value", item)
			}
		}

		// If configs is still empty after processing, all items were invalid
		if len(configs) == 0 {
			log.Warn("Transformers list had no valid items, using defaults")
			return models.DefaultTransformerConfigs()
		}

		return configs

	case nil:
		// Explicit nil value (transformers: null or transformers: ~)
		log.Info("Transformers explicitly set to null - disabling all transformers")
		return []models.TransformerConfig{}

	default:
		log.Warn("Unexpected transformers configuration format, using defaults",
			"type", fmt.Sprintf("%T", v),
			"value", v)
		return models.DefaultTransformerConfigs()
	}
}
