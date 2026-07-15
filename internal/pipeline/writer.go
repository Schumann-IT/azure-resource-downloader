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
	outputDir    string
	workerCount  int
	dryRun       bool
	writePrompts bool
	// promptsByType collects the documentation LLM prompt for each resource type
	promptsByType map[string]string
	mu            sync.Mutex
}

// NewWriter creates a new writer. When writePrompts is true, a per-resource-type
// documentation LLM prompt file (doc-prompt.md) is written alongside the YAML.
func NewWriter(outputDir string, workerCount int, dryRun, writePrompts bool) *Writer {
	return &Writer{
		outputDir:     outputDir,
		workerCount:   workerCount,
		dryRun:        dryRun,
		writePrompts:  writePrompts,
		promptsByType: make(map[string]string),
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

		// Write the documentation prompt file per resource type (opt-in)
		if w.writePrompts {
			w.writePromptFiles()
		}
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

	w.mu.Lock()
	if w.writePrompts && transformResult.DocumentationPrompt != "" {
		w.promptsByType[transformResult.ResourceType] = transformResult.DocumentationPrompt
	}
	w.mu.Unlock()

	log.Debug("Resource files written successfully",
		"resource_id", transformResult.ResourceID,
		"yaml_path", yamlPath)

	return &models.WriteResult{
		ResourceID: transformResult.ResourceID,
		YAMLPath:   yamlPath,
		Error:      nil,
	}
}

// writePromptFiles writes one documentation LLM prompt file per resource type.
// The prompt instructs a model to document every setting of a resource of that
// type, including best-practice references, Microsoft documentation links and
// fully expanded embedded payloads (e.g. configurationXml). The file is named
// "doc-prompt.md" inside the resource type directory.
func (w *Writer) writePromptFiles() {
	log := logger.Default

	w.mu.Lock()
	prompts := make(map[string]string, len(w.promptsByType))
	for resourceType, prompt := range w.promptsByType {
		prompts[resourceType] = prompt
	}
	w.mu.Unlock()

	for resourceType, prompt := range prompts {
		if prompt == "" {
			continue
		}

		resourceTypeDir := filepath.Join(w.outputDir, resourceType)
		promptPath := filepath.Join(resourceTypeDir, "doc-prompt.md")

		if w.dryRun {
			log.Info("Would write documentation prompt file",
				"resource_type", resourceType,
				"path", promptPath)
			continue
		}

		if err := os.MkdirAll(resourceTypeDir, 0755); err != nil {
			log.Error("Failed to create directory for documentation prompt",
				"resource_type", resourceType,
				"directory", resourceTypeDir,
				"error", err)
			continue
		}

		var content strings.Builder
		fmt.Fprintf(&content, "# Documentation prompt for %s\n\n", resourceType)
		content.WriteString("<!-- Generated by azure-resource-downloader. ")
		content.WriteString("Paste this prompt together with a resource YAML from this directory into an LLM. -->\n\n")
		content.WriteString(prompt)
		content.WriteString("\n")

		if err := os.WriteFile(promptPath, []byte(content.String()), 0644); err != nil {
			log.Error("Failed to write documentation prompt file",
				"resource_type", resourceType,
				"path", promptPath,
				"error", err)
		} else {
			log.Info("Documentation prompt file written",
				"resource_type", resourceType,
				"path", promptPath)
		}
	}
}
