// Package runprep prepares a resource-facing run: it reads the configuration,
// builds the worker/transformer/filter constructions, verifies the session,
// runs the dedicated-app probe and prompt, authenticates, resolves the tenant
// and the export directory, and builds the handler registry and the fetch
// requests. It exists so the commands that do the same list, fetch and
// transform work (resource download and resource drift) share one preparation
// and are incapable of diverging in authentication or selection semantics.
// Flag groups and interactive prompts stay in internal/cmdutil.
package runprep

import (
	"context"
	"fmt"
	"os"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/tenantdir"

	"github.com/spf13/viper"
)

// Options carries the per-command inputs Prepare cannot read from the
// configuration.
type Options struct {
	// Domain is the tenant domain explicitly passed as --domain, asserting
	// which tenant the run may act on. Empty means "whichever tenant the
	// session resolves to".
	Domain string
}

// Prepared bundles everything a resource-fetching command needs after
// preparation: the effective configuration, the authenticated client, the
// per-tenant output directory and the pre-populated handler registry.
type Prepared struct {
	// Client is the authenticated Azure client (CLI session or device code).
	Client *azure.Client
	// Registry is pre-populated with every supported resource type handler.
	Registry *handlers.Registry
	// Subscription is the effective subscription id ("" when none available).
	Subscription string
	// BaseOutput is the --output directory as configured.
	BaseOutput string
	// Output is the per-tenant output directory (BaseOutput/<tenant>). A run
	// that cannot name its tenant is refused rather than written elsewhere.
	Output string
	// Tenant is the tenant's Entra default domain, and the name of Output's
	// last path element.
	Tenant string
	// TenantUnverified reports that Tenant was taken from --domain without a
	// session confirming it, so the run must say so rather than imply the
	// tenant agreed.
	TenantUnverified bool
	// DryRun mirrors the global --dry-run switch.
	DryRun bool
	// Timeout is the per-operation timeout in seconds.
	Timeout int
	// ResolveSecrets mirrors --resolve-secrets; it changes both the required
	// Graph permission and the bytes the transform produces.
	ResolveSecrets bool
	// ResourceIDs, SelectedTypes and ResourceGroup are the selection.
	ResourceIDs   []string
	SelectedTypes []string
	ResourceGroup string
	// WorkersFlag and WorkersExplicit carry the --workers value and whether it
	// was set explicitly; WorkerConfig is the effective worker configuration.
	WorkersFlag     int
	WorkersExplicit bool
	WorkerConfig    *models.WorkerConfig
	// TransformerConfigs and ResourceFilters are the effective transform
	// configuration; both feed the pipeline and the recorded config hashes.
	TransformerConfigs []models.TransformerConfig
	ResourceFilters    []models.ResourceFilter
}

// Prepare performs the run preparation shared by the commands that fetch
// resources. It reads the effective configuration, verifies the Azure CLI
// session (unless the profile names a dedicated app), prompts for a dedicated
// app registration when a selected type needs one, authenticates, decides the
// per-tenant output directory, and builds the real handler registry.
func Prepare(ctx context.Context, opts Options) (*Prepared, error) {
	log := logger.Default

	workers, workersExplicit := WorkersFromConfig()
	p := &Prepared{
		Subscription:    viper.GetString("subscription"),
		BaseOutput:      viper.GetString("output"),
		DryRun:          viper.GetBool("dry-run"),
		Timeout:         viper.GetInt("timeout"),
		ResolveSecrets:  viper.GetBool("resolve-secrets"),
		ResourceIDs:     viper.GetStringSlice("resource-id"),
		SelectedTypes:   viper.GetStringSlice("type"),
		ResourceGroup:   viper.GetString("resource-group"),
		WorkersFlag:     workers,
		WorkersExplicit: workersExplicit,
	}
	p.WorkerConfig = BuildWorkerConfig()
	p.TransformerConfigs = BuildTransformerConfigs()
	p.ResourceFilters = BuildResourceFilters()

	logTransformers(p.TransformerConfigs)

	if p.Subscription == "" {
		log.Info("No subscription specified, will use default from Azure CLI session")
	}

	clientID, tenantID, err := p.resolveCredentials(ctx)
	if err != nil {
		return nil, err
	}

	// Create the Azure client (auto-detects the subscription when not
	// provided). Authentication uses the existing Azure CLI session (az login)
	// by default, or device-code sign-in against the dedicated app the profile
	// names.
	log.Info("Authenticating with Azure...")
	azureClient, err := azure.NewClient(ctx, p.Subscription, clientID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}
	p.Client = azureClient
	p.Subscription = azureClient.GetSubscriptionID()
	log.Info("Authentication successful", "subscription", p.Subscription)

	if err := p.resolveTenantDir(ctx, opts.Domain); err != nil {
		return nil, err
	}

	// Create the handler registry pre-populated with all supported types.
	p.Registry = handlers.NewRegistry(azureClient.GetCredential(), p.Subscription, p.ResolveSecrets)
	log.Info("Registered resource type handlers", "count", len(p.Registry.GetAllTypes()))

	return p, nil
}

// resolveCredentials decides which identity this run signs in with: the Azure
// CLI session, or the dedicated app registration the tenant's profile names. It
// proves the CLI session exists before anything depends on it, and asks for an
// app registration when a selected resource type cannot be read without one.
func (p *Prepared) resolveCredentials(ctx context.Context) (clientID, tenantID string, err error) {
	log := logger.Default

	clientID = viper.GetString("client-id")
	tenantID = viper.GetString("tenant-id")

	// Before authenticating, determine whether any selected resource type needs
	// a dedicated app registration (Microsoft Graph scopes the Azure CLI app
	// cannot provide). Building the probe registry is a local operation (no
	// network) and only reads static per-type metadata, so a plain Azure CLI
	// credential is enough here regardless of the final sign-in method.
	probeCred, err := azure.NewCredential("", "")
	if err != nil {
		return "", "", fmt.Errorf("failed to prepare Azure credentials: %w", err)
	}

	// Without a configured client-id, this run leans on the Azure CLI session —
	// for the token itself, or at least for the tenant default of the
	// dedicated-app prompt below — so prove that session exists FIRST. Every
	// later step degrades deliberately when it fails (subscription resolution
	// warns and continues for tenant-only identities; unlistable types are
	// skipped per type), so without this check a missing 'az login' compounds
	// into warnings, and the operator is asked for an app registration before
	// ever learning they are not signed in. A configured client-id is exempt:
	// that device-code sign-in happens at the first token request and needs no
	// CLI session.
	if clientID == "" {
		if err := azure.VerifySession(ctx, probeCred); err != nil {
			log.Debug("Session verification failed", "error", err)
			return "", "", fmt.Errorf("not signed in to Azure; run 'az login' first, or set client-id and tenant-id in the tenant's configuration profile for device-code sign-in (%s)",
				azure.ErrorSummary(err))
		}
	}

	probeRegistry := handlers.NewRegistry(probeCred, p.Subscription, p.ResolveSecrets)
	requirements := probeRegistry.DedicatedAppRequirements(
		SelectedTypeNames(probeRegistry, p.SelectedTypes, p.ResourceGroup, p.ResourceIDs))
	if len(requirements) == 0 || (clientID != "" && tenantID != "") {
		return clientID, tenantID, nil
	}

	// A selected type needs a dedicated app but the profile does not name a
	// complete one: request it interactively rather than failing later with
	// permission errors. Both defaults come from the profile, and the tenant ID
	// falls back to the current Azure CLI session's tenant so the user can
	// usually just press Enter. The prompt prints the profile snippet to save,
	// so the answer lands in the single source of truth.
	defaultTenantID := tenantID
	if defaultTenantID == "" {
		defaultTenantID = azure.CLIDefaultTenantID(ctx)
	}
	clientID, tenantID, err = cmdutil.PromptForDedicatedApp(requirements, os.Stdin, clientID, defaultTenantID)
	if err != nil {
		return "", "", fmt.Errorf("cannot proceed with the selected resource types without a dedicated app registration: %w", err)
	}
	return clientID, tenantID, nil
}

// resolveTenantDir decides which tenant directory this run acts on, through the
// resolver every command shares: the declared --domain is intent, the signed-in
// tenant is ground truth, and a disagreement refuses before anything is written.
//
// There is deliberately no fallback to the bare output directory — an export
// written there would not match the <output>/<domain>/ layout every other
// command looks for, so it would be invisible from the moment it was written.
func (p *Prepared) resolveTenantDir(ctx context.Context, declaredDomain string) error {
	log := logger.Default

	resolvedDomain, err := p.Client.GetTenantDomain(ctx)
	if err != nil {
		log.Warn("Could not resolve the tenant domain from the session", "reason", azure.ErrorSummary(err))
		log.Debug("Tenant domain resolution failed", "error", err)
	}

	target, err := tenantdir.Resolve(p.BaseOutput, declaredDomain, resolvedDomain)
	if err != nil {
		return err
	}
	p.Tenant = target.Domain
	p.Output = target.Dir
	p.TenantUnverified = target.Unverified()
	if p.TenantUnverified {
		log.Warn("The tenant domain is taken from --domain and was NOT confirmed by the session", "tenant", p.Tenant)
	}
	log.Info("Scoping output under tenant", "tenant", p.Tenant, "output", p.Output)
	return nil
}

// BuildRequests expands the prepared selection into individual fetch requests
// through the registry's shared listing path, bounding the per-type listing
// concurrency the same way for every caller.
func (p *Prepared) BuildRequests(ctx context.Context) ([]*models.FetchRequest, []models.SkippedType, []string, error) {
	listConcurrency := ListingConcurrency(p.WorkerConfig, p.WorkersFlag, p.WorkersExplicit)
	return p.Registry.BuildFetchRequests(ctx, p.ResourceIDs, p.ResourceGroup, p.SelectedTypes, p.Subscription, listConcurrency)
}

// EffectiveType returns the single selected resource type when exactly one is
// targeted, or "" for a mixed run. Worker tuning is API-specific and only
// meaningful for a single type.
func (p *Prepared) EffectiveType() string {
	if len(p.SelectedTypes) == 1 {
		return p.SelectedTypes[0]
	}
	return ""
}

// FetchWorkerCount returns the effective per-resource fetch concurrency for
// this run, honouring an explicit --workers and the API-specific defaults.
func (p *Prepared) FetchWorkerCount() int {
	return DetermineWorkerCount(p.WorkerConfig, p.EffectiveType(), p.WorkersFlag, p.WorkersExplicit)
}

// LogWorkerConfiguration reports the effective fetch concurrency and warns
// when it exceeds the targeted API's recommendation, which can slow a run down
// through rate limiting and exponential backoff.
func (p *Prepared) LogWorkerConfiguration(workers int) {
	log := logger.Default
	effectiveType := p.EffectiveType()

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

	if effectiveType == "" {
		return
	}
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

// logTransformers reports the active transformer set once per run.
func logTransformers(transformerConfigs []models.TransformerConfig) {
	log := logger.Default

	if len(transformerConfigs) == 0 {
		log.Info("No transformers enabled - raw Azure data will be output")
		return
	}

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

// SelectedTypeNames resolves the resource types a run targets, mirroring the
// selection precedence in Registry.BuildFetchRequests, so the permission probe
// examines exactly the types that will be fetched:
//   - explicit --resource-id: the type parsed from each ID (unparseable IDs,
//     e.g. bare Microsoft Graph GUIDs, are skipped);
//   - --resource-group: the resource group type (ARM, no dedicated app);
//   - --type: the listed types;
//   - none of the above: every registered type (a full run).
func SelectedTypeNames(registry *handlers.Registry, selectedTypes []string, resourceGroup string, resourceIDs []string) []string {
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

// WorkersFromConfig returns the configured worker count and whether it was set
// at all. Presence is the signal: the per-API defaults (Microsoft Graph 5, ARM
// 20) apply unless the operator chose a single count deliberately. This is
// decidable with viper.IsSet only because "workers" has neither a flag nor a
// registered default — either would make IsSet always true and silently flatten
// both API counts to one number.
func WorkersFromConfig() (count int, explicit bool) {
	return viper.GetInt("workers"), viper.IsSet("workers")
}

// BuildWorkerConfig constructs worker configuration from the config file. It is
// exported so the config.example.yaml no-op guard in package cmd can assert
// that loading the example produces the built-in defaults.
func BuildWorkerConfig() *models.WorkerConfig {
	config := models.DefaultWorkerConfig()

	// Apply the general worker count only when the configuration actually sets
	// it; otherwise the API-specific defaults stand. They are overridden below
	// by workers-by-api only, never by this general value.
	if generalWorkers, explicit := WorkersFromConfig(); explicit && generalWorkers > 0 {
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

// ListingConcurrency returns the bounded concurrency for the per-type listing
// calls, shared by every resource subcommand that lists (download, drift,
// list, types) so a configured worker count means the same thing in all of
// them. An explicitly configured count wins; otherwise the Microsoft Graph
// worker count applies, because most listed types are Graph collections and its
// rate limits are the stricter ones.
func ListingConcurrency(workerConfig *models.WorkerConfig, workersFlag int, workersExplicit bool) int {
	if workersExplicit && workersFlag > 0 {
		return workersFlag
	}
	if workerConfig.MicrosoftGraph > 0 {
		return workerConfig.MicrosoftGraph
	}
	return workerConfig.Default
}

// DetermineWorkerCount determines the worker count based on resource type.
// Explicitness is passed in rather than sniffed from the value, so a configured
// count that happens to equal the default is still honoured for API types and
// the default literal is never duplicated here.
func DetermineWorkerCount(workerConfig *models.WorkerConfig, resourceType string, workersFlag int, workersExplicit bool) int {
	// Priority 1: an explicitly configured count wins for every API.
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
