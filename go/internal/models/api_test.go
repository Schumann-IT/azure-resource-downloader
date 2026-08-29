package models

import (
	"testing"
)

func TestDetectAPIType(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		expected     APIType
	}{
		{
			name:         "Microsoft Graph - Conditional Access",
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			expected:     APIMicrosoftGraph,
		},
		{
			name:         "Microsoft Graph - generic",
			resourceType: "Microsoft.Graph/users",
			expected:     APIMicrosoftGraph,
		},
		{
			name:         "ARM - Storage Account",
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     APIAzureResourceManager,
		},
		{
			name:         "ARM - Virtual Machine",
			resourceType: "Microsoft.Compute/virtualMachines",
			expected:     APIAzureResourceManager,
		},
		{
			name:         "ARM - Resource Group",
			resourceType: "Microsoft.Resources/resourceGroups",
			expected:     APIAzureResourceManager,
		},
		{
			name:         "ARM - Network",
			resourceType: "Microsoft.Network/virtualNetworks",
			expected:     APIAzureResourceManager,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectAPIType(tt.resourceType)
			if result != tt.expected {
				t.Errorf("DetectAPIType(%q) = %v, want %v", tt.resourceType, result, tt.expected)
			}
		})
	}
}

func TestGetAPIConfig(t *testing.T) {
	tests := []struct {
		name                string
		resourceType        string
		expectedAPI         APIType
		expectedRecommended int
		expectedMax         int
	}{
		{
			name:                "Graph API resource",
			resourceType:        "Microsoft.Graph/conditionalAccessPolicies",
			expectedAPI:         APIMicrosoftGraph,
			expectedRecommended: 5,
			expectedMax:         5,
		},
		{
			name:                "ARM resource",
			resourceType:        "Microsoft.Storage/storageAccounts",
			expectedAPI:         APIAzureResourceManager,
			expectedRecommended: 10,
			expectedMax:         20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := GetAPIConfig(tt.resourceType)
			if config == nil {
				t.Fatal("GetAPIConfig() returned nil")
				return
			}
			if config.Name != tt.expectedAPI {
				t.Errorf("API Name = %v, want %v", config.Name, tt.expectedAPI)
			}
			if config.RecommendedWorkers != tt.expectedRecommended {
				t.Errorf("RecommendedWorkers = %d, want %d", config.RecommendedWorkers, tt.expectedRecommended)
			}
			if config.MaxRecommendedWorkers != tt.expectedMax {
				t.Errorf("MaxRecommendedWorkers = %d, want %d", config.MaxRecommendedWorkers, tt.expectedMax)
			}
		})
	}
}

func TestShouldWarnAboutWorkerCount(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		workerCount  int
		shouldWarn   bool
	}{
		{
			name:         "Graph API - acceptable count (5)",
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			workerCount:  5,
			shouldWarn:   false,
		},
		{
			name:         "Graph API - too many (20)",
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			workerCount:  20,
			shouldWarn:   true,
		},
		{
			name:         "ARM - acceptable count (10)",
			resourceType: "Microsoft.Storage/storageAccounts",
			workerCount:  10,
			shouldWarn:   false,
		},
		{
			name:         "ARM - acceptable count (20)",
			resourceType: "Microsoft.Storage/storageAccounts",
			workerCount:  20,
			shouldWarn:   false,
		},
		{
			name:         "ARM - too many (25)",
			resourceType: "Microsoft.Storage/storageAccounts",
			workerCount:  25,
			shouldWarn:   true,
		},
		{
			name:         "Graph API - low count (1)",
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			workerCount:  1,
			shouldWarn:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldWarn, _ := ShouldWarnAboutWorkerCount(tt.resourceType, tt.workerCount)
			if shouldWarn != tt.shouldWarn {
				t.Errorf("ShouldWarnAboutWorkerCount(%q, %d) = %v, want %v",
					tt.resourceType, tt.workerCount, shouldWarn, tt.shouldWarn)
			}
		})
	}
}

func TestDefaultWorkerConfig(t *testing.T) {
	config := DefaultWorkerConfig()

	if config.Default != 5 {
		t.Errorf("Default workers = %d, want 5", config.Default)
	}
	if config.MicrosoftGraph != 5 {
		t.Errorf("MicrosoftGraph workers = %d, want 5", config.MicrosoftGraph)
	}
	if config.AzureResourceManager != 20 {
		t.Errorf("AzureResourceManager workers = %d, want 20", config.AzureResourceManager)
	}
}

func TestWorkerConfig_GetWorkerCount(t *testing.T) {
	tests := []struct {
		name         string
		config       *WorkerConfig
		resourceType string
		expected     int
	}{
		{
			name:         "Graph API - use default",
			config:       DefaultWorkerConfig(),
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			expected:     5,
		},
		{
			name:         "ARM - use default",
			config:       DefaultWorkerConfig(),
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     20,
		},
		{
			name: "Graph API - custom override",
			config: &WorkerConfig{
				Default:              5,
				MicrosoftGraph:       3,
				AzureResourceManager: 20,
			},
			resourceType: "Microsoft.Graph/conditionalAccessPolicies",
			expected:     3,
		},
		{
			name: "ARM - custom override",
			config: &WorkerConfig{
				Default:              5,
				MicrosoftGraph:       5,
				AzureResourceManager: 15,
			},
			resourceType: "Microsoft.Compute/virtualMachines",
			expected:     15,
		},
		{
			name:         "nil config - fallback",
			config:       nil,
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetWorkerCount(tt.resourceType)
			if result != tt.expected {
				t.Errorf("GetWorkerCount(%q) = %d, want %d", tt.resourceType, result, tt.expected)
			}
		})
	}
}
