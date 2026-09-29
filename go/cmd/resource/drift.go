package resource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"azure-resource-downloader/internal/cmdutil"
	"azure-resource-downloader/internal/docs"
	"azure-resource-downloader/internal/drift"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/pipeline"
	"azure-resource-downloader/internal/runprep"
	"azure-resource-downloader/internal/tenantdir"
	"azure-resource-downloader/internal/version"

	"github.com/spf13/cobra"
)

// Exit codes for resource drift, carried to the root Execute via
// cmdutil.WithExitCode. 0 is success whether or not drift was found (unless
// --exit-code); a distinct code marks "could not answer" so CI can tell an
// unanswerable run from a clean one.
const (
	driftExitCannotAnswer = 2
	driftExitDriftFound   = 3
)

// NewDriftCommand builds the `resource drift` command: has the tenant changed
// since the last download? It shares the selection flags and --domain declared
// on the `resource` parent and the run preparation with download, so the two
// can never diverge in auth or selection semantics.
func NewDriftCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Detect drift between the tenant and the export on disk",
		Long: `Fetch the tenant's current state, compare it against the export already on
disk, and record what was added, changed, renamed or removed — without
re-baselining anything. The export stays untouched: this command writes nothing
under resources/ or docs/; its only output is the <tenant>/drift/ tree, cleared
and rebuilt on every run so it holds exactly the latest observation
(drift/metadata.yaml plus the fetched bytes of every added, changed and renamed
resource at paths mirroring resources/). Re-baselining is a normal
'resource download'.

The comparison is only meaningful against the configuration that produced the
baseline: the command refuses (exit 2) when the export's recorded transform or
filter configuration differs from this run's, and reports entries written under
another configuration as unattested rather than drifted. Removals are asserted
only when the run is complete and the type was actually covered — the same rule
--prune applies — and suppressed otherwise.

Unlike a download, nothing about drift can be answered without the tenant's
current bytes, so --dry-run still fetches and still reports in full; it only
withholds the drift/ tree (clearing nothing).

Exit codes: 0 on success (drift found or not), 2 when the question cannot be
answered (no baseline, wrong tenant, incomparable configuration), non-zero only
for failed fetches — plus, with --exit-code, 3 when drift was found (for CI).

Examples:
  # Compare the tenant against its export
  azure-rd resource drift

  # One type only
  azure-rd resource drift --type "Microsoft.Graph/groups"

  # Assert which export to compare against (refuses a different tenant)
  azure-rd resource drift --domain contoso.onmicrosoft.com

  # Report in full but leave the drift/ tree untouched
  azure-rd resource drift --dry-run

  # Fail (exit 3) when drift was found, for CI gating
  azure-rd resource drift --exit-code`,
		RunE: runDrift,
	}

	// The selection flags and --domain are inherited from the `resource` parent.
	// Only --exit-code is declared here: it changes this invocation's exit status
	// and nothing else, and its meaning is command-specific (drift found), so it
	// cannot be one shared configuration key. The settings that must match how
	// the baseline was written — the transformers, the filters, secret resolution
	// — are configuration precisely so they cannot be typed differently here than
	// they were for the download; the comparability preflight refuses otherwise.
	cmd.Flags().Bool("exit-code", false, "exit non-zero (3) when drift was found, for CI gating")

	return cmd
}

func runDrift(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	log := logger.Default

	exitCode, _ := cmd.Flags().GetBool("exit-code")

	// The preparation shared with `resource download`: configuration, session
	// verification, the dedicated-app probe and prompt, authentication, tenant
	// resolution and the handler registry. It also decides which export this run
	// compares against and refuses rather than compare the wrong tenant — drift
	// against another tenant's export is all noise. Those refusals are
	// "cannot answer", not failures, so they carry the distinct exit code.
	prep, err := runprep.Prepare(ctx, runprep.Options{Domain: cmdutil.DeclaredDomain(cmd)})
	if err != nil {
		if errors.Is(err, tenantdir.ErrMismatch) || errors.Is(err, tenantdir.ErrUnresolved) {
			return cmdutil.WithExitCode(driftExitCannotAnswer, err)
		}
		return err
	}
	expectDomain := prep.Tenant
	tenantDir := prep.Output

	// Load the baseline and run the comparability preflight before fetching
	// anything: an unanswerable run must refuse cheaply, not after a full
	// tenant fetch.
	meta, err := docs.LoadExportMetadata(tenantDir)
	if err != nil {
		if errors.Is(err, docs.ErrNoMetadata) {
			err = fmt.Errorf("no baseline to compare against; run 'azure-rd resource download' first: %w", err)
		}
		return cmdutil.WithExitCode(driftExitCannotAnswer, err)
	}

	currentTransformSha := docs.HashTransformConfig(prep.TransformerConfigs, prep.ResolveSecrets)
	currentFiltersSha := docs.HashResourceFilters(prep.ResourceFilters)

	warnings, err := drift.Preflight(meta, expectDomain, currentTransformSha, currentFiltersSha)
	if err != nil {
		return cmdutil.WithExitCode(driftExitCannotAnswer, err)
	}
	for _, w := range warnings {
		log.Warn(w)
	}
	if w := drift.Base64FileModeWarning(prep.TransformerConfigs); w != "" {
		log.Warn(w)
	}

	log.Info("Comparing tenant against export", "export", tenantDir, "baseline_generated_at", meta.GeneratedAt)

	// Build fetch requests through the shared listing path, then run the
	// download's own fetch and transform stages — compare instead of writing.
	requests, skippedTypes, emptyTypes, err := prep.BuildRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to build fetch requests: %w", err)
	}

	workers := prep.FetchWorkerCount()
	prep.LogWorkerConfiguration(workers)

	// DryRun stays false in the pipeline config: it governs the writer, which
	// a drift run never uses — under --dry-run drift still fetches (nothing
	// about drift is answerable offline) and only withholds its writes below.
	pipelineConfig := &models.PipelineConfig{
		OutputDir:          tenantDir,
		WorkerCount:        workers,
		Timeout:            time.Duration(prep.Timeout) * time.Second,
		DryRun:             false,
		SubscriptionID:     prep.Subscription,
		TransformerConfigs: prep.TransformerConfigs,
		ResourceFilters:    prep.ResourceFilters,
	}

	p := pipeline.NewPipeline(prep.Client, prep.Registry, pipelineConfig)
	log.Info("Fetching the tenant's current state...", "resources", len(requests), "workers", workers)
	results, err := p.CollectTransforms(ctx, requests)
	if err != nil {
		return fmt.Errorf("pipeline execution failed: %w", err)
	}

	rep := drift.Compare(drift.Options{
		Baseline:               meta,
		ResourcesDir:           filepath.Join(tenantDir, models.ResourcesDirName),
		CurrentTransformSha256: currentTransformSha,
		Scope:                  docs.RunScope{Types: prep.SelectedTypes, ResourceIDs: prep.ResourceIDs, ResourceGroup: prep.ResourceGroup},
		TotalRequests:          len(requests),
		Results:                results,
		SkippedTypes:           skippedTypes,
		EmptyTypes:             emptyTypes,
	})
	for _, w := range rep.Warnings {
		log.Warn(w)
	}

	// Persist the observation — or, under --dry-run, withhold it and say so:
	// an observation from an earlier run stays on disk and was not refreshed.
	if prep.DryRun {
		noteStaleObservation(drift.ObservationPath(tenantDir))
	}
	metaPath, err := drift.WriteObservation(tenantDir, rep, time.Now(), version.Tool(), prep.DryRun)
	if err != nil {
		return fmt.Errorf("failed to write the drift observation: %w", err)
	}

	reportDrift(rep, metaPath, prep.DryRun)

	if failed := rep.Observation.Counts.Failed; failed > 0 {
		return fmt.Errorf("drift check completed with errors (%d resources failed to fetch)", failed)
	}
	if exitCode && rep.DriftFound {
		return cmdutil.WithExitCode(driftExitDriftFound, errors.New("drift detected (see the report above)"))
	}
	return nil
}

// noteStaleObservation prints the path and age of an existing observation so a
// dry run cannot be mistaken for having refreshed it.
func noteStaleObservation(metaPath string) {
	info, err := os.Stat(metaPath)
	if err != nil {
		return
	}
	logger.Default.Warn("An observation from an earlier run is still on disk and will NOT be refreshed by this dry run",
		"path", metaPath,
		"age", time.Since(info.ModTime()).Round(time.Second).String())
}

// reportDrift prints the observation: a summary, the findings, the exclusions
// and a verdict line, plus how many documents the observed drift will make
// stale once re-baselined.
func reportDrift(rep *drift.Report, metaPath string, dryRun bool) {
	log := logger.Default
	obs := rep.Observation
	c := obs.Counts

	log.Info("Drift Summary",
		"compared", c.Compared,
		"unchanged", c.Unchanged,
		"changed", c.Changed,
		"added", c.Added,
		"removed", c.Removed,
		"renamed", c.Renamed,
		"unknown_types", len(obs.UnknownTypes),
		"unattested", c.Unattested,
		"excluded", c.Excluded,
		"failed", c.Failed)

	if !obs.Run.Complete {
		log.Warn("The comparison is INCOMPLETE; it may understate the drift", "reason", obs.Run.IncompleteReason)
	}
	if obs.RemovalsSuppressed {
		log.Warn("Removals were suppressed: an incomplete run cannot tell a deleted resource from one it never reached")
	}
	for _, t := range obs.UnknownTypes {
		log.Warn("Type could not be listed; its drift is unknown (excluded from the totals)", "type", t)
	}
	for _, nc := range obs.NotComparable {
		log.Warn("Baseline entry is not comparable; reported as unattested, not as drift", "source", nc.Key, "reason", nc.Reason)
	}

	for key, f := range obs.Findings {
		kv := []interface{}{"resource", key, "name", f.DisplayName}
		switch f.Verdict {
		case drift.VerdictRenamed:
			kv = append(kv, "previous_name", f.PreviousDisplayName, "previous_path", f.BaselineKey)
		case drift.VerdictChanged:
			kv = append(kv, "deltas", len(f.Deltas))
		}
		log.Info("  "+f.Verdict, kv...)
		for _, d := range f.Deltas {
			log.Debug("    delta", "path", d.Path, "old", d.Old, "new", d.New)
		}
	}

	if rep.DriftFound {
		log.Warn("The tenant has drifted from the export",
			"drifted", c.Added+c.Changed+c.Renamed+c.Removed)
		// Close the loop with the documentation pipeline: once re-baselined,
		// added/changed/renamed resources make their documents stale and
		// removed resources leave orphans.
		log.Info("Once re-baselined ('azure-rd resource download'), documents will need regenerating (see 'azure-rd docs generate-prompt')",
			"documents_stale", c.Added+c.Changed+c.Renamed,
			"documents_orphaned", c.Removed)
	} else {
		log.Info("No drift detected: the tenant matches the export", "findings_sha256", obs.FindingsSha256)
	}

	if dryRun {
		log.Info("Dry-run: drift tree not written", "would_write", metaPath)
		return
	}
	log.Info("Drift observation written", "path", metaPath, "payloads", len(obs.Payloads))
}
