package handlers

import (
	"context"
	"testing"
	"time"

	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// stubCredential is a no-network azcore.TokenCredential used to exercise
// handler construction in NewRegistry without contacting Azure.
type stubCredential struct{}

func (stubCredential) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "stub", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// MockHandler is a mock implementation of ResourceHandler for testing
type MockHandler struct {
	resourceType string
}

func (m *MockHandler) GetType() string {
	return m.resourceType
}

func (m *MockHandler) List(ctx context.Context) ([]string, error) {
	return []string{}, nil
}

func (m *MockHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	return map[string]interface{}{"id": resourceID}, nil
}

func (m *MockHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	return &models.TransformedResource{
		ID:   "test-id",
		Type: m.resourceType,
		Name: "test-name",
	}, nil
}

func (m *MockHandler) GetDocumentationPrompt() string {
	return models.BuildDocumentationPrompt(models.ResourceDocumentation{
		AzureType: m.resourceType,
	})
}

func TestNewEmptyRegistry(t *testing.T) {
	registry := NewEmptyRegistry()

	if registry == nil {
		t.Fatal("NewEmptyRegistry() returned nil")
		return
	}

	if registry.handlers == nil {
		t.Error("NewEmptyRegistry() did not initialize handlers map")
	}

	if len(registry.handlers) != 0 {
		t.Errorf("NewEmptyRegistry() handlers map should be empty, got %d items", len(registry.handlers))
	}
}

func TestNewRegistryRegistersDefaults(t *testing.T) {
	registry := NewRegistry(stubCredential{}, "sub-123", false)

	if registry == nil {
		t.Fatal("NewRegistry() returned nil")
		return
	}

	// ARM handlers are always registered regardless of credential.
	for _, armType := range []string{
		"Microsoft.Resources/resourceGroups",
		"Microsoft.Storage/storageAccounts",
		"Microsoft.Compute/virtualMachines",
	} {
		if !registry.HasHandler(armType) {
			t.Errorf("NewRegistry() did not register expected ARM type %q", armType)
		}
	}

	// Graph handlers are registered too, so the populated registry must hold
	// substantially more than just the ARM types.
	if got := len(registry.GetAllTypes()); got <= 3 {
		t.Errorf("NewRegistry() registered %d types, want more than the 3 ARM types", got)
	}
}

func TestRegister(t *testing.T) {
	registry := NewEmptyRegistry()
	handler := &MockHandler{
		resourceType: "Microsoft.Storage/storageAccounts",
	}

	registry.Register("Microsoft.Storage/storageAccounts", handler)

	if len(registry.handlers) != 1 {
		t.Errorf("Register() failed, expected 1 handler, got %d", len(registry.handlers))
	}
}

func TestGet(t *testing.T) {
	registry := NewEmptyRegistry()
	handler := &MockHandler{
		resourceType: "Microsoft.Storage/storageAccounts",
	}

	registry.Register("Microsoft.Storage/storageAccounts", handler)

	tests := []struct {
		name         string
		resourceType string
		expectError  bool
	}{
		{
			name:         "existing handler",
			resourceType: "Microsoft.Storage/storageAccounts",
			expectError:  false,
		},
		{
			name:         "non-existing handler",
			resourceType: "Microsoft.Compute/virtualMachines",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := registry.Get(tt.resourceType)

			if tt.expectError {
				if err == nil {
					t.Errorf("Get() expected error for non-existing handler")
				}
				if result != nil {
					t.Errorf("Get() should return nil handler for non-existing type")
				}
			} else {
				if err != nil {
					t.Errorf("Get() unexpected error: %v", err)
				}
				if result == nil {
					t.Errorf("Get() returned nil handler")
				}
				if result.GetType() != tt.resourceType {
					t.Errorf("Get() returned wrong handler type: got %q, want %q", result.GetType(), tt.resourceType)
				}
			}
		})
	}
}

func TestGetAllTypes(t *testing.T) {
	registry := NewEmptyRegistry()

	// Empty registry
	types := registry.GetAllTypes()
	if len(types) != 0 {
		t.Errorf("GetAllTypes() on empty registry should return empty slice, got %d items", len(types))
	}

	// Add handlers
	handler1 := &MockHandler{resourceType: "Microsoft.Storage/storageAccounts"}
	handler2 := &MockHandler{resourceType: "Microsoft.Compute/virtualMachines"}
	handler3 := &MockHandler{resourceType: "Microsoft.Network/virtualNetworks"}

	registry.Register("Microsoft.Storage/storageAccounts", handler1)
	registry.Register("Microsoft.Compute/virtualMachines", handler2)
	registry.Register("Microsoft.Network/virtualNetworks", handler3)

	types = registry.GetAllTypes()
	if len(types) != 3 {
		t.Errorf("GetAllTypes() expected 3 types, got %d", len(types))
	}

	// Verify all types are present
	typeMap := make(map[string]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	expectedTypes := []string{
		"Microsoft.Storage/storageAccounts",
		"Microsoft.Compute/virtualMachines",
		"Microsoft.Network/virtualNetworks",
	}

	for _, expected := range expectedTypes {
		if !typeMap[expected] {
			t.Errorf("GetAllTypes() missing expected type: %q", expected)
		}
	}
}

func TestHasHandler(t *testing.T) {
	registry := NewEmptyRegistry()
	handler := &MockHandler{resourceType: "Microsoft.Storage/storageAccounts"}

	registry.Register("Microsoft.Storage/storageAccounts", handler)

	tests := []struct {
		name         string
		resourceType string
		expected     bool
	}{
		{
			name:         "existing handler",
			resourceType: "Microsoft.Storage/storageAccounts",
			expected:     true,
		},
		{
			name:         "non-existing handler",
			resourceType: "Microsoft.Compute/virtualMachines",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.HasHandler(tt.resourceType)
			if result != tt.expected {
				t.Errorf("HasHandler(%q) = %v, want %v", tt.resourceType, result, tt.expected)
			}
		})
	}
}

func TestRegistryConcurrency(t *testing.T) {
	registry := NewEmptyRegistry()

	// Test concurrent registration
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			handler := &MockHandler{
				resourceType: "Microsoft.Test/resource" + string(rune('0'+idx)),
			}
			registry.Register(handler.resourceType, handler)
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all handlers were registered
	types := registry.GetAllTypes()
	if len(types) != 10 {
		t.Errorf("Concurrent Register() failed, expected 10 handlers, got %d", len(types))
	}
}
