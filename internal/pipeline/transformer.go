package pipeline

import (
	"context"
	"fmt"
	"sync"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/transform"
)

// Transformer handles transforming resources
type Transformer struct {
	registry          *handlers.Registry
	workerCount       int
	excludeKeys       []string
	excludeKeysByType map[string][]string
}

// NewTransformer creates a new transformer
func NewTransformer(registry *handlers.Registry, workerCount int, excludeKeys []string, excludeKeysByType map[string][]string) *Transformer {
	return &Transformer{
		registry:          registry,
		workerCount:       workerCount,
		excludeKeys:       excludeKeys,
		excludeKeysByType: excludeKeysByType,
	}
}

// Transform processes fetch results and transforms them
func (t *Transformer) Transform(ctx context.Context, fetchResults <-chan *models.FetchResult) <-chan *models.TransformResult {
	out := make(chan *models.TransformResult)

	go func() {
		defer close(out)

		// Start worker pool
		var wg sync.WaitGroup
		for i := 0; i < t.workerCount; i++ {
			wg.Add(1)
			go t.transformWorker(ctx, fetchResults, out, &wg)
		}

		// Wait for all workers to complete
		wg.Wait()
	}()

	return out
}

// transformWorker processes transformation
func (t *Transformer) transformWorker(ctx context.Context, fetchResults <-chan *models.FetchResult, transformResults chan<- *models.TransformResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for fetchResult := range fetchResults {
		select {
		case <-ctx.Done():
			transformResults <- &models.TransformResult{
				ResourceID: fetchResult.ResourceID,
				Error:      ctx.Err(),
			}
			return
		default:
			// Check if fetch had an error
			if fetchResult.Error != nil {
				transformResults <- &models.TransformResult{
					ResourceID: fetchResult.ResourceID,
					Error:      fetchResult.Error,
				}
				continue
			}

			result := t.transformResource(fetchResult)
			transformResults <- result
		}
	}
}

// transformResource transforms a single resource
func (t *Transformer) transformResource(fetchResult *models.FetchResult) *models.TransformResult {
	log := logger.Default

	log.Debug("Transforming resource",
		"resource_id", fetchResult.ResourceID,
		"type", fetchResult.ResourceType)

	// Get handler for this resource type
	handler, err := t.registry.Get(fetchResult.ResourceType)
	if err != nil {
		log.Error("No handler for resource type during transform",
			"resource_id", fetchResult.ResourceID,
			"type", fetchResult.ResourceType,
			"error", err)
		return &models.TransformResult{
			ResourceID: fetchResult.ResourceID,
			Error:      fmt.Errorf("no handler for resource type %s: %w", fetchResult.ResourceType, err),
		}
	}

	// Transform using handler
	transformed, err := handler.Transform(fetchResult.RawData)
	if err != nil {
		log.Error("Failed to transform resource",
			"resource_id", fetchResult.ResourceID,
			"error", err)
		return &models.TransformResult{
			ResourceID: fetchResult.ResourceID,
			Error:      fmt.Errorf("failed to transform resource: %w", err),
		}
	}

	// Apply additional transformations
	// Get type-specific exclude keys if available
	typeSpecificKeys := []string{}
	if t.excludeKeysByType != nil {
		if keys, ok := t.excludeKeysByType[fetchResult.ResourceType]; ok {
			typeSpecificKeys = keys
		}
	}

	cleanedData := transform.CleanProperties(transformed.Properties, t.excludeKeys, typeSpecificKeys)
	resolvedData := azure.ResolveIDsInProperties(cleanedData)
	sanitizedName := transform.SanitizeFileName(transformed.DisplayName)

	// Generate Terraform import statement
	terraformImport := transform.GenerateTerraformImport(
		handler.GetTerraformResourceType(),
		sanitizedName,
		fetchResult.ResourceID,
	)

	log.Debug("Resource transformed successfully",
		"resource_id", fetchResult.ResourceID,
		"name", transformed.DisplayName,
		"sanitized_name", sanitizedName)

	return &models.TransformResult{
		ResourceID:      fetchResult.ResourceID,
		ResourceType:    fetchResult.ResourceType,
		DisplayName:     transformed.DisplayName,
		SanitizedName:   sanitizedName,
		CleanedData:     resolvedData,
		TerraformImport: terraformImport,
		Error:           nil,
	}
}
