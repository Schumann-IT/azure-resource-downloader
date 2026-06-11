package pipeline

import (
	"context"
	"fmt"
	"sync"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/retry"
)

// Fetcher handles fetching resources from Azure
type Fetcher struct {
	azureClient *azure.Client
	registry    *handlers.Registry
	workerCount int
}

// NewFetcher creates a new fetcher
func NewFetcher(azureClient *azure.Client, registry *handlers.Registry, workerCount int) *Fetcher {
	return &Fetcher{
		azureClient: azureClient,
		registry:    registry,
		workerCount: workerCount,
	}
}

// Fetch retrieves resources from Azure asynchronously
func (f *Fetcher) Fetch(ctx context.Context, requests []*models.FetchRequest) <-chan *models.FetchResult {
	out := make(chan *models.FetchResult)

	go func() {
		defer close(out)

		// Create input channel for workers
		requestsChan := make(chan *models.FetchRequest, len(requests))
		for _, req := range requests {
			requestsChan <- req
		}
		close(requestsChan)

		// Start worker pool
		var wg sync.WaitGroup
		for i := 0; i < f.workerCount; i++ {
			wg.Add(1)
			go f.fetchWorker(ctx, requestsChan, out, &wg)
		}

		// Wait for all workers to complete
		wg.Wait()
	}()

	return out
}

// fetchWorker processes fetch requests
func (f *Fetcher) fetchWorker(ctx context.Context, requests <-chan *models.FetchRequest, results chan<- *models.FetchResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for req := range requests {
		select {
		case <-ctx.Done():
			results <- &models.FetchResult{
				ResourceID: req.ResourceID,
				Error:      ctx.Err(),
			}
			return
		default:
			result := f.fetchResource(ctx, req)
			results <- result
		}
	}
}

// fetchResource fetches a single resource with retry logic
func (f *Fetcher) fetchResource(ctx context.Context, req *models.FetchRequest) *models.FetchResult {
	log := logger.Default

	// Parse resource ID to get type information
	idInfo, err := azure.ParseResourceID(req.ResourceID)
	if err != nil {
		log.Error("Failed to parse resource ID",
			"resource_id", req.ResourceID,
			"error", err)
		return &models.FetchResult{
			ResourceID: req.ResourceID,
			Error:      fmt.Errorf("failed to parse resource ID: %w", err),
		}
	}

	resourceType := idInfo.FullType
	if resourceType == "" {
		resourceType = req.ResourceType
	}

	// Get handler for this resource type
	handler, err := f.registry.Get(resourceType)
	if err != nil {
		log.Error("No handler for resource type",
			"resource_id", req.ResourceID,
			"type", resourceType,
			"error", err)
		return &models.FetchResult{
			ResourceID:   req.ResourceID,
			ResourceType: resourceType,
			Error:        fmt.Errorf("no handler for resource type %s: %w", resourceType, err),
		}
	}

	// Log start of fetch
	log.Debug("Fetching resource",
		"resource_id", req.ResourceID,
		"type", resourceType,
		"name", idInfo.ResourceName)

	// Fetch the resource using the handler with retry logic
	retryConfig := retry.DefaultConfig()
	attemptNum := 0

	rawData, err := retry.DoWithData(ctx, retryConfig, func() (interface{}, error) {
		attemptNum++
		if attemptNum > 1 {
			log.Warn("Retrying resource fetch",
				"resource_id", req.ResourceID,
				"attempt", attemptNum,
				"max_attempts", retryConfig.MaxAttempts)
		}
		return handler.Fetch(ctx, req.ResourceID)
	})

	if err != nil {
		// A permission/authorization failure means the signed-in user simply
		// isn't allowed to read this resource. Warn and skip instead of failing
		// the whole run.
		if azure.IsPermissionError(err) {
			log.Warn("Skipping resource: not permitted for the signed-in user",
				"resource_id", req.ResourceID,
				"type", resourceType,
				"reason", azure.ErrorSummary(err))
			log.Debug("Fetch failed with permission error",
				"resource_id", req.ResourceID,
				"type", resourceType,
				"error", err)
			return &models.FetchResult{
				ResourceID:   req.ResourceID,
				ResourceType: resourceType,
				Skipped:      true,
				SkipReason:   err.Error(),
			}
		}

		log.Error("Failed to fetch resource",
			"resource_id", req.ResourceID,
			"type", resourceType,
			"attempts", attemptNum,
			"error", err)
		return &models.FetchResult{
			ResourceID:   req.ResourceID,
			ResourceType: resourceType,
			Error:        fmt.Errorf("failed to fetch resource: %w", err),
		}
	}

	if attemptNum > 1 {
		log.Info("Resource fetch succeeded after retries",
			"resource_id", req.ResourceID,
			"attempts", attemptNum)
	} else {
		log.Debug("Resource fetched successfully",
			"resource_id", req.ResourceID,
			"type", resourceType)
	}

	return &models.FetchResult{
		ResourceID:   req.ResourceID,
		ResourceType: resourceType,
		RawData:      rawData,
		Error:        nil,
	}
}
