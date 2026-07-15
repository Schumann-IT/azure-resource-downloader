package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Download-specific flag variables are prefixed with "flag" so they never
// collide with (and shadow) local variables/params in command code that read
// the same settings back from Viper using natural names like timeout. These
// are referenced only for flag binding (and resourceIDs in runDownload).
var (
	flagResourceIDs    []string
	flagTimeout        int
	flagResolveSecrets bool
	flagWritePrompts   bool
)

// downloadCmd represents the download command
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download Azure resources",
	Long: `Download Azure resources and transform them into YAML format. With
--write-prompts, each resource type directory also receives a dedicated AI
documentation prompt (doc-prompt.md).

You can specify resources in multiple ways:
  - By resource ID: --resource-id "/subscriptions/.../resourceGroups/my-rg"
  - By resource type: --type "Microsoft.Storage/storageAccounts" (repeatable; downloads all resources of the given type(s))
  - By resource group: --resource-group "my-rg" (downloads the resource group itself)

The --type flag acts as a filter and may be specified multiple times. If no
--type (and no --resource-id/--resource-group) is given, all registered
resource types are downloaded.

The subscription ID is optional. If not specified, the default subscription from your 'az login' session will be used.

Authentication reuses your 'az login' session by default. To download Microsoft
Graph/Intune types that need scopes the Azure CLI app cannot provide, sign in to
a dedicated app registration with --client-id/--tenant-id (device-code flow).

Examples:
  # Download a specific resource by ID
  azure-rd download --resource-id "/subscriptions/.../resourceGroups/my-rg"

  # Download one or more resource types (--type is repeatable)
  azure-rd download --type "Microsoft.Storage/storageAccounts" --type "Microsoft.Compute/virtualMachines"

  # Download every registered resource type (no --type filter)
  azure-rd download

  # Download all resources in a resource group with an explicit subscription
  azure-rd download --subscription "sub-id" --resource-group "my-rg"

  # Preview without writing files
  azure-rd download --type "Microsoft.Compute/virtualMachines" --dry-run

  # Resolve masked Intune OMA-URI secrets to plaintext (writes secrets to disk)
  azure-rd download --type "Microsoft.Graph/deviceConfigurations" --resolve-secrets

  # Sign in to a dedicated app registration (device-code) for Graph/Intune scopes
  azure-rd download --client-id "<app-id>" --tenant-id "<tenant-id>"`,
	RunE: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	// Download-specific flags
	downloadCmd.Flags().StringSliceVar(&flagResourceIDs, "resource-id", []string{}, "explicit Azure resource ID to download; repeatable")
	downloadCmd.Flags().IntVar(&flagTimeout, "timeout", 300, "per-operation timeout in seconds")
	downloadCmd.Flags().BoolVar(&flagResolveSecrets, "resolve-secrets", false, "resolve masked Intune OMA-URI secrets to plaintext (writes secrets to disk)")
	downloadCmd.Flags().BoolVar(&flagWritePrompts, "write-prompts", false, "write a per-type documentation LLM prompt file (<type>.prompt.md) into each resource type directory")

	// Bind config-backed flags to viper so they can also be set via the config
	// file or AZURE_RD_* env vars (precedence: flag > env > config > default).
	_ = viper.BindPFlag("timeout", downloadCmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("resolve-secrets", downloadCmd.Flags().Lookup("resolve-secrets"))
	_ = viper.BindPFlag("write-prompts", downloadCmd.Flags().Lookup("write-prompts"))
}

func runDownload(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Get configuration
	sub := viper.GetString("subscription")
	output := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	workersFlag := viper.GetInt("workers")

	// Selection/tuning options are config-backed (flag > env > config > default).
	selectedTypes := viper.GetStringSlice("type")
	resourceGroup := viper.GetString("resource-group")
	timeout := viper.GetInt("timeout")

	// Build worker configuration
	workerConfig := buildWorkerConfig()

	log := logger.Default

	// Build transformer configurations
	transformerConfigs := buildTransformerConfigs()

	// Build per-resource-type property filters
	resourceFilters := buildResourceFilters()

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

	// Create Azure client (will auto-detect subscription if not provided).
	// Authentication uses the existing Azure CLI session (az login) by default,
	// or device-code sign-in against a dedicated app when --client-id is set.
	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub, viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		// Runtime error - print and exit without showing help
		log.Error("Failed to create Azure client", "error", err)
		os.Exit(1)
	}

	// Get the actual subscription ID being used (may have been auto-detected)
	sub = azureClient.GetSubscriptionID()
	log.Info("Authentication successful", "subscription", sub)

	// Scope the output under the tenant's default domain so downloads from
	// different tenants never collide. Resolution is best-effort: if it fails
	// (e.g. insufficient permissions), warn and keep the base output path.
	if tenantDomain, err := azureClient.GetTenantDomain(ctx); err != nil {
		log.Warn("Could not resolve tenant domain; output path will not include the tenant",
			"reason", azure.ErrorSummary(err))
		log.Debug("Tenant domain resolution failed", "error", err)
	} else {
		output = filepath.Join(output, tenantDomain)
		log.Info("Scoping output under tenant", "tenant", tenantDomain, "output", output)
	}

	// Create handler registry pre-populated with all supported resource types
	registry := handlers.NewRegistry(azureClient.GetCredential(), azureClient.GetSubscriptionID(), viper.GetBool("resolve-secrets"))

	log.Info("Registered resource type handlers", "count", len(registry.GetAllTypes()))

	// Bound the concurrency of the per-type listing calls. Most listed types
	// are Microsoft Graph collections, so use the Graph worker count (which
	// respects its stricter rate limits) as the listing concurrency.
	listConcurrency := workerConfig.MicrosoftGraph
	if listConcurrency < 1 {
		listConcurrency = workerConfig.Default
	}

	// Build fetch requests
	requests, skippedTypes, emptyTypes, err := registry.BuildFetchRequests(ctx, flagResourceIDs, resourceGroup, selectedTypes, sub, listConcurrency)
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
	if len(selectedTypes) == 1 {
		effectiveType = selectedTypes[0]
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
		ResourceFilters:    resourceFilters,
		WritePrompts:       viper.GetBool("write-prompts"),
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

	// Print summary, including resource types that could not be listed and
	// types that returned no resources
	summary.SkippedTypes = skippedTypes
	summary.EmptyTypes = emptyTypes
	summary.PrintSummary()

	if summary.FailedResources > 0 {
		// Runtime error - print and exit without showing help
		log.Error("Pipeline completed with errors", "failed", summary.FailedResources)
		os.Exit(1)
	}

	log.Info("Download completed successfully")
	return nil
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

// buildResourceFilters constructs per-resource-type property filters from the
// "filters" config key (resourceType -> {property -> regex}). A resource is
// kept only when every property regex for its type matches. Invalid entries are
// logged and skipped so the run proceeds with the valid filters.
func buildResourceFilters() []models.ResourceFilter {
	log := logger.Default

	if !viper.IsSet("filters") {
		return nil
	}

	raw, ok := viper.Get("filters").(map[string]interface{})
	if !ok {
		log.Warn("Ignoring 'filters' config: expected a map of resource type to property filters",
			"type", fmt.Sprintf("%T", viper.Get("filters")))
		return nil
	}

	filters, err := models.ParseResourceFilters(raw)
	if err != nil {
		log.Warn("Some resource filters were skipped", "error", err)
	}

	for _, f := range filters {
		matchers := make([]string, len(f.Properties))
		for i, p := range f.Properties {
			matchers[i] = fmt.Sprintf("%s=~%s", p.Property, p.Pattern.String())
		}
		log.Info("Resource filter active", "type", f.ResourceType, "match", matchers)
	}

	return filters
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
