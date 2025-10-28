package models

import (
	"context"
	"strings"
	"time"
)

// FetchRequest represents a request to fetch Azure resources
type FetchRequest struct {
	ResourceID    string
	ResourceType  string
	Subscription  string
	ResourceGroup string
}

// FetchResult represents the result of fetching a resource
type FetchResult struct {
	ResourceID   string
	ResourceType string
	RawData      interface{}
	Error        error
}

// TransformResult represents the result of transforming a resource
type TransformResult struct {
	ResourceID            string
	ResourceType          string
	DisplayName           string
	SanitizedName         string
	CleanedData           map[string]interface{}
	TerraformImport       string
	TerraformResourceType string // The Terraform resource type (e.g., "azurerm_resource_group")
	Error                 error
}

// WriteResult represents the result of writing files
type WriteResult struct {
	ResourceID    string
	YAMLPath      string
	TerraformPath string
	Error         error
}

// TransformedResource represents a fully transformed Azure resource
type TransformedResource struct {
	ID              string
	Type            string
	Name            string
	DisplayName     string
	SanitizedName   string
	Properties      map[string]interface{}
	TerraformImport string
}

// ResourceHandler defines the interface for handling specific Azure resource types
type ResourceHandler interface {
	// GetType returns the Azure resource type (e.g., "Microsoft.Storage/storageAccounts")
	GetType() string

	// Fetch retrieves the resource from Azure
	Fetch(ctx context.Context, resourceID string) (interface{}, error)

	// Transform converts the raw resource into a cleaned, transformed version
	Transform(resource interface{}) (*TransformedResource, error)

	// GetTerraformResourceType returns the Terraform resource type (e.g., "azurerm_storage_account")
	GetTerraformResourceType() string
}

// PipelineConfig holds configuration for the pipeline
type PipelineConfig struct {
	OutputDir          string
	WorkerCount        int
	Timeout            time.Duration
	DryRun             bool
	SubscriptionID     string
	TransformerConfigs []TransformerConfig // List of transformer configurations
}

// TransformerType represents a transformer step name
type TransformerType string

const (
	TransformerCleaning         TransformerType = "cleaning"
	TransformerIDResolution     TransformerType = "id-resolution"
	TransformerNameSanitization TransformerType = "name-sanitization"
	TransformerTerraformImport  TransformerType = "terraform-import"
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

// TerraformImportConfig holds configuration for the terraform-import transformer
type TerraformImportConfig struct {
	TargetFormat string // Format template for 'to' address
}

// DefaultTransformerConfigs returns the default transformer configurations
func DefaultTransformerConfigs() []TransformerConfig {
	return []TransformerConfig{
		{Name: string(TransformerCleaning), Config: map[string]interface{}{}},
		{Name: string(TransformerIDResolution), Config: map[string]interface{}{}},
		{Name: string(TransformerNameSanitization), Config: map[string]interface{}{}},
		{Name: string(TransformerTerraformImport), Config: map[string]interface{}{}},
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

	// Support both new name (remove-keys) and old name (exclude-keys) for backward compatibility
	removeKeys := config["remove-keys"]
	if removeKeys == nil {
		removeKeys = config["exclude-keys"] // Backward compatibility
	}

	if keys, ok := removeKeys.([]interface{}); ok {
		for _, key := range keys {
			if strKey, ok := key.(string); ok {
				result.RemoveKeys = append(result.RemoveKeys, strKey)
			}
		}
	}

	// Support both new and old names for type-specific keys
	removeKeysByType := config["remove-keys-by-type"]
	if removeKeysByType == nil {
		removeKeysByType = config["exclude-keys-by-type"] // Backward compatibility
	}

	if keysByType, ok := removeKeysByType.(map[string]interface{}); ok {
		for resourceType, keys := range keysByType {
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

// ParseTerraformImportConfig extracts terraform-import configuration
func ParseTerraformImportConfig(config map[string]interface{}) *TerraformImportConfig {
	result := &TerraformImportConfig{
		TargetFormat: "{resource_type}.{name}", // Default
	}

	if targetFormat, ok := config["target-format"].(string); ok && targetFormat != "" {
		result.TargetFormat = targetFormat
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
