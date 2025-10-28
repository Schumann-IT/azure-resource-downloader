package models

import (
	"context"
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
	ResourceID      string
	ResourceType    string
	DisplayName     string
	SanitizedName   string
	CleanedData     map[string]interface{}
	TerraformImport string
	Error           error
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
	OutputDir         string
	WorkerCount       int
	Timeout           time.Duration
	DryRun            bool
	SubscriptionID    string
	ExcludeKeys       []string            // Global keys to exclude from all resources
	ExcludeKeysByType map[string][]string // Resource-type-specific keys to exclude
}
