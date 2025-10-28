package handlers

import (
	"fmt"
	"sync"

	"azure-resource-downloader/internal/models"
)

// Registry manages all registered resource handlers
type Registry struct {
	handlers map[string]models.ResourceHandler
	mu       sync.RWMutex
}

// NewRegistry creates a new handler registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[string]models.ResourceHandler),
	}
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
