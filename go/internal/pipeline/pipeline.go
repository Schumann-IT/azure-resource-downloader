package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
)

// Pipeline orchestrates the fetch-transform-write pipeline
type Pipeline struct {
	fetcher     *Fetcher
	transformer *Transformer
	writer      *Writer
	config      *models.PipelineConfig
}

// NewPipeline creates a new pipeline
func NewPipeline(azureClient *azure.Client, registry *handlers.Registry, config *models.PipelineConfig) *Pipeline {
	log := logger.Default

	// Use transformer configs from config (could be empty if user wants no transformers)
	transformerConfigs := config.TransformerConfigs

	// NOTE: We do NOT apply defaults here if empty!
	// Empty list means user explicitly disabled transformers
	// Only cmd/download.go should apply defaults when config key is missing

	if len(transformerConfigs) == 0 {
		log.Debug("Pipeline initialized with no transformers")
	} else {
		log.Debug("Pipeline initialized with transformers", "count", len(transformerConfigs))
	}

	return &Pipeline{
		fetcher:     NewFetcher(azureClient, registry, config.WorkerCount, config.Timeout),
		transformer: NewTransformer(registry, config.WorkerCount, transformerConfigs, config.ResourceFilters),
		writer:      NewWriter(config.OutputDir, config.WorkerCount, config.DryRun, config.WritePrompts),
		config:      config,
	}
}

// Execute runs the pipeline for the given resources
func (p *Pipeline) Execute(ctx context.Context, requests []*models.FetchRequest) (*ExecutionSummary, error) {
	log := logger.Default

	summary := &ExecutionSummary{
		TotalResources: len(requests),
		Results:        make([]*models.WriteResult, 0),
	}

	// Create performance metrics tracker
	metrics := NewPipelineMetrics(p.config.WorkerCount, len(requests))
	defer metrics.LogSummary()

	// NOTE: config.Timeout is a PER-OPERATION deadline applied around each
	// resource fetch inside the fetcher, not a whole-run budget. The pipeline
	// intentionally runs on the caller's context so a slow tenant cannot
	// silently abandon queued requests (which would break the completeness
	// invariant below).

	log.Info("Starting pipeline",
		"resources", len(requests),
		"workers", p.config.WorkerCount)

	// All three stages start immediately and run concurrently
	// They're connected via Go channels for streaming data flow
	pipelineStart := time.Now()

	// Stage 1: Fetch (starts immediately, returns channel)
	fetchResults := p.fetcher.Fetch(ctx, requests)

	// Stage 2: Transform (starts consuming immediately)
	transformResults := p.transformer.Transform(ctx, fetchResults)

	// Stage 3: Write (starts consuming immediately)
	writeResults := p.writer.Write(ctx, transformResults)

	log.Info("All pipeline stages started",
		"elapsed", time.Since(pipelineStart).Round(time.Millisecond),
		"note", "Stages are now running in parallel")

	// Collect results with progress tracking
	processedCount := 0
	for writeResult := range writeResults {
		summary.Results = append(summary.Results, writeResult)
		processedCount++
		metrics.RecordResult()

		switch {
		case writeResult.Cancelled:
			summary.CancelledResources++
		case writeResult.Filtered:
			summary.FilteredResources++
		case writeResult.Skipped:
			summary.SkippedResources++
		case writeResult.Error != nil:
			summary.FailedResources++
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %v", writeResult.ResourceID, writeResult.Error))
		default:
			summary.SuccessfulResources++
		}

		// Log progress every 10% or on errors
		progressInterval := max(1, len(requests)/10)
		if processedCount%progressInterval == 0 || writeResult.Error != nil || processedCount == len(requests) {
			log.Info("Progress",
				"completed", processedCount,
				"total", len(requests),
				"percentage", fmt.Sprintf("%.1f%%", float64(processedCount)/float64(len(requests))*100),
				"successful", summary.SuccessfulResources,
				"skipped", summary.SkippedResources,
				"filtered", summary.FilteredResources,
				"cancelled", summary.CancelledResources,
				"failed", summary.FailedResources,
				"elapsed", time.Since(metrics.StartTime).Round(time.Second))
		}
	}

	// Collect the per-type documentation prompt hashes gathered by the writer
	// (populated only when prompt writing is enabled, i.e. --no-prompt was not
	// passed). writePromptFiles runs before the write channel closes, so they
	// are complete here.
	summary.PromptSHAByType = p.writer.PromptSHAByType()

	// Assert the accounting invariant: every request must have produced exactly
	// one result. A mismatch is a pipeline bug (a lost or duplicated result),
	// not a condition to tolerate, because a missing result would later look
	// like a resource deleted in the tenant and could be pruned from disk.
	if len(summary.Results) != summary.TotalResources {
		return summary, fmt.Errorf("pipeline invariant violated: %d results produced for %d requests", len(summary.Results), summary.TotalResources)
	}

	return summary, nil
}

// DryRunSummary builds the ExecutionSummary for a list-only dry run: one
// would-download result per request, with no fetch, transform or write
// performed. It exists so the completeness accounting and the one-result-per-
// request invariant hold for a dry run exactly as they do for a real run,
// without the per-resource Azure traffic a real download incurs. The request
// set itself is the offline answer to "what would be downloaded" — it is
// produced by the per-type listing upstream of the fetcher, which a dry run
// still performs.
//
// Each result carries only its resource id and type (no path, no facts): under
// --dry-run nothing is marshalled, so there is no output path or hash to record.
// That is enough for the metadata prune preview, which matches present
// resources by id.
func DryRunSummary(requests []*models.FetchRequest) *ExecutionSummary {
	s := &ExecutionSummary{
		TotalResources: len(requests),
		DryRun:         true,
		Results:        make([]*models.WriteResult, 0, len(requests)),
	}
	for _, r := range requests {
		s.Results = append(s.Results, &models.WriteResult{
			ResourceID:   r.ResourceID,
			ResourceType: r.ResourceType,
		})
		s.WouldDownload++
	}
	return s
}

// ExecutionSummary contains the results of a pipeline execution
type ExecutionSummary struct {
	TotalResources      int
	SuccessfulResources int
	FailedResources     int
	// DryRun reports that this summary describes a list-only dry run: the
	// request set was listed but no resource was fetched, transformed or
	// written. WouldDownload then counts the resources a real run would fetch.
	DryRun        bool
	WouldDownload int
	// SkippedResources counts resources the signed-in user was not permitted to
	// read. They are reported as warnings and do not cause a non-zero exit.
	SkippedResources int
	// FilteredResources counts resources excluded by a configured resource
	// filter. They are not written and do not cause a non-zero exit.
	FilteredResources int
	// CancelledResources counts requests that produced no work because the
	// pipeline was cancelled before processing them. Any cancellation makes the
	// run incomplete.
	CancelledResources int
	// SkippedTypes lists resource types whose listing failed before the
	// pipeline ran; their resource counts are not part of the totals above.
	SkippedTypes []models.SkippedType
	// EmptyTypes lists resource types whose listing succeeded but returned no
	// resources (nothing exists, insufficient permissions, or different scope).
	EmptyTypes []string
	// PromptSHAByType maps a resource type to the SHA-256 of its assembled
	// doc-prompt.md content. Populated only when prompt writing is enabled
	// (i.e. --no-prompt was not passed).
	PromptSHAByType map[string]string
	// Complete reports whether the run knows it downloaded everything in scope:
	// every request produced a result, no stage was cancelled, and no type
	// failed to list. When false, IncompleteReason states why.
	Complete         bool
	IncompleteReason string
	Results          []*models.WriteResult
	Errors           []string
}

// MarkCompleteness derives Complete and IncompleteReason from the summary's
// fields. It must be called after SkippedTypes has been populated, since a type
// that failed to list makes the run incomplete. A run is complete when every
// request produced a result, nothing was cancelled, and every type listed.
func (s *ExecutionSummary) MarkCompleteness() {
	var reasons []string
	if len(s.Results) != s.TotalResources {
		reasons = append(reasons, fmt.Sprintf("only %d of %d requests produced a result", len(s.Results), s.TotalResources))
	}
	if s.CancelledResources > 0 {
		reasons = append(reasons, fmt.Sprintf("%d requests were cancelled", s.CancelledResources))
	}
	if len(s.SkippedTypes) > 0 {
		reasons = append(reasons, fmt.Sprintf("%d resource types could not be listed", len(s.SkippedTypes)))
	}
	s.Complete = len(reasons) == 0
	if s.Complete {
		s.IncompleteReason = ""
	} else {
		s.IncompleteReason = strings.Join(reasons, "; ")
	}
}

// PrintSummary prints a summary of the execution
func (s *ExecutionSummary) PrintSummary() {
	log := logger.Default

	// A dry run lists rather than downloads, so report what it would fetch
	// instead of success/failure counts that never accrue when no stage ran.
	if s.DryRun {
		log.Info("Dry-run Summary (list only — nothing fetched, transformed or written)",
			"would_download", s.WouldDownload,
			"total", s.TotalResources,
			"skipped_types", len(s.SkippedTypes),
			"empty_types", len(s.EmptyTypes))
		if s.Complete {
			log.Info("Listing is complete: every type in scope was listed, so this is the full set that would be downloaded")
		} else {
			log.Warn("Listing is INCOMPLETE; the set that would be downloaded is a subset of the tenant", "reason", s.IncompleteReason)
		}
		if len(s.SkippedTypes) > 0 {
			for _, st := range s.SkippedTypes {
				log.Warn("Skipped type (could not be listed)", "type", st.ResourceType, "reason", st.Reason)
			}
		}
		if len(s.EmptyTypes) > 0 {
			for _, t := range s.EmptyTypes {
				log.Warn("Empty type (listed, no resources)", "type", t)
			}
		}
		return
	}

	log.Info("Pipeline Execution Summary",
		"total", s.TotalResources,
		"successful", s.SuccessfulResources,
		"skipped", s.SkippedResources,
		"filtered", s.FilteredResources,
		"cancelled", s.CancelledResources,
		"failed", s.FailedResources,
		"skipped_types", len(s.SkippedTypes),
		"empty_types", len(s.EmptyTypes))

	if s.Complete {
		log.Info("Run is complete: every request produced a result and every type in scope was listed")
	} else {
		log.Warn("Run is INCOMPLETE; do not treat missing resources as deleted", "reason", s.IncompleteReason)
	}

	if s.CancelledResources > 0 {
		log.Warn("Some requests were cancelled before completion", "cancelled", s.CancelledResources)
	}

	if s.FilteredResources > 0 {
		log.Info("Some resources were excluded by configured resource filters",
			"filtered", s.FilteredResources)
	}

	if s.SkippedResources > 0 {
		log.Warn("Some resources were skipped because the signed-in user is not permitted to read them",
			"skipped", s.SkippedResources)
	}

	if len(s.SkippedTypes) > 0 {
		log.Warn("Some resource types could not be listed and were skipped entirely; their resource counts are unknown and not included in the totals",
			"count", len(s.SkippedTypes))
		for _, st := range s.SkippedTypes {
			log.Warn("Skipped type", "type", st.ResourceType, "reason", st.Reason)
		}
	}

	if len(s.EmptyTypes) > 0 {
		log.Warn("Some resource types returned no resources (nothing exists, insufficient permissions, or a different scope)",
			"count", len(s.EmptyTypes))
		for _, t := range s.EmptyTypes {
			log.Warn("Empty type", "type", t)
		}
	}

	if len(s.Errors) > 0 {
		log.Warn("Errors occurred during execution")
		for _, err := range s.Errors {
			log.Error(err)
		}
	}

	// Log successful results
	if s.SuccessfulResources > 0 {
		log.Info("Files written", "count", s.SuccessfulResources)
		for _, result := range s.Results {
			if result.Error == nil {
				log.Debug("Resource files",
					"yaml", result.YAMLPath)
			}
		}
	}
}
