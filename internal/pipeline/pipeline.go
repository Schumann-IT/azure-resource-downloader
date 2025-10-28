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
	return &Pipeline{
		fetcher:     NewFetcher(azureClient, registry, config.WorkerCount),
		transformer: NewTransformer(registry, config.WorkerCount, config.ExcludeKeys, config.ExcludeKeysByType),
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
	log.Info("⚡ Pipeline stages run CONCURRENTLY via streaming channels")
	log.Info("   Each resource flows: Fetch → Transform → Write in parallel")

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

		if writeResult.Error != nil {
			summary.FailedResources++
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %v", writeResult.ResourceID, writeResult.Error))
		} else {
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
	Results             []*models.WriteResult
	Errors              []string
}

// PrintSummary prints a summary of the execution
func (s *ExecutionSummary) PrintSummary() {
	log := logger.Default

	log.Info("Pipeline Execution Summary",
		"total", s.TotalResources,
		"successful", s.SuccessfulResources,
		"failed", s.FailedResources)

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
