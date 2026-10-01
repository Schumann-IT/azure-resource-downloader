// Package resource implements the `azure-rd resource ...` subcommands: the
// commands that act on a tenant's Azure resources. The parent command lives in
// package cmd, which attaches each subcommand through the constructors exported
// here; this package must not import package cmd (that would be an import
// cycle), so anything shared with the root command comes from internal/.
package resource

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"
	"azure-resource-downloader/internal/runprep"
	"azure-resource-downloader/internal/version"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewDownloadCommand builds the `resource download` command. It declares no
// flags of its own: the selection flags and --domain come from the `resource`
// parent, and every other setting a download needs is configuration.
func NewDownloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download Azure resources",
		Long: `Download Azure resources and transform them into YAML format. By
default each resource type directory also receives a dedicated AI documentation
prompt (doc-prompt.md); set no-prompt in the configuration to skip writing them.

You can scope a single run in three ways:
  - By resource ID: --resource-id "/subscriptions/.../resourceGroups/my-rg"
  - By resource type: --type "Microsoft.Storage/storageAccounts" (repeatable)
  - By resource group: --resource-group "my-rg" (downloads the resource group itself)

--type narrows the types the configuration selects, for this run only; with no
selection at all, every registered resource type is downloaded.

Everything else is configuration, not a flag: the subscription and the
credentials come from the tenant's profile, and the worker counts, the
per-operation timeout, secret resolution, prompt writing and pruning come from
the base configuration file (see config.example.yaml and
config.example.domain.yaml). Requiring the destructive and secret-writing
switches to be written down is deliberate.

Authentication reuses your 'az login' session by default. To download Microsoft
Graph/Intune types that need scopes the Azure CLI app cannot provide, put a
dedicated app registration's client-id and tenant-id in the tenant's profile;
if a selected type needs one and the profile has none, the run asks for it and
prints the snippet to save.

Examples:
  # Download every registered resource type with the built-in defaults
  azure-rd resource download

  # A tenant selected by domain, with its profile from a config directory
  azure-rd resource download --config-dir ~/.azure-rd --domain contoso.onmicrosoft.com

  # Narrow one run to two types
  azure-rd resource download --type "Microsoft.Storage/storageAccounts" --type "Microsoft.Compute/virtualMachines"

  # A single resource by ID
  azure-rd resource download --resource-id "/subscriptions/.../resourceGroups/my-rg"

  # Preview without writing files
  azure-rd resource download --dry-run

  # Download elsewhere to compare, leaving the real export untouched
  azure-rd resource download --domain contoso.onmicrosoft.com --output ./compare

  # Load a base configuration file explicitly
  azure-rd resource download --config ~/.azure-rd/base.yaml`,
		RunE: runDownload,
	}

	// Nothing is declared here: the selection flags and --domain come from the
	// `resource` parent, and every switch this command used to carry (--timeout,
	// --resolve-secrets, --no-prompt, --prune) is now a configuration setting, so
	// there is exactly one place to look for each of them.
	return cmd
}

func runDownload(cmd *cobra.Command, args []string) error {
	// The command context is cancelled on interrupt (Ctrl+C), so listing and
	// fetching stop cleanly: every request still produces a result and the run
	// is recorded as incomplete rather than killed mid-write.
	ctx := cmd.Context()

	log := logger.Default

	// Documentation prompts are written by default; the no-prompt setting opts
	// out. This is a download-only setting, so it is read here rather than in
	// the shared preparation.
	writePrompts := !viper.GetBool("no-prompt")

	// The preparation shared with `resource drift`: configuration, session
	// verification, the dedicated-app probe and prompt, authentication, tenant
	// and output resolution, and the handler registry.
	prep, err := runprep.Prepare(ctx, runprep.Options{Domain: cmdutil.DeclaredDomain(cmd)})
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

	if nothingListed(requests, emptyTypes) {
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

	summary, err := executeDownload(ctx, prep, pipelineConfig, requests)
	if err != nil {
		return err
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
		Scope:                  docs.RunScope{Types: prep.SelectedTypes, ResourceIDs: prep.ResourceIDs, ResourceGroup: prep.ResourceGroup, ExcludedTypes: prep.ExcludedTypes},
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
	} else {
		// The metadata write moved the baseline's generatedAt (partial runs
		// included), which supersedes any drift observation by definition —
		// drift is always relative to the baseline. Clear the drift/ tree so a
		// stale observation (and its analysis artifacts) cannot outlive the
		// baseline it was decided against.
		rebaselineClearDrift(prep.Output, prep.DryRun)
	}

	if summary.FailedResources > 0 {
		return fmt.Errorf("pipeline completed with errors (%d resources failed)", summary.FailedResources)
	}

	log.Info("Download completed successfully")
	return nil
}

// nothingListed reports whether a download has nothing to act on: the listing
// produced no fetch request and no type listed as empty. That happens when
// every type in scope could not be listed, or a --resource-group run had no
// subscription. A run whose types merely listed empty is not "nothing": those
// types are covered, so the run must still record metadata (marking their
// earlier entries absent, and pruning them on a complete run with prune on).
func nothingListed(requests []*models.FetchRequest, emptyTypes []string) bool {
	return len(requests) == 0 && len(emptyTypes) == 0
}

// executeDownload produces the run's ExecutionSummary for the listed requests.
//
// A dry run answers "what would be downloaded" offline: the per-type listing
// that built the request set has already run (upstream of the fetcher), so it
// stops there rather than fetching, transforming and discarding every
// resource. This keeps the dry run cheap and consistent with
// 'docs generate-prompt --dry-run', and its output is a subset of a real run,
// never a preview of the files a real run would write.
//
// When only empty types were listed there is nothing to fetch, so the pipeline
// is skipped and the summary is empty — zero requests, zero results — which
// MarkCompleteness accounts exactly as a full run's.
func executeDownload(ctx context.Context, prep *runprep.Prepared, cfg *models.PipelineConfig, requests []*models.FetchRequest) (*pipeline.ExecutionSummary, error) {
	log := logger.Default
	if prep.DryRun {
		log.Info("Dry-run: listing resources that would be downloaded (no fetch, transform or write)")
		return pipeline.DryRunSummary(requests), nil
	}
	if len(requests) == 0 {
		log.Info("Every listed type is empty: nothing to fetch, recording the export metadata only")
		return &pipeline.ExecutionSummary{Results: []*models.WriteResult{}}, nil
	}
	p := pipeline.NewPipeline(prep.Client, prep.Registry, cfg)
	log.Info("Starting pipeline execution...")
	summary, err := p.Execute(ctx, requests)
	if err != nil {
		return nil, fmt.Errorf("pipeline execution failed: %w", err)
	}
	return summary, nil
}

// rebaselineClearDrift removes the tenant's drift/ tree after a run that
// updated the export baseline. Never under dry-run: a dry run writes nothing,
// so it deletes nothing. The delete goes through drift.ClearTree, whose path is
// constructed — never derived from input — so it cannot reach into resources/
// or docs/. A failure only warns: the download's own output is already safe,
// and the stale observation remains self-describing (docs analyze-drift
// refuses it as superseded).
func rebaselineClearDrift(tenantDir string, dryRun bool) {
	log := logger.Default
	if dryRun {
		return
	}
	removed, err := drift.ClearTree(tenantDir)
	switch {
	case err != nil:
		log.Warn("Superseded drift observation not cleared", "error", err)
	case removed:
		log.Info("Cleared the drift observation: this run re-baselined the export it was compared against",
			"dir", filepath.Join(tenantDir, drift.DriftDirName))
	}
}
