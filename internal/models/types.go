package models

import (
	"context"
	"strings"
	"time"
)

// ResourcesDirName is the single subdirectory under the per-tenant output
// directory that azure-rd writes exclusively. Every downloaded YAML, sidecar
// artifact, doc-prompt.md and the metadata.yaml live under it. That ownership
// boundary is what makes --prune safe to reason about.
const ResourcesDirName = "resources"

// FetchRequest represents a request to fetch Azure resources
type FetchRequest struct {
	ResourceID    string
	ResourceType  string
	Subscription  string
	ResourceGroup string
}

// SkippedType describes a resource type that could not be listed at all (e.g.
// missing permissions or no subscription). Because listing failed, the number
// of resources of this type is unknown and none of them were downloaded.
type SkippedType struct {
	ResourceType string
	Reason       string
}

// FetchResult represents the result of fetching a resource
type FetchResult struct {
	ResourceID   string
	ResourceType string
	RawData      interface{}
	Error        error
	// Skipped marks a resource the signed-in user is not permitted to read.
	// Skipped resources are reported as warnings and are neither counted as
	// failures nor written to disk.
	Skipped    bool
	SkipReason string
	// Cancelled marks a request that produced no work because the pipeline was
	// cancelled (e.g. a per-run deadline or interrupt) before it was processed.
	// It exists so every request accounts for exactly one result, which the
	// completeness invariant depends on.
	Cancelled bool
}

// TransformResult represents the result of transforming a resource
type TransformResult struct {
	ResourceID          string
	ResourceType        string
	DisplayName         string
	SanitizedName       string
	CleanedData         map[string]interface{}
	DocumentationPrompt string         // LLM prompt for generating documentation for this resource type
	Artifacts           []FileArtifact // Extra sidecar files to write alongside the YAML (e.g., decoded payloads)
	Error               error
	// Skipped is propagated from the fetch stage for resources the signed-in
	// user is not permitted to read.
	Skipped    bool
	SkipReason string
	// Filtered marks a resource that was excluded by a configured resource
	// filter. Filtered resources are neither written nor counted as failures.
	Filtered bool
	// Cancelled is propagated from the fetch stage (see FetchResult.Cancelled).
	Cancelled bool
}

// FileArtifact represents an additional file to be written alongside a
// resource's YAML document (e.g., a base64-decoded payload).
type FileArtifact struct {
	Filename string // File name relative to the resource type output directory
	Content  []byte // Raw file content
}

// WriteResult represents the result of writing files
type WriteResult struct {
	ResourceID   string
	ResourceType string
	YAMLPath     string
	Error        error
	// Skipped is propagated from earlier stages for resources the signed-in
	// user is not permitted to read (no file is written).
	Skipped    bool
	SkipReason string
	// Filtered is propagated from the transform stage for resources excluded by
	// a configured resource filter (no file is written).
	Filtered bool
	// Cancelled is propagated from earlier stages (see FetchResult.Cancelled).
	Cancelled bool
	// Facts carries the immutable, observed facts about a successfully written
	// resource for resources/metadata.yaml. It is nil for results that wrote no
	// file (skipped, filtered, cancelled, errored, or dry-run).
	Facts *ResourceFacts
}

// ResourceFacts captures the immutable facts about a single written resource,
// computed in the pipeline at write time (the only place the marshalled YAML
// bytes exist) and recorded in resources/metadata.yaml. Facts describe what a
// resource is, never how it should be classified.
type ResourceFacts struct {
	// SourceSha256 is the hex-encoded SHA-256 of the YAML bytes written to disk.
	SourceSha256 string
	// ResourceID is the Azure resource id, letting a later diff distinguish a
	// genuine edit from a regenerated id.
	ResourceID string
	// DisplayName comes from the handler's Transform (not always recoverable
	// from the YAML on disk).
	DisplayName string
	// ODataType, Platforms and Technologies are best-effort lookups from the
	// cleaned data; treat as nullable, never as a primary key.
	ODataType    string
	Platforms    string
	Technologies string
	// Artifacts lists the sidecar file names written alongside the YAML.
	Artifacts []string
	// AssignmentTargets holds the raw, per-resource assignment targets as read
	// from the cleaned data. Resolving GUIDs to names is a cross-resource join
	// that belongs to a later post-processing step.
	AssignmentTargets []interface{}
	// GroupTypes and SecurityEnabled are group-only facts (empty/nil for every
	// other type), read raw from the group YAML so a later step can resolve a
	// referenced group's kind without re-opening its file. They are recorded as
	// plain values, never as a rendered "dynamic security group" string, which
	// is a decision the renderer makes.
	GroupTypes      []string
	SecurityEnabled *bool
}

// TransformedResource represents a fully transformed Azure resource
type TransformedResource struct {
	ID            string
	Type          string
	Name          string
	DisplayName   string
	SanitizedName string
	Properties    map[string]interface{}
}

// ResourceHandler defines the interface for handling specific Azure resource types
type ResourceHandler interface {
	// GetType returns the Azure resource type (e.g., "Microsoft.Storage/storageAccounts")
	GetType() string

	// List enumerates all resource IDs of this handler's type within the
	// current scope (subscription for ARM types, tenant for Microsoft Graph
	// types). It is used to expand a "--type" download into individual fetches.
	List(ctx context.Context) ([]string, error)

	// Fetch retrieves the resource from Azure
	Fetch(ctx context.Context, resourceID string) (interface{}, error)

	// Transform converts the raw resource into a cleaned, transformed version
	Transform(resource interface{}) (*TransformedResource, error)

	// GetDocumentationPrompt returns an LLM prompt that, given a resource of
	// this type, instructs a model to generate end-user documentation covering
	// every setting with best-practice guidance, Microsoft documentation links,
	// and fully expanded embedded payloads (e.g. configurationXml).
	GetDocumentationPrompt() string
}

// PermissionScoped is optionally implemented by a ResourceHandler whose
// resource type needs delegated Microsoft Graph permissions that the Azure CLI
// first-party app cannot provide, and therefore requires signing in to a
// dedicated app registration (--client-id/--tenant-id, device-code flow).
//
// Handlers that do not implement it are assumed to work with the default Azure
// CLI credentials (e.g. ARM types authorised via subscription RBAC). Callers
// use RequiresDedicatedApp to decide, before authenticating, whether they must
// obtain a dedicated app registration for the selected resource types.
type PermissionScoped interface {
	// RequiresDedicatedApp reports whether this type can only be read with a
	// dedicated app registration.
	RequiresDedicatedApp() bool

	// RequiredPermissions lists the delegated permissions/scopes the type needs,
	// for user-facing messages. It may be empty.
	RequiredPermissions() []string
}

// PipelineConfig holds configuration for the pipeline
type PipelineConfig struct {
	OutputDir          string
	WorkerCount        int
	Timeout            time.Duration
	DryRun             bool
	SubscriptionID     string
	TransformerConfigs []TransformerConfig // List of transformer configurations
	ResourceFilters    []ResourceFilter    // Per-resource-type property regex filters
	WritePrompts       bool                // If true, write a per-type documentation LLM prompt file (<type>.prompt.md)
}

// TransformerType represents a transformer step name
type TransformerType string

const (
	TransformerCleaning         TransformerType = "cleaning"
	TransformerIDResolution     TransformerType = "id-resolution"
	TransformerNameSanitization TransformerType = "name-sanitization"
	TransformerBase64Decode     TransformerType = "base64-decode"
)

// TransformerConfig holds configuration for a single transformer
type TransformerConfig struct {
	Name   string                 // Transformer name (e.g., "cleaning", "id-resolution")
	Config map[string]interface{} // Transformer-specific configuration
}

// CleaningConfig holds configuration for the cleaning transformer
type CleaningConfig struct {
	RemoveKeys       []string            // Global keys to remove from all resources
	RemoveKeysByType map[string][]string // Resource-type-specific keys to remove
	PreserveKeys     []string            // Keys to preserve even if in remove lists (takes precedence)
	Replace          []KeyReplacement    // Replace nested values (e.g., replace object with a specific field)
	CleanEmpty       bool                // If true, remove keys with empty values (null, [], "", {}). Default: true
}

// KeyReplacement defines a value replacement operation
type KeyReplacement struct {
	From string // Source path (e.g., "grantControls.authenticationStrength.displayName")
	To   string // Destination path (e.g., "grantControls.authenticationStrength")
}

// Base64 decode transformer modes
const (
	// Base64ModeInline replaces the encoded property value with the decoded text in the YAML output
	Base64ModeInline = "inline"
	// Base64ModeFile writes the decoded value to a sidecar file alongside the YAML
	Base64ModeFile = "file"
)

// Base64DecodeConfig holds configuration for the base64-decode transformer
type Base64DecodeConfig struct {
	Mode         string // Decode mode: "inline" (default) or "file"
	SourceKey    string // Property key holding the base64-encoded value (default: "payload")
	FilenameKey  string // Property key holding the target file name, file mode only (default: "payloadFileName")
	Extension    string // Extension for the decoded file, file mode only (default: ".mobileconfig")
	RemoveSource bool   // File mode only: if true, remove the source (encoded) key from the YAML output (default: false)
}

// DefaultTransformerConfigs returns the default transformer configurations
func DefaultTransformerConfigs() []TransformerConfig {
	return []TransformerConfig{
		{Name: string(TransformerCleaning), Config: map[string]interface{}{}},
		{Name: string(TransformerIDResolution), Config: map[string]interface{}{}},
		{Name: string(TransformerNameSanitization), Config: map[string]interface{}{}},
		{Name: string(TransformerBase64Decode), Config: map[string]interface{}{}},
	}
}

// GetTransformerConfig finds a transformer config by name
func GetTransformerConfig(configs []TransformerConfig, name TransformerType) *TransformerConfig {
	for i := range configs {
		if strings.EqualFold(configs[i].Name, string(name)) {
			return &configs[i]
		}
	}
	return nil
}

// HasTransformer checks if a transformer is configured
func HasTransformer(configs []TransformerConfig, name TransformerType) bool {
	return GetTransformerConfig(configs, name) != nil
}

// ParseCleaningConfig extracts cleaning configuration
func ParseCleaningConfig(config map[string]interface{}) *CleaningConfig {
	result := &CleaningConfig{
		RemoveKeys:       []string{},
		RemoveKeysByType: make(map[string][]string),
		PreserveKeys:     []string{},
		CleanEmpty:       true, // Default: remove empty values (maintains original behavior)
	}

	// Parse remove-keys
	if removeKeys, ok := config["remove-keys"].([]interface{}); ok {
		for _, key := range removeKeys {
			if strKey, ok := key.(string); ok {
				result.RemoveKeys = append(result.RemoveKeys, strKey)
			}
		}
	}

	// Parse remove-keys-by-type
	if removeKeysByType, ok := config["remove-keys-by-type"].(map[string]interface{}); ok {
		for resourceType, keys := range removeKeysByType {
			if keyList, ok := keys.([]interface{}); ok {
				strKeys := []string{}
				for _, k := range keyList {
					if strKey, ok := k.(string); ok {
						strKeys = append(strKeys, strKey)
					}
				}
				result.RemoveKeysByType[strings.ToLower(resourceType)] = strKeys
			}
		}
	}

	// Parse preserve-keys
	if preserveKeys, ok := config["preserve-keys"].([]interface{}); ok {
		for _, key := range preserveKeys {
			if strKey, ok := key.(string); ok {
				result.PreserveKeys = append(result.PreserveKeys, strKey)
			}
		}
	}

	// Parse replace
	if replace, ok := config["replace"].([]interface{}); ok {
		for _, item := range replace {
			if replaceMap, ok := item.(map[string]interface{}); ok {
				from, _ := replaceMap["from"].(string)
				to, _ := replaceMap["to"].(string)
				if from != "" && to != "" {
					result.Replace = append(result.Replace, KeyReplacement{
						From: from,
						To:   to,
					})
				}
			}
		}
	}

	if cleanEmpty, ok := config["clean-empty"].(bool); ok {
		result.CleanEmpty = cleanEmpty
	}

	return result
}

// ParseBase64DecodeConfig extracts base64-decode configuration
func ParseBase64DecodeConfig(config map[string]interface{}) *Base64DecodeConfig {
	result := &Base64DecodeConfig{
		Mode:         Base64ModeInline,
		SourceKey:    "payload",
		FilenameKey:  "payloadFileName",
		Extension:    ".mobileconfig",
		RemoveSource: false,
	}

	if mode, ok := config["mode"].(string); ok && strings.EqualFold(mode, Base64ModeFile) {
		result.Mode = Base64ModeFile
	}
	if sourceKey, ok := config["source-key"].(string); ok && sourceKey != "" {
		result.SourceKey = sourceKey
	}
	if filenameKey, ok := config["filename-key"].(string); ok && filenameKey != "" {
		result.FilenameKey = filenameKey
	}
	if extension, ok := config["extension"].(string); ok && extension != "" {
		result.Extension = extension
	}
	if removeSource, ok := config["remove-source"].(bool); ok {
		result.RemoveSource = removeSource
	}

	return result
}

// WorkerConfig holds worker count configuration
type WorkerConfig struct {
	Default              int            // Default worker count (fallback)
	MicrosoftGraph       int            // Workers for Microsoft Graph API
	AzureResourceManager int            // Workers for Azure Resource Manager API
	ByAPI                map[string]int // Custom per-API overrides
}
