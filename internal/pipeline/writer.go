package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"gopkg.in/yaml.v3"
)

// Writer handles writing resources to disk
type Writer struct {
	outputDir   string
	workerCount int
	dryRun      bool
	// importsByType collects import statements grouped by resource type
	importsByType map[string][]importStatement
	mu            sync.Mutex
}

// importStatement holds information for a single import block
type importStatement struct {
	DisplayName     string
	TerraformImport string
}

// NewWriter creates a new writer
func NewWriter(outputDir string, workerCount int, dryRun bool) *Writer {
	return &Writer{
		outputDir:     outputDir,
		workerCount:   workerCount,
		dryRun:        dryRun,
		importsByType: make(map[string][]importStatement),
	}
}

// Write processes transform results and writes them to disk
func (w *Writer) Write(ctx context.Context, transformResults <-chan *models.TransformResult) <-chan *models.WriteResult {
	out := make(chan *models.WriteResult)

	go func() {
		defer close(out)

		// Start worker pool
		var wg sync.WaitGroup
		for i := 0; i < w.workerCount; i++ {
			wg.Add(1)
			go w.writeWorker(ctx, transformResults, out, &wg)
		}

		// Wait for all workers to complete
		wg.Wait()

		// Write consolidated import.tf files per resource type
		w.writeImportFiles(out)
	}()

	return out
}

// writeWorker processes write operations
func (w *Writer) writeWorker(ctx context.Context, transformResults <-chan *models.TransformResult, writeResults chan<- *models.WriteResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for transformResult := range transformResults {
		select {
		case <-ctx.Done():
			writeResults <- &models.WriteResult{
				ResourceID: transformResult.ResourceID,
				Error:      ctx.Err(),
			}
			return
		default:
			// Propagate resources the user was not permitted to read; nothing
			// is written for them.
			if transformResult.Skipped {
				writeResults <- &models.WriteResult{
					ResourceID: transformResult.ResourceID,
					Skipped:    true,
					SkipReason: transformResult.SkipReason,
				}
				continue
			}

			// Propagate resources excluded by a configured filter; nothing is
			// written for them.
			if transformResult.Filtered {
				writeResults <- &models.WriteResult{
					ResourceID: transformResult.ResourceID,
					Filtered:   true,
				}
				continue
			}

			// Check if transform had an error
			if transformResult.Error != nil {
				writeResults <- &models.WriteResult{
					ResourceID: transformResult.ResourceID,
					Error:      transformResult.Error,
				}
				continue
			}

			result := w.writeResource(transformResult)
			writeResults <- result
		}
	}
}

// writeResource writes a single resource to disk
func (w *Writer) writeResource(transformResult *models.TransformResult) *models.WriteResult {
	log := logger.Default

	log.Debug("Writing resource files",
		"resource_id", transformResult.ResourceID,
		"name", transformResult.DisplayName,
		"type", transformResult.ResourceType)

	// Create resource type directory
	resourceTypeDir := filepath.Join(w.outputDir, transformResult.ResourceType)

	if !w.dryRun {
		if err := os.MkdirAll(resourceTypeDir, 0755); err != nil {
			log.Error("Failed to create directory",
				"resource_id", transformResult.ResourceID,
				"directory", resourceTypeDir,
				"error", err)
			return &models.WriteResult{
				ResourceID: transformResult.ResourceID,
				Error:      fmt.Errorf("failed to create directory: %w", err),
			}
		}
	}

	// Write YAML file
	yamlPath := filepath.Join(resourceTypeDir, transformResult.SanitizedName+".yaml")
	if !w.dryRun {
		yamlData, err := yaml.Marshal(transformResult.CleanedData)
		if err != nil {
			log.Error("Failed to marshal YAML",
				"resource_id", transformResult.ResourceID,
				"error", err)
			return &models.WriteResult{
				ResourceID: transformResult.ResourceID,
				Error:      fmt.Errorf("failed to marshal YAML: %w", err),
			}
		}

		if err := os.WriteFile(yamlPath, yamlData, 0644); err != nil {
			log.Error("Failed to write YAML file",
				"resource_id", transformResult.ResourceID,
				"path", yamlPath,
				"error", err)
			return &models.WriteResult{
				ResourceID: transformResult.ResourceID,
				Error:      fmt.Errorf("failed to write YAML file: %w", err),
			}
		}
	}

	// Write sidecar artifacts (e.g. base64-decoded payloads) alongside the YAML
	for _, artifact := range transformResult.Artifacts {
		if artifact.Filename == "" {
			continue
		}
		artifactPath := filepath.Join(resourceTypeDir, artifact.Filename)

		if w.dryRun {
			log.Info("Would write artifact",
				"resource_id", transformResult.ResourceID,
				"path", artifactPath,
				"bytes", len(artifact.Content))
			continue
		}

		if err := os.WriteFile(artifactPath, artifact.Content, 0644); err != nil {
			log.Error("Failed to write artifact file",
				"resource_id", transformResult.ResourceID,
				"path", artifactPath,
				"error", err)
			return &models.WriteResult{
				ResourceID: transformResult.ResourceID,
				Error:      fmt.Errorf("failed to write artifact file: %w", err),
			}
		}

		log.Debug("Artifact file written",
			"resource_id", transformResult.ResourceID,
			"path", artifactPath,
			"bytes", len(artifact.Content))
	}

	// Collect import statement for consolidated import.tf file
	terraformPath := filepath.Join(resourceTypeDir, "import.tf")
	w.mu.Lock()
	w.importsByType[transformResult.ResourceType] = append(
		w.importsByType[transformResult.ResourceType],
		importStatement{
			DisplayName:     transformResult.DisplayName,
			TerraformImport: transformResult.TerraformImport,
		},
	)
	w.mu.Unlock()

	log.Debug("Resource files written successfully",
		"resource_id", transformResult.ResourceID,
		"yaml_path", yamlPath,
		"terraform_path", terraformPath)

	return &models.WriteResult{
		ResourceID:    transformResult.ResourceID,
		YAMLPath:      yamlPath,
		TerraformPath: terraformPath,
		Error:         nil,
	}
}

// writeImportFiles writes consolidated import.tf files for each resource type
func (w *Writer) writeImportFiles(writeResults chan<- *models.WriteResult) {
	log := logger.Default

	for resourceType, imports := range w.importsByType {
		// Skip if no imports (e.g., terraform-import transformer disabled)
		if len(imports) == 0 {
			log.Debug("Skipping import file (no imports)",
				"resource_type", resourceType)
			continue
		}

		// Filter out empty import statements
		validImports := []importStatement{}
		for _, imp := range imports {
			if imp.TerraformImport != "" {
				validImports = append(validImports, imp)
			}
		}

		// Skip if no valid imports after filtering
		if len(validImports) == 0 {
			log.Debug("Skipping import file (no valid imports after filtering)",
				"resource_type", resourceType,
				"total_imports", len(imports))
			continue
		}

		resourceTypeDir := filepath.Join(w.outputDir, resourceType)
		terraformPath := filepath.Join(resourceTypeDir, "import.tf")

		if w.dryRun {
			log.Info("Would write import file",
				"resource_type", resourceType,
				"path", terraformPath,
				"import_count", len(validImports))
			continue
		}

		// Build the consolidated import file content
		var content strings.Builder
		content.WriteString("# Terraform import statements\n")
		content.WriteString("# Generated by azure-resource-downloader\n\n")

		for _, imp := range validImports {
			fmt.Fprintf(&content, "# Import for %s\n", imp.DisplayName)
			content.WriteString(imp.TerraformImport)
			content.WriteString("\n")
		}

		// Write the import.tf file
		if err := os.WriteFile(terraformPath, []byte(content.String()), 0644); err != nil {
			log.Error("Failed to write consolidated import file",
				"resource_type", resourceType,
				"path", terraformPath,
				"error", err)
		} else {
			log.Info("Consolidated import file written",
				"resource_type", resourceType,
				"path", terraformPath,
				"import_count", len(validImports))
		}
	}
}
