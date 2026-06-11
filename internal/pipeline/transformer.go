package pipeline

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/handlers"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"azure-resource-downloader/internal/transform"
)

// Transformer handles transforming resources
type Transformer struct {
	registry           *handlers.Registry
	workerCount        int
	transformerConfigs []models.TransformerConfig
}

// NewTransformer creates a new transformer
func NewTransformer(registry *handlers.Registry, workerCount int, transformerConfigs []models.TransformerConfig) *Transformer {
	return &Transformer{
		registry:           registry,
		workerCount:        workerCount,
		transformerConfigs: transformerConfigs,
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

	// Apply transformations based on configuration
	processedData := transformed.Properties

	// Step 1: Clean properties (remove excluded keys)
	if cleaningConfig := models.GetTransformerConfig(t.transformerConfigs, models.TransformerCleaning); cleaningConfig != nil {
		config := models.ParseCleaningConfig(cleaningConfig.Config)

		// Get type-specific keys for this resource type
		normalizedType := strings.ToLower(fetchResult.ResourceType)
		typeSpecificKeys := config.RemoveKeysByType[normalizedType]

		processedData = transform.CleanPropertiesWithReplace(processedData, config.RemoveKeys, typeSpecificKeys, config.PreserveKeys, config.Replace, config.CleanEmpty)
	} else {
		log.Debug("Skipping cleaning transformer (not configured)")
	}

	// Step 2: Resolve Azure resource IDs to names
	if models.HasTransformer(t.transformerConfigs, models.TransformerIDResolution) {
		processedData = azure.ResolveIDsInProperties(processedData)
	} else {
		log.Debug("Skipping id-resolution transformer (not configured)")
	}

	// Step 2b: Decode base64 payloads (inline replacement or sidecar file)
	var artifacts []models.FileArtifact
	if base64Config := models.GetTransformerConfig(t.transformerConfigs, models.TransformerBase64Decode); base64Config != nil {
		config := models.ParseBase64DecodeConfig(base64Config.Config)
		decoded, err := transform.ApplyBase64Decode(processedData, config)
		if err != nil {
			log.Error("Failed to decode base64 payload",
				"resource_id", fetchResult.ResourceID,
				"error", err)
			return &models.TransformResult{
				ResourceID: fetchResult.ResourceID,
				Error:      fmt.Errorf("failed to decode base64 payload: %w", err),
			}
		}
		artifacts = append(artifacts, decoded...)
	} else {
		log.Debug("Skipping base64-decode transformer (not configured)")
	}

	// Step 3: Sanitize name for file/Terraform compatibility
	var sanitizedName string
	if models.HasTransformer(t.transformerConfigs, models.TransformerNameSanitization) {
		sanitizedName = transform.SanitizeFileName(transformed.DisplayName)
	} else {
		sanitizedName = transformed.DisplayName
		log.Debug("Skipping name-sanitization transformer (not configured)",
			"using_original_name", sanitizedName)
	}

	terraformResourceType := handler.GetTerraformResourceType()

	// Step 4: Generate Terraform import block
	var terraformImport string
	if importConfig := models.GetTransformerConfig(t.transformerConfigs, models.TransformerTerraformImport); importConfig != nil {
		config := models.ParseTerraformImportConfig(importConfig.Config)
		terraformImport = transform.GenerateTerraformImportBlock(
			terraformResourceType,
			sanitizedName,
			fetchResult.ResourceID,
			config.TargetFormat,
		)
	} else {
		log.Debug("Skipping terraform-import transformer (not configured)")
	}

	// Build list of active transformers for logging
	activeTransformers := []string{}
	for _, tc := range t.transformerConfigs {
		activeTransformers = append(activeTransformers, tc.Name)
	}

	log.Debug("Resource transformed successfully",
		"resource_id", fetchResult.ResourceID,
		"name", transformed.DisplayName,
		"sanitized_name", sanitizedName,
		"transformers", strings.Join(activeTransformers, ", "))

	return &models.TransformResult{
		ResourceID:            fetchResult.ResourceID,
		ResourceType:          fetchResult.ResourceType,
		DisplayName:           transformed.DisplayName,
		SanitizedName:         sanitizedName,
		CleanedData:           processedData,
		TerraformImport:       terraformImport,
		TerraformResourceType: terraformResourceType,
		Artifacts:             artifacts,
		Error:                 nil,
	}
}
