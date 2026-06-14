package pipeline

import (
	"context"
	"fmt"
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
		fetcher:     NewFetcher(azureClient, registry, config.WorkerCount),
		transformer: NewTransformer(registry, config.WorkerCount, transformerConfigs, config.ResourceFilters),
		writer:      NewWriter(config.OutputDir, config.WorkerCount, config.DryRun),
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

	// Create context with timeout if configured
	if p.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.Timeout)
		defer cancel()
	}

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
				"failed", summary.FailedResources,
				"elapsed", time.Since(metrics.StartTime).Round(time.Second))
		}
	}

	return summary, nil
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// ExecutionSummary contains the results of a pipeline execution
type ExecutionSummary struct {
	TotalResources      int
	SuccessfulResources int
	FailedResources     int
	// SkippedResources counts resources the signed-in user was not permitted to
	// read. They are reported as warnings and do not cause a non-zero exit.
	SkippedResources int
	// FilteredResources counts resources excluded by a configured resource
	// filter. They are not written and do not cause a non-zero exit.
	FilteredResources int
	// SkippedTypes lists resource types whose listing failed before the
	// pipeline ran; their resource counts are not part of the totals above.
	SkippedTypes []models.SkippedType
	// EmptyTypes lists resource types whose listing succeeded but returned no
	// resources (nothing exists, insufficient permissions, or different scope).
	EmptyTypes []string
	Results    []*models.WriteResult
	Errors     []string
}

// PrintSummary prints a summary of the execution
func (s *ExecutionSummary) PrintSummary() {
	log := logger.Default

	log.Info("Pipeline Execution Summary",
		"total", s.TotalResources,
		"successful", s.SuccessfulResources,
		"skipped", s.SkippedResources,
		"filtered", s.FilteredResources,
		"failed", s.FailedResources,
		"skipped_types", len(s.SkippedTypes),
		"empty_types", len(s.EmptyTypes))

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
					"yaml", result.YAMLPath,
					"terraform", result.TerraformPath)
			}
		}
	}
}
