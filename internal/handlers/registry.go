package handlers

import (
	"fmt"
	"sync"

	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

// Registry manages all registered resource handlers
type Registry struct {
	handlers map[string]models.ResourceHandler
	mu       sync.RWMutex
}

// NewEmptyRegistry creates a new handler registry with no handlers registered.
// Use NewRegistry to obtain a registry pre-populated with all supported types.
func NewEmptyRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]models.ResourceHandler),
	}
}

// NewRegistry creates a registry pre-populated with all supported resource
// handlers (ARM + Microsoft Graph). cred is the shared Azure credential,
// subscriptionID scopes ARM listing, and resolveSecrets toggles Intune OMA-URI
// secret resolution in the device configuration handler.
func NewRegistry(cred azcore.TokenCredential, subscriptionID string, resolveSecrets bool) *Registry {
	r := NewEmptyRegistry()
	registerDefaults(r, cred, subscriptionID, resolveSecrets)
	return r
}

// Register adds a handler for a specific resource type
func (r *Registry) Register(resourceType string, handler models.ResourceHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[resourceType] = handler
}

// Get retrieves a handler for a specific resource type
func (r *Registry) Get(resourceType string) (models.ResourceHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, exists := r.handlers[resourceType]
	if !exists {
		return nil, fmt.Errorf("no handler registered for resource type: %s", resourceType)
	}

	return handler, nil
}

// GetAllTypes returns all registered resource types
func (r *Registry) GetAllTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.handlers))
	for resourceType := range r.handlers {
		types = append(types, resourceType)
	}
	return types
}

// HasHandler checks if a handler exists for the given resource type
func (r *Registry) HasHandler(resourceType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.handlers[resourceType]
	return exists
}
