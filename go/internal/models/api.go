package models

import "strings"

// APIType represents the Azure API being used
type APIType string

const (
	// APIMicrosoftGraph represents Microsoft Graph API (tenant-level resources)
	APIMicrosoftGraph APIType = "Microsoft.Graph"

	// APIAzureResourceManager represents Azure Resource Manager API (subscription-level resources)
	APIAzureResourceManager APIType = "Azure.ResourceManager"
)

// APIConfig holds API-specific configuration
type APIConfig struct {
	Name                  APIType
	RecommendedWorkers    int
	MaxRecommendedWorkers int
	RateLimitInfo         string
}

// GetAPIConfigs returns configuration for all supported APIs
func GetAPIConfigs() map[APIType]*APIConfig {
	return map[APIType]*APIConfig{
		APIMicrosoftGraph: {
			Name:                  APIMicrosoftGraph,
			RecommendedWorkers:    5,
			MaxRecommendedWorkers: 5,
			RateLimitInfo:         "~2000 requests per 300 seconds (~6.67 req/sec)",
		},
		APIAzureResourceManager: {
			Name:                  APIAzureResourceManager,
			RecommendedWorkers:    10,
			MaxRecommendedWorkers: 20,
			RateLimitInfo:         "Generous limits (thousands per minute)",
		},
	}
}

// DetectAPIType determines which API a resource type uses
func DetectAPIType(resourceType string) APIType {
	if strings.HasPrefix(resourceType, "Microsoft.Graph/") {
		return APIMicrosoftGraph
	}
	// All other Microsoft.* resources use ARM
	return APIAzureResourceManager
}

// GetAPIConfig returns the configuration for a given resource type
func GetAPIConfig(resourceType string) *APIConfig {
	apiType := DetectAPIType(resourceType)
	configs := GetAPIConfigs()
	return configs[apiType]
}

// ShouldWarnAboutWorkerCount checks if worker count is too high for the API
func ShouldWarnAboutWorkerCount(resourceType string, workerCount int) (bool, string) {
	config := GetAPIConfig(resourceType)
	if config == nil {
		return false, ""
	}

	if workerCount > config.MaxRecommendedWorkers {
		return true, config.RateLimitInfo
	}

	return false, ""
}

// DefaultWorkerConfig returns the default worker configuration with API-specific defaults
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		Default:              5,  // Safe default
		MicrosoftGraph:       5,  // Strict rate limits
		AzureResourceManager: 20, // Generous limits
		ByAPI:                make(map[string]int),
	}
}

// GetWorkerCount returns the appropriate worker count for a given resource type
func (w *WorkerConfig) GetWorkerCount(resourceType string) int {
	if w == nil {
		return 5 // Fallback
	}

	apiType := DetectAPIType(resourceType)

	// Check for custom per-API override
	if w.ByAPI != nil {
		if count, ok := w.ByAPI[string(apiType)]; ok && count > 0 {
			return count
		}
	}

	// Use API-specific defaults
	switch apiType {
	case APIMicrosoftGraph:
		if w.MicrosoftGraph > 0 {
			return w.MicrosoftGraph
		}
		return 5
	case APIAzureResourceManager:
		if w.AzureResourceManager > 0 {
			return w.AzureResourceManager
		}
		return 20
	default:
		if w.Default > 0 {
			return w.Default
		}
		return 5
	}
}
