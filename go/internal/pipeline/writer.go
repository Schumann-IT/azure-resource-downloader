package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	// usedNames tracks the file base names already claimed within each resource
	// type (keyed "<type>/<name>") so two resources whose display names sanitize
	// to the same string never overwrite each other on disk (or collapse into a
	// single metadata entry). Guarded by mu.
	usedNames map[string]bool
	mu        sync.Mutex
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
		usedNames:       make(map[string]bool),
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

// Write processes transform results and writes them to disk.
//
// Unlike a purely streaming stage, the writer first drains all of its input —
// emitting pass-through results (cancelled/skipped/filtered/errored) as they
// arrive and buffering the resources that will produce a file — then assigns
// every buffered resource a file name in a deterministic order before writing
// the files concurrently. This is what makes the output reproducible: which of
// several resources whose display names collide keeps the bare file name is
// decided by resource id (see planFileNames), never by the non-deterministic
// order the concurrent upstream stages deliver results.
func (w *Writer) Write(ctx context.Context, transformResults <-chan *models.TransformResult) <-chan *models.WriteResult {
	out := make(chan *models.WriteResult)

	go func() {
		defer close(out)

		// Phase 1: drain the input completely. Every request must still yield
		// exactly one result, so pass-through statuses are emitted here and the
		// writable resources are buffered. Draining fully (never returning early
		// on ctx.Done) preserves the one-result-per-request invariant.
		var writable []*models.TransformResult
		for tr := range transformResults {
			switch {
			case ctx.Err() != nil:
				out <- &models.WriteResult{ResourceID: tr.ResourceID, ResourceType: tr.ResourceType, Cancelled: true, Error: ctx.Err()}
			case tr.Cancelled:
				out <- &models.WriteResult{ResourceID: tr.ResourceID, ResourceType: tr.ResourceType, Cancelled: true, Error: tr.Error}
			case tr.Skipped:
				out <- &models.WriteResult{ResourceID: tr.ResourceID, ResourceType: tr.ResourceType, Skipped: true, SkipReason: tr.SkipReason}
			case tr.Filtered:
				out <- &models.WriteResult{ResourceID: tr.ResourceID, ResourceType: tr.ResourceType, Filtered: true}
			case tr.Error != nil:
				out <- &models.WriteResult{ResourceID: tr.ResourceID, ResourceType: tr.ResourceType, Error: tr.Error}
			default:
				writable = append(writable, tr)
			}
		}

		// If the run was cancelled, nothing buffered is written; account for
		// each remaining request with a cancelled result so the invariant holds.
		if ctx.Err() != nil {
			for _, tr := range writable {
				out <- &models.WriteResult{ResourceID: tr.ResourceID, ResourceType: tr.ResourceType, Cancelled: true, Error: ctx.Err()}
			}
			return
		}

		// Phase 2: assign a unique file name to each resource in a deterministic
		// order, then write the files concurrently. Names are fixed before any
		// write, so the concurrent write order no longer affects the result.
		planned := w.planFileNames(writable)

		var wg sync.WaitGroup
		work := make(chan plannedWrite)
		for i := 0; i < w.workerCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for p := range work {
					out <- w.writeResource(p.result, p.fileName)
				}
			}()
		}
		for _, p := range planned {
			work <- p
		}
		close(work)
		wg.Wait()

		// Phase 3: write the documentation prompt file per resource type (opt-in)
		if w.writePrompts {
			w.writePromptFiles()
		}
	}()

	return out
}

// plannedWrite pairs a resource with the unique file base name reserved for it.
type plannedWrite struct {
	result   *models.TransformResult
	fileName string
}

// planFileNames reserves a unique file base name for every writable resource in
// a deterministic order: resources are sorted by (type, sanitized name,
// resource id) before names are assigned, so when several display names
// sanitize to the same string the one that keeps the bare name is always the
// lowest resource id — never whichever the concurrent upstream stages happened
// to deliver first. Reservation itself is done by reserveFileName.
func (w *Writer) planFileNames(writable []*models.TransformResult) []plannedWrite {
	sort.Slice(writable, func(i, j int) bool {
		a, b := writable[i], writable[j]
		if a.ResourceType != b.ResourceType {
			return a.ResourceType < b.ResourceType
		}
		if a.SanitizedName != b.SanitizedName {
			return a.SanitizedName < b.SanitizedName
		}
		return a.ResourceID < b.ResourceID
	})

	planned := make([]plannedWrite, len(writable))
	for i, tr := range writable {
		planned[i] = plannedWrite{
			result:   tr,
			fileName: w.reserveFileName(tr.ResourceType, tr.SanitizedName, tr.ResourceID),
		}
	}
	return planned
}

// writeResource writes a single resource to disk using the pre-assigned,
// collision-free file base name from planFileNames.
func (w *Writer) writeResource(transformResult *models.TransformResult, fileName string) *models.WriteResult {
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
	yamlPath := filepath.Join(resourceTypeDir, fileName+".yaml")
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

// reserveFileName returns a file base name (no extension) for a resource that
// is unique within its resource type. The first resource to claim a sanitized
// name keeps it; any later resource whose name sanitizes to the same string is
// given a deterministic discriminator derived from its resource ID, so colliding
// resources are written to distinct files instead of silently overwriting one
// another. It never overwrites: the returned name is guaranteed unused so far.
func (w *Writer) reserveFileName(resourceType, sanitizedName, resourceID string) string {
	w.mu.Lock()
	defer w.mu.Unlock()

	key := func(name string) string { return resourceType + "/" + name }

	if !w.usedNames[key(sanitizedName)] {
		w.usedNames[key(sanitizedName)] = true
		return sanitizedName
	}

	// Collision: append a stable discriminator. A numeric counter guards the
	// (astronomically unlikely) case that the discriminated name is also taken,
	// e.g. two resources sharing both a sanitized name and a resource ID.
	base := sanitizedName + "_" + nameDiscriminator(resourceID)
	candidate := base
	for i := 2; w.usedNames[key(candidate)]; i++ {
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	w.usedNames[key(candidate)] = true

	logger.Default.Warn("Resource file name collides with another resource of the same type; writing to a disambiguated name to avoid overwriting",
		"type", resourceType,
		"name", sanitizedName,
		"resolved_name", candidate,
		"resource_id", resourceID)

	return candidate
}

// nameDiscriminator derives a short, stable, filesystem-safe token from a
// resource ID, used to disambiguate colliding file names. It is a prefix of the
// SHA-256 of the ID, so it is deterministic for a given resource and does not
// depend on the (non-deterministic) order resources are written.
func nameDiscriminator(resourceID string) string {
	sum := sha256.Sum256([]byte(resourceID))
	return hex.EncodeToString(sum[:])[:8]
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
		GroupTypes:        stringSliceFromData(tr.CleanedData, "groupTypes"),
		SecurityEnabled:   boolPtrFromData(tr.CleanedData, "securityEnabled"),
	}
}

// stringSliceFromData returns the string elements at key in the cleaned data,
// or nil when the key is absent or not a slice of strings. Non-string elements
// are skipped so a malformed value never panics.
func stringSliceFromData(data map[string]interface{}, key string) []string {
	if data == nil {
		return nil
	}
	raw, ok := data[key].([]interface{})
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// boolPtrFromData returns a pointer to the bool value at key in the cleaned
// data, or nil when the key is absent or not a bool. The pointer distinguishes
// "false" from "not a group / not present".
func boolPtrFromData(data map[string]interface{}, key string) *bool {
	if data == nil {
		return nil
	}
	if v, ok := data[key].(bool); ok {
		return &v
	}
	return nil
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
