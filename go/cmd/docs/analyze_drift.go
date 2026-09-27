package docs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"azure-resource-downloader/internal/cmdutil"
	docsengine "azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewAnalyzeDriftCommand builds the `docs analyze-drift` subcommand. It emits a
// ready-to-use analysis prompt covering exactly the findings of the latest
// drift observation, so an LLM can report their impact (security, compliance,
// lifecycle, who is affected). It is exported so the parent `docs` command (in
// package cmd) can attach it without an import cycle.
func NewAnalyzeDriftCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze-drift",
		Short: "Emit a prompt that analyzes the impact of the latest drift observation",
		Long: `Render a drift-analysis prompt from the latest 'resource drift' observation
(drift/metadata.yaml) and the export baseline (resources/metadata.yaml). The
prompt directs an LLM to judge each finding's impact — security posture,
compliance, lifecycle and who is affected — and to write one drift document per
finding (drift/<path>.md, beside the payload it judges, so a frontend can show
the YAML diff and the judgment side by side) plus a summary index to
drift/index.md.

This command never fetches a resource and never writes into resources/ or
docs/. It authenticates only to resolve the tenant's Entra default domain (the
export folder name), exactly as 'download' does. Pass --domain to skip
authentication entirely and run offline against a named export folder.

The prompt is written to output/<tenant>/drift/analyze.md (override with --out).
Prompt, drift documents and index live with the observation they belong to: the
next 'resource drift' run — and a re-baselining 'resource download' — clears
the drift/ tree, so archive them manually if they must be kept. It refuses
(exit 2) when there is no observation, or when the export was re-baselined
after the observation was made; re-run 'resource drift' first in both cases.

Examples:
  # Resolve the tenant via 'az login', then write the analysis prompt
  azure-rd docs analyze-drift

  # Offline: name the export folder explicitly (no sign-in)
  azure-rd docs analyze-drift --domain contoso.onmicrosoft.com

  # Preview what would be analyzed without writing the prompt
  azure-rd docs analyze-drift --domain contoso.onmicrosoft.com --dry-run`,
		RunE: runAnalyzeDrift,
	}

	addExportFlags(cmd, "path to write the prompt to (default: <output>/<tenant>/drift/analyze.md)")

	cmd.Flags().String("prompt", "", "path to a template file overriding the built-in drift-analysis template")

	return cmd
}

func runAnalyzeDrift(cmd *cobra.Command, _ []string) error {
	cmdutil.BindFlags(cmd)

	ctx := cmd.Context()
	log := logger.Default

	baseOutput := viper.GetString("output")
	dryRun := viper.GetBool("dry-run")
	domain := viper.GetString("domain")
	outPath := viper.GetString("out")
	promptPath := viper.GetString("prompt")

	// Resolve the export directory and the domain to cross-check the
	// observation and metadata against.
	tenantDir, expectDomain, err := resolveExportDir(ctx, baseOutput, domain,
		viper.GetString("subscription"), viper.GetString("client-id"), viper.GetString("tenant-id"))
	if err != nil {
		return cmdutil.WithExitCode(exitCannotAnswer, fmt.Errorf("cannot resolve which export to analyze: %w", err))
	}
	log.Info("Analyzing drift observation", "dir", tenantDir)

	// Load the template: the embedded default, or a --prompt override.
	template := drift.DefaultAnalyzeTemplate()
	if promptPath != "" {
		template, err = os.ReadFile(promptPath)
		if err != nil {
			return cmdutil.WithExitCode(exitCannotAnswer, fmt.Errorf("cannot read --prompt template: %w", err))
		}
	}

	// A stale analyze.md from an earlier run outlives a dry run — surface it so
	// nobody pastes an out-of-date prompt.
	resolvedOut := outPath
	if resolvedOut == "" {
		resolvedOut = filepath.Join(tenantDir, drift.DriftDirName, drift.AnalyzeFileName)
	}
	if dryRun {
		noteStaleGeneratePrompt(resolvedOut)
	}

	res, err := drift.GenerateAnalyzePrompt(drift.AnalyzeOptions{
		TenantDir:    tenantDir,
		ExpectDomain: expectDomain,
		Template:     template,
		OutPath:      outPath,
		DryRun:       dryRun,
	})
	if err != nil {
		return cmdutil.WithExitCode(exitCannotAnswer, analyzeDriftError(err))
	}

	reportAnalyzeDrift(res, dryRun)
	return nil
}

// analyzeDriftError maps the engine's sentinel errors to actionable messages.
func analyzeDriftError(err error) error {
	switch {
	case errors.Is(err, drift.ErrNoObservation):
		return fmt.Errorf("no drift observation found; run 'azure-rd resource drift' first: %w", err)
	case errors.Is(err, drift.ErrObservationSuperseded):
		return fmt.Errorf("the export was re-baselined after this observation; run 'azure-rd resource drift' again: %w", err)
	case errors.Is(err, drift.ErrPayloadMismatch):
		return fmt.Errorf("the drift tree does not match its observation; re-run 'azure-rd resource drift': %w", err)
	case errors.Is(err, docsengine.ErrNoMetadata):
		return fmt.Errorf("no export metadata found; run 'azure-rd resource download' first: %w", err)
	case errors.Is(err, docsengine.ErrTenantMismatch):
		return fmt.Errorf("refusing to analyze the wrong export: %w", err)
	default:
		return fmt.Errorf("failed to generate drift-analysis prompt: %w", err)
	}
}

// reportAnalyzeDrift prints the outcome, including the observation's freshness
// so a forgotten drift run is visible.
func reportAnalyzeDrift(res *drift.AnalyzeResult, dryRun bool) {
	log := logger.Default

	c := res.Counts
	log.Info("Observation under analysis",
		"observed_at", res.ObservedAt,
		"baseline_generated_at", res.BaselineGeneratedAt,
		"changed", c.Changed,
		"added", c.Added,
		"removed", c.Removed,
		"renamed", c.Renamed)

	for _, t := range res.MissingSpecTypes {
		log.Warn("Type has no doc-prompt.md; the analysis loses its type-specific lens (was the export run with --no-prompt?)", "type", t)
	}

	if res.NothingToAnalyze {
		log.Info("The observation recorded no drift; nothing to analyze and no prompt written")
		return
	}
	if dryRun {
		log.Info("Dry-run: analysis prompt not written", "would_write", res.OutPath)
		return
	}
	log.Info("Drift-analysis prompt written", "path", res.OutPath, "index_destination", res.IndexPath)
	log.Info("The prompt, the drift documents and the index live with the observation: the next 'resource drift' or re-baselining 'resource download' run deletes them")
}
