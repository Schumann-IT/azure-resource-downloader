// Package resource implements the `azure-rd resource ...` subcommands: the
// commands that act on a tenant's Azure resources. The parent command lives in
// package cmd, which attaches each subcommand through the constructors exported
// here; this package must not import package cmd (that would be an import
// cycle), so anything shared with the root command comes from internal/.
package resource

import (
	"errors"
	"fmt"
	"time"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"
	"azure-resource-downloader/internal/runprep"
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

	log := logger.Default

	// Documentation prompts are written by default; --no-prompt (or no-prompt in
	// config) opts out. This is a download-only switch, so it is read here, not
	// in the shared preparation.
	writePrompts := !viper.GetBool("no-prompt")

	// The preparation shared with `resource drift`: configuration, session
	// verification, the dedicated-app probe and prompt, authentication, tenant
	// and output resolution, and the handler registry.
	prep, err := runprep.Prepare(ctx, runprep.Options{WorkersExplicit: cmd.Flags().Changed("workers")})
	if err != nil {
		return err
	}

	log.Info("azure-rd",
		"subscription", func() string {
			if prep.Subscription == "" {
				return "<default>"
			}
			return prep.Subscription
		}(),
		"output", prep.Output,
		"workers", prep.WorkersFlag,
		"dry_run", prep.DryRun)

	// Build fetch requests through the shared listing path.
	requests, skippedTypes, emptyTypes, err := prep.BuildRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to build fetch requests: %w", err)
	}

	if len(requests) == 0 {
		return errors.New("no resources to download")
	}

	log.Info("Preparing to download resources", "count", len(requests))

	// Worker tuning is API-specific and only meaningful when a single type is
	// targeted. With multiple types (or all registered types), treat as mixed.
	workers := prep.FetchWorkerCount()
	prep.LogWorkerConfiguration(workers)

	// Create and configure pipeline
	pipelineConfig := &models.PipelineConfig{
		OutputDir:          prep.Output,
		WorkerCount:        workers,
		Timeout:            time.Duration(prep.Timeout) * time.Second,
		DryRun:             prep.DryRun,
		SubscriptionID:     prep.Subscription,
		TransformerConfigs: prep.TransformerConfigs,
		ResourceFilters:    prep.ResourceFilters,
		WritePrompts:       writePrompts,
	}

	// A dry run answers "what would be downloaded" offline: the per-type
	// listing that built the request set has already run (upstream of the
	// fetcher), so stop here rather than fetching, transforming and discarding
	// every resource. This keeps the dry run cheap and consistent with
	// 'docs generate-prompt --dry-run', and its output is a subset of a real
	// run, never a preview of the files a real run would write.
	var summary *pipeline.ExecutionSummary
	if prep.DryRun {
		log.Info("Dry-run: listing resources that would be downloaded (no fetch, transform or write)")
		summary = pipeline.DryRunSummary(requests)
	} else {
		p := pipeline.NewPipeline(prep.Client, prep.Registry, pipelineConfig)
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
		Output:                 prep.Output,
		Tenant:                 prep.Tenant,
		ToolVersion:            version.Tool(),
		GeneratedAt:            time.Now(),
		Scope:                  docs.RunScope{Types: prep.SelectedTypes, ResourceIDs: prep.ResourceIDs, ResourceGroup: prep.ResourceGroup},
		TransformConfigSha256:  docs.HashTransformConfig(prep.TransformerConfigs, prep.ResolveSecrets),
		FiltersSha256:          docs.HashResourceFilters(prep.ResourceFilters),
		ResolveSecrets:         prep.ResolveSecrets,
		WritePrompts:           writePrompts,
		DryRun:                 prep.DryRun,
		Prune:                  viper.GetBool("prune"),
		AssignmentCapableTypes: prep.Registry.AssignmentCapableTypes(),
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
