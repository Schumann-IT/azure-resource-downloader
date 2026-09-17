// Package resource implements the `azure-rd resource ...` subcommands: the
// commands that act on a tenant's Azure resources. The parent command lives in
// package cmd, which attaches each subcommand through the constructors exported
// here; this package must not import package cmd (that would be an import
// cycle), so anything shared with the root command comes from internal/.
package resource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"
	"azure-resource-downloader/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewDownloadCommand builds the `resource download` command. The
// authentication and selection flags and --workers come from the `resource`
// parent; only the download-only switches are declared here.
func NewDownloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download Azure resources",
		Long: `Download Azure resources and transform them into YAML format. By
default each resource type directory also receives a dedicated AI documentation
prompt (doc-prompt.md); pass --no-prompt to skip writing them.

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
  azure-rd resource download --resource-id "/subscriptions/.../resourceGroups/my-rg"

  # Download one or more resource types (--type is repeatable)
  azure-rd resource download --type "Microsoft.Storage/storageAccounts" --type "Microsoft.Compute/virtualMachines"

  # Download every registered resource type (no --type filter)
  azure-rd resource download

  # Download all resources in a resource group with an explicit subscription
  azure-rd resource download --subscription "sub-id" --resource-group "my-rg"

  # Preview without writing files
  azure-rd resource download --type "Microsoft.Compute/virtualMachines" --dry-run

  # Resolve masked Intune OMA-URI secrets to plaintext (writes secrets to disk)
  azure-rd resource download --type "Microsoft.Graph/deviceConfigurations" --resolve-secrets

  # Sign in to a dedicated app registration (device-code) for Graph/Intune scopes
  azure-rd resource download --client-id "<app-id>" --tenant-id "<tenant-id>"

  # Load settings from a config file (see config.example.yaml; flags still win)
  azure-rd resource download --config ~/.azure-rd.yaml

  # Also delete resources that are no longer in the tenant (complete runs only)
  azure-rd resource download --prune

  # List what --prune would delete, without deleting anything
  azure-rd resource download --prune --dry-run`,
		RunE: runDownload,
	}

	// The authentication and selection flags and --workers are inherited from
	// the `resource` parent; --timeout stays here because only download fetches
	// individual resources, so a sibling advertising it would ignore it. Flags
	// are bound to viper per-execution in runDownload via cmdutil.BindFlags.
	cmdutil.AddTimeoutFlag(cmd)

	cmd.Flags().Bool("resolve-secrets", false, "resolve masked Intune OMA-URI secrets to plaintext (writes secrets to disk)")
	cmd.Flags().Bool("no-prompt", false, "skip writing the per-type documentation LLM prompt files (doc-prompt.md); prompts are written by default")
	cmd.Flags().Bool("prune", false, "delete files under resources/ for resources this run establishes are no longer in the tenant (requires a complete run)")

	return cmd
}

func runDownload(cmd *cobra.Command, args []string) error {
	// Bind the flags that apply to this command (its own and those inherited
	// from the resource group and root) to viper before reading any values, so
	// the flag > env > config > default precedence holds without a sibling
	// command stealing the binding.
	cmdutil.BindFlags(cmd)

	// The command context is cancelled on interrupt (Ctrl+C), so listing and
	// fetching stop cleanly: every request still produces a result and the run
	// is recorded as incomplete rather than killed mid-write.
	ctx := cmd.Context()

	// Get configuration
	sub := viper.GetString("subscription")
	output := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	workersFlag := viper.GetInt("workers")
	workersExplicit := cmd.Flags().Changed("workers")
	resourceIDs := viper.GetStringSlice("resource-id")

	// Selection/tuning options are config-backed (flag > env > config > default).
	selectedTypes := viper.GetStringSlice("type")
	resourceGroup := viper.GetString("resource-group")
	timeout := viper.GetInt("timeout")

	// Documentation prompts are written by default; --no-prompt (or no-prompt in
	// config) opts out.
	writePrompts := !viper.GetBool("no-prompt")

	// Secret resolution changes the delegated Graph permission the Intune device
	// configuration type needs (ReadWrite.All instead of Read.All), so read it
	// once and thread it through both the permission probe and the real
	// registry.
	resolveSecrets := viper.GetBool("resolve-secrets")

	// Build worker configuration
	workerConfig := BuildWorkerConfig(workersExplicit)

	log := logger.Default

	// Build transformer configurations
	transformerConfigs := BuildTransformerConfigs()

	// Build per-resource-type property filters
	resourceFilters := BuildResourceFilters()

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

	log.Info("azure-rd",
		"subscription", func() string {
			if sub == "" {
				return "<default>"
			}
			return sub
		}(),
		"output", output,
		"workers", workersFlag,
		"dry_run", dryRun)

	clientID := viper.GetString("client-id")
	tenantID := viper.GetString("tenant-id")

	// Before authenticating, determine whether any selected resource type needs
	// a dedicated app registration (Microsoft Graph scopes the Azure CLI app
	// cannot provide). Building the probe registry is a local operation (no
	// network) and only reads static per-type metadata, so a plain Azure CLI
	// credential is enough here regardless of the final sign-in method.
	probeCred, err := azure.NewCredential("", "")
	if err != nil {
		return fmt.Errorf("failed to prepare Azure credentials: %w", err)
	}

	// With no --client-id, this run leans on the Azure CLI session — for the
	// token itself, or at least for the tenant default of the dedicated-app
	// prompt below — so prove that session exists FIRST. Every later step
	// degrades deliberately when it fails (subscription and tenant resolution
	// warn and continue for tenant-only identities; unlistable types are
	// skipped per type), so without this check a missing 'az login' compounds
	// into warnings ending in "No resources to download", and the operator is
	// asked for an app registration before ever learning they are not signed
	// in. Explicit --client-id/--tenant-id (device-code) is exempt: that
	// sign-in happens at the first token request and needs no CLI session.
	if clientID == "" {
		if err := azure.VerifySession(ctx, probeCred); err != nil {
			log.Debug("Session verification failed", "error", err)
			return fmt.Errorf("not signed in to Azure; run 'az login' first or pass --client-id/--tenant-id for device-code sign-in (%s)",
				azure.ErrorSummary(err))
		}
	}

	probeRegistry := handlers.NewRegistry(probeCred, sub, resolveSecrets)
	requirements := probeRegistry.DedicatedAppRequirements(
		selectedTypeNames(probeRegistry, selectedTypes, resourceGroup, resourceIDs))

	// If such a type is targeted but the client ID or tenant ID is missing,
	// request them interactively rather than failing later with permission
	// errors. The client ID default comes from --client-id/AZURE_RD_CLIENT_ID
	// (config), and the tenant ID defaults to the current Azure CLI session's
	// tenant so the user can usually just press Enter.
	if len(requirements) > 0 && (clientID == "" || tenantID == "") {
		defaultTenantID := tenantID
		if defaultTenantID == "" {
			defaultTenantID = azure.CLIDefaultTenantID(ctx)
		}
		clientID, tenantID, err = cmdutil.PromptForDedicatedApp(requirements, os.Stdin, clientID, defaultTenantID)
		if err != nil {
			return fmt.Errorf("cannot download the selected resource types without a dedicated app registration: %w", err)
		}
	}

	// Create Azure client (will auto-detect subscription if not provided).
	// Authentication uses the existing Azure CLI session (az login) by default,
	// or device-code sign-in against a dedicated app when --client-id is set.
	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, sub, clientID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to create Azure client: %w", err)
	}

	// Get the actual subscription ID being used (may have been auto-detected)
	sub = azureClient.GetSubscriptionID()
	log.Info("Authentication successful", "subscription", sub)

	// Scope the output under the tenant's default domain so downloads from
	// different tenants never collide. Resolution is best-effort: if it fails
	// (e.g. insufficient permissions), warn and keep the base output path.
	tenant := ""
	if tenantDomain, err := azureClient.GetTenantDomain(ctx); err != nil {
		log.Warn("Could not resolve tenant domain; output path will not include the tenant",
			"reason", azure.ErrorSummary(err))
		log.Debug("Tenant domain resolution failed", "error", err)
	} else {
		tenant = tenantDomain
		output = filepath.Join(output, tenantDomain)
		log.Info("Scoping output under tenant", "tenant", tenantDomain, "output", output)
	}

	// Create handler registry pre-populated with all supported resource types
	registry := handlers.NewRegistry(azureClient.GetCredential(), azureClient.GetSubscriptionID(), resolveSecrets)

	log.Info("Registered resource type handlers", "count", len(registry.GetAllTypes()))

	// Bound the concurrency of the per-type listing calls.
	listConcurrency := listingConcurrency(workerConfig, workersFlag, workersExplicit)

	// Build fetch requests
	requests, skippedTypes, emptyTypes, err := registry.BuildFetchRequests(ctx, resourceIDs, resourceGroup, selectedTypes, sub, listConcurrency)
	if err != nil {
		return fmt.Errorf("failed to build fetch requests: %w", err)
	}

	if len(requests) == 0 {
		return errors.New("no resources to download")
	}

	log.Info("Preparing to download resources", "count", len(requests))

	// Worker tuning is API-specific and only meaningful when a single type is
	// targeted. With multiple types (or all registered types), treat as mixed.
	effectiveType := ""
	if len(selectedTypes) == 1 {
		effectiveType = selectedTypes[0]
	}

	// Determine worker count based on resource type and API
	workers := determineWorkerCount(workerConfig, effectiveType, workersFlag, workersExplicit)

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
		WritePrompts:       writePrompts,
	}

	// A dry run answers "what would be downloaded" offline: the per-type
	// listing that built the request set has already run (upstream of the
	// fetcher), so stop here rather than fetching, transforming and discarding
	// every resource. This keeps the dry run cheap and consistent with
	// 'docs generate-prompt --dry-run', and its output is a subset of a real
	// run, never a preview of the files a real run would write.
	var summary *pipeline.ExecutionSummary
	if dryRun {
		log.Info("Dry-run: listing resources that would be downloaded (no fetch, transform or write)")
		summary = pipeline.DryRunSummary(requests)
	} else {
		p := pipeline.NewPipeline(azureClient, registry, pipelineConfig)
		log.Info("Starting pipeline execution...")
		summary, err = p.Execute(ctx, requests)
		if err != nil {
			return fmt.Errorf("pipeline execution failed: %w", err)
		}
	}

	// Attach the resource types that could not be listed and the types that
	// returned no resources, then derive completeness (a failed listing makes
	// the run incomplete) before printing the summary.
	summary.SkippedTypes = skippedTypes
	summary.EmptyTypes = emptyTypes
	summary.MarkCompleteness()
	summary.PrintSummary()

	// Record the export metadata (resources/metadata.yaml) and, when requested,
	// prune resources this run proved are gone from the tenant. This runs before
	// the failure exit below so the metadata always reflects what happened. A
	// metadata failure only warns: the downloaded YAML is the valuable output.
	exportRun := docs.ExportRun{
		Output:                 output,
		Tenant:                 tenant,
		ToolVersion:            version.Tool(),
		GeneratedAt:            time.Now(),
		Scope:                  docs.RunScope{Types: selectedTypes, ResourceIDs: resourceIDs, ResourceGroup: resourceGroup},
		TransformConfigSha256:  docs.HashTransformConfig(transformerConfigs, resolveSecrets),
		ResolveSecrets:         resolveSecrets,
		WritePrompts:           writePrompts,
		DryRun:                 dryRun,
		Prune:                  viper.GetBool("prune"),
		AssignmentCapableTypes: registry.AssignmentCapableTypes(),
		Summary:                summary,
	}
	if err := docs.WriteExportMetadata(exportRun); err != nil {
		log.Warn("Export metadata not written", "error", err)
	}

	if summary.FailedResources > 0 {
		return fmt.Errorf("pipeline completed with errors (%d resources failed)", summary.FailedResources)
	}

	log.Info("Download completed successfully")
	return nil
}

// selectedTypeNames resolves the resource types a run targets, mirroring the
// selection precedence in Registry.BuildFetchRequests, so the permission probe
// examines exactly the types that will be downloaded:
//   - explicit --resource-id: the type parsed from each ID (unparseable IDs,
//     e.g. bare Microsoft Graph GUIDs, are skipped);
//   - --resource-group: the resource group type (ARM, no dedicated app);
//   - --type: the listed types;
//   - none of the above: every registered type (a full export).
func selectedTypeNames(registry *handlers.Registry, selectedTypes []string, resourceGroup string, resourceIDs []string) []string {
	switch {
	case len(resourceIDs) > 0:
		var types []string
		seen := make(map[string]bool)
		for _, id := range resourceIDs {
			info, err := azure.ParseResourceID(id)
			if err != nil || info.FullType == "" || seen[info.FullType] {
				continue
			}
			seen[info.FullType] = true
			types = append(types, info.FullType)
		}
		return types
	case resourceGroup != "":
		return []string{"Microsoft.Resources/resourceGroups"}
	case len(selectedTypes) > 0:
		return selectedTypes
	default:
		return registry.GetAllTypes()
	}
}

// BuildWorkerConfig constructs worker configuration from config file.
// workersExplicit reports whether --workers was set on the command line
// (cmd.Flags().Changed). It is exported so the config.example.yaml no-op guard
// in package cmd can assert that loading the example produces the built-in
// defaults.
func BuildWorkerConfig(workersExplicit bool) *models.WorkerConfig {
	config := models.DefaultWorkerConfig()

	// Apply the general workers setting only when it was actually provided.
	// viper.IsSet cannot decide that: once flags are bound it is always true (the
	// flag default answers), which would copy the flag default over the general
	// default unconditionally. An explicit flag always counts; otherwise a value
	// differing from the flag default must come from env or config. A config
	// value equal to the flag default is indistinguishable from no setting, and
	// applying it would change nothing.
	if generalWorkers := viper.GetInt("workers"); generalWorkers > 0 &&
		(workersExplicit || generalWorkers != cmdutil.DefaultWorkerCount) {
		config.Default = generalWorkers
		// Don't override API-specific defaults yet - those come from workers-by-api
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

// listingConcurrency returns the bounded concurrency for the per-type listing
// calls, shared by every resource subcommand that lists (download, list,
// types) so a --workers override means the same thing in all of them. An
// explicit --workers wins; otherwise the Microsoft Graph worker count applies,
// because most listed types are Graph collections and its rate limits are the
// stricter ones.
func listingConcurrency(workerConfig *models.WorkerConfig, workersFlag int, workersExplicit bool) int {
	if workersExplicit && workersFlag > 0 {
		return workersFlag
	}
	if workerConfig.MicrosoftGraph > 0 {
		return workerConfig.MicrosoftGraph
	}
	return workerConfig.Default
}

// determineWorkerCount determines the worker count based on resource type.
// Explicitness is passed in (cmd.Flags().Changed("workers")) rather than sniffed
// from the value, so an explicit --workers 5 is honoured for API types and the
// default literal is never duplicated here.
func determineWorkerCount(workerConfig *models.WorkerConfig, resourceType string, workersFlag int, workersExplicit bool) int {
	// Priority 1: an explicitly set --workers flag wins for every API.
	if workersExplicit {
		return workersFlag
	}

	// Priority 2: Use API-specific worker count based on resource type
	if resourceType != "" {
		return workerConfig.GetWorkerCount(resourceType)
	}

	// Priority 3: For mixed resource types, use safe default
	return workerConfig.Default
}

// BuildResourceFilters constructs per-resource-type property filters from the
// "filters" config key (resourceType -> {property -> regex}). A resource is
// kept only when every property regex for its type matches. Invalid entries are
// logged and skipped so the run proceeds with the valid filters. It is exported
// so the config.example.yaml no-op guard in package cmd can assert that the
// example defines no filters.
func BuildResourceFilters() []models.ResourceFilter {
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

// BuildTransformerConfigs constructs transformer configurations from viper. It
// is exported so the config.example.yaml no-op guard in package cmd can assert
// that loading the example yields the default transformer set — and therefore
// the same transformConfigSha256 in resources/metadata.yaml.
func BuildTransformerConfigs() []models.TransformerConfig {
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
