package pipeline

import (
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
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
		transformer: NewTransformer(registry, config.WorkerCount),
		writer:      NewWriter(config.OutputDir, config.WorkerCount, config.DryRun),
		config:      config,
	}
}

// Execute runs the pipeline for the given resources
func (p *Pipeline) Execute(ctx context.Context, requests []*models.FetchRequest) (*ExecutionSummary, error) {
	summary := &ExecutionSummary{
		TotalResources: len(requests),
		Results:        make([]*models.WriteResult, 0),
	}

	// Create context with timeout if configured
	if p.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.config.Timeout)
		defer cancel()
	}

	// Stage 1: Fetch
	fetchResults := p.fetcher.Fetch(ctx, requests)

	// Stage 2: Transform
	transformResults := p.transformer.Transform(ctx, fetchResults)

	// Stage 3: Write
	writeResults := p.writer.Write(ctx, transformResults)

	// Collect results
	for writeResult := range writeResults {
		summary.Results = append(summary.Results, writeResult)

		if writeResult.Error != nil {
			summary.FailedResources++
			summary.Errors = append(summary.Errors, fmt.Sprintf("%s: %v", writeResult.ResourceID, writeResult.Error))
		} else {
			summary.SuccessfulResources++
		}
	}

	return summary, nil
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
	fmt.Printf("\n=== Pipeline Execution Summary ===\n")
	fmt.Printf("Total resources: %d\n", s.TotalResources)
	fmt.Printf("Successful: %d\n", s.SuccessfulResources)
	fmt.Printf("Failed: %d\n", s.FailedResources)

	if len(s.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, err := range s.Errors {
			fmt.Printf("  - %s\n", err)
		}
	}

	fmt.Printf("\nFiles written:\n")
	for _, result := range s.Results {
		if result.Error == nil {
			fmt.Printf("  - YAML: %s\n", result.YAMLPath)
			fmt.Printf("  - TF:   %s\n", result.TerraformPath)
		}
	}
}
