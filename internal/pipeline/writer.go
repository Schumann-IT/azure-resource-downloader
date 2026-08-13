package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	// promptSHAByType records the SHA-256 of the assembled doc-prompt.md content
	// per resource type, hashed over the exact bytes written to disk so a later
	// comparison against the file matches.
	promptSHAByType map[string]string
	mu              sync.Mutex
}

// NewWriter creates a new writer. When writePrompts is true, a per-resource-type
// documentation LLM prompt file (doc-prompt.md) is written alongside the YAML.
func NewWriter(outputDir string, workerCount int, dryRun, writePrompts bool) *Writer {
	return &Writer{
		outputDir:       outputDir,
		workerCount:     workerCount,
		dryRun:          dryRun,
		writePrompts:    writePrompts,
		promptsByType:   make(map[string]string),
		promptSHAByType: make(map[string]string),
	}
}

// resourcesDir returns the directory that azure-rd writes exclusively. Every
// YAML, sidecar artifact and doc-prompt.md lives under it.
func (w *Writer) resourcesDir() string {
	return filepath.Join(w.outputDir, models.ResourcesDirName)
}

// PromptSHAByType returns a copy of the per-type doc-prompt.md content hashes
// collected during the run. It is only populated when writePrompts is enabled.
func (w *Writer) PromptSHAByType() map[string]string {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[string]string, len(w.promptSHAByType))
	for k, v := range w.promptSHAByType {
		out[k] = v
	}
	return out
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
		// On cancellation keep draining the input channel, emitting one
		// cancelled result per remaining item so every request still accounts
		// for exactly one result.
		if err := ctx.Err(); err != nil {
			writeResults <- &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Cancelled:    true,
				Error:        err,
			}
			continue
		}

		// Propagate requests cancelled in an earlier stage.
		if transformResult.Cancelled {
			writeResults <- &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Cancelled:    true,
				Error:        transformResult.Error,
			}
			continue
		}

		// Propagate resources the user was not permitted to read; nothing
		// is written for them.
		if transformResult.Skipped {
			writeResults <- &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Skipped:      true,
				SkipReason:   transformResult.SkipReason,
			}
			continue
		}

		// Propagate resources excluded by a configured filter; nothing is
		// written for them.
		if transformResult.Filtered {
			writeResults <- &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Filtered:     true,
			}
			continue
		}

		// Check if transform had an error
		if transformResult.Error != nil {
			writeResults <- &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Error:        transformResult.Error,
			}
			continue
		}

		writeResults <- w.writeResource(transformResult)
	}
}

// writeResource writes a single resource to disk
func (w *Writer) writeResource(transformResult *models.TransformResult) *models.WriteResult {
	log := logger.Default

	log.Debug("Writing resource files",
		"resource_id", transformResult.ResourceID,
		"name", transformResult.DisplayName,
		"type", transformResult.ResourceType)

	// Create resource type directory under resources/, the tree azure-rd owns
	// exclusively.
	resourceTypeDir := filepath.Join(w.resourcesDir(), transformResult.ResourceType)

	if !w.dryRun {
		if err := os.MkdirAll(resourceTypeDir, 0755); err != nil {
			log.Error("Failed to create directory",
				"resource_id", transformResult.ResourceID,
				"directory", resourceTypeDir,
				"error", err)
			return &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Error:        fmt.Errorf("failed to create directory: %w", err),
			}
		}
	}

	// Write YAML file. The marshalled bytes exist only here, so this is also
	// where the source hash for metadata.yaml is computed.
	yamlPath := filepath.Join(resourceTypeDir, transformResult.SanitizedName+".yaml")
	var facts *models.ResourceFacts
	if !w.dryRun {
		yamlData, err := yaml.Marshal(transformResult.CleanedData)
		if err != nil {
			log.Error("Failed to marshal YAML",
				"resource_id", transformResult.ResourceID,
				"error", err)
			return &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Error:        fmt.Errorf("failed to marshal YAML: %w", err),
			}
		}

		if err := os.WriteFile(yamlPath, yamlData, 0644); err != nil {
			log.Error("Failed to write YAML file",
				"resource_id", transformResult.ResourceID,
				"path", yamlPath,
				"error", err)
			return &models.WriteResult{
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Error:        fmt.Errorf("failed to write YAML file: %w", err),
			}
		}

		facts = buildResourceFacts(transformResult, yamlData)
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
				ResourceID:   transformResult.ResourceID,
				ResourceType: transformResult.ResourceType,
				Error:        fmt.Errorf("failed to write artifact file: %w", err),
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
		ResourceID:   transformResult.ResourceID,
		ResourceType: transformResult.ResourceType,
		YAMLPath:     yamlPath,
		Facts:        facts,
		Error:        nil,
	}
}

// buildResourceFacts assembles the immutable facts recorded in metadata.yaml
// for a single successfully written resource. yamlData is the exact byte slice
// written to disk, so its hash matches the file.
func buildResourceFacts(tr *models.TransformResult, yamlData []byte) *models.ResourceFacts {
	sum := sha256.Sum256(yamlData)

	artifactNames := make([]string, 0, len(tr.Artifacts))
	for _, a := range tr.Artifacts {
		if a.Filename != "" {
			artifactNames = append(artifactNames, a.Filename)
		}
	}

	return &models.ResourceFacts{
		SourceSha256:      hex.EncodeToString(sum[:]),
		ResourceID:        tr.ResourceID,
		DisplayName:       tr.DisplayName,
		ODataType:         stringFromData(tr.CleanedData, "@odata.type"),
		Platforms:         stringFromData(tr.CleanedData, "platforms"),
		Technologies:      stringFromData(tr.CleanedData, "technologies"),
		Artifacts:         artifactNames,
		AssignmentTargets: assignmentTargetsFromData(tr.CleanedData),
	}
}

// stringFromData returns the string value at key in the cleaned data, or "" if
// it is absent or not a string. These facts are nullable, never a primary key.
func stringFromData(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

// assignmentTargetsFromData returns the raw assignments slice from the cleaned
// data, or nil when there are none.
func assignmentTargetsFromData(data map[string]interface{}) []interface{} {
	if data == nil {
		return nil
	}
	if v, ok := data["assignments"].([]interface{}); ok && len(v) > 0 {
		return v
	}
	return nil
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

		// Hash the assembled file bytes, not the raw prompt string, so a later
		// comparison against the file on disk matches. This is computed even in
		// dry-run (nothing is written) so callers can still record the hash.
		content := assembleDocPrompt(resourceType, prompt)
		sum := sha256.Sum256([]byte(content))
		w.mu.Lock()
		w.promptSHAByType[resourceType] = hex.EncodeToString(sum[:])
		w.mu.Unlock()

		resourceTypeDir := filepath.Join(w.resourcesDir(), resourceType)
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

		if err := os.WriteFile(promptPath, []byte(content), 0644); err != nil {
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

// assembleDocPrompt builds the exact doc-prompt.md content for a resource type.
// The same assembled string is both written to disk and hashed, so the two can
// never diverge.
func assembleDocPrompt(resourceType, prompt string) string {
	var content strings.Builder
	fmt.Fprintf(&content, "# Documentation prompt for %s\n\n", resourceType)
	content.WriteString("<!-- Generated by azure-resource-downloader. ")
	content.WriteString("Paste this prompt together with a resource YAML from this directory into an LLM. -->\n\n")
	content.WriteString(prompt)
	content.WriteString("\n")
	return content.String()
}
