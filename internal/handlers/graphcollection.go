package handlers

import (
	"context"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
)

// GraphCollectionHandler implements models.ResourceHandler for simple Microsoft
// Graph collections that require no $expand or child fetches: GET the
// collection, GET each item by ID, and serialize the full (potentially
// polymorphic) model to a generic map.
//
// Concrete handlers (one per resource type) configure it with closures for
// listing, fetching and naming, keeping all Graph SDK builder specifics in the
// per-resource constructor.
type GraphCollectionHandler struct {
	azureType     string
	terraformType string
	listIDs       func(ctx context.Context) ([]string, error)
	fetchItem     func(ctx context.Context, itemID string) (serialization.Parsable, error)
	displayName   func(item serialization.Parsable) string
}

// newBetaGraphClient creates a Microsoft Graph beta client for the given
// credential, requesting the default scope so the token carries all consented
// delegated permissions.
func newBetaGraphClient(credential azcore.TokenCredential) (*msgraphbeta.GraphServiceClient, error) {
	client, err := msgraphbeta.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create beta Graph client: %w", err)
	}
	return client, nil
}

// GetType returns the Azure resource type
func (h *GraphCollectionHandler) GetType() string {
	return h.azureType
}

// GetTerraformResourceType returns the Terraform resource type
func (h *GraphCollectionHandler) GetTerraformResourceType() string {
	return h.terraformType
}

// List returns the IDs of all items in the collection.
func (h *GraphCollectionHandler) List(ctx context.Context) ([]string, error) {
	return h.listIDs(ctx)
}

// Fetch retrieves a single item from the collection by resource ID.
func (h *GraphCollectionHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	itemID := extractGraphItemID(resourceID)
	if itemID == "" {
		return nil, fmt.Errorf("invalid resource ID format: %s", resourceID)
	}
	return h.fetchItem(ctx, itemID)
}

// Transform converts the raw Graph model into a cleaned, generic representation
// via the shared serializeParsableToMap helper.
func (h *GraphCollectionHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	item, ok := resource.(serialization.Parsable)
	if !ok {
		return nil, fmt.Errorf("invalid resource type for %s, expected a Microsoft Graph model", h.azureType)
	}

	displayName := h.displayName(item)
	if displayName == "" {
		return nil, fmt.Errorf("%s resource has an empty display name", h.azureType)
	}

	properties, err := serializeParsableToMap(item)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize %s resource: %w", h.azureType, err)
	}

	itemID, _ := properties["id"].(string)

	return &models.TransformedResource{
		ID:          itemID,
		Type:        h.azureType,
		Name:        displayName,
		DisplayName: displayName,
		Properties:  properties,
	}, nil
}

// extractGraphItemID extracts the item ID from various resource ID formats:
// either a full Graph path (last path segment) or a bare item ID.
func extractGraphItemID(resourceID string) string {
	if strings.Contains(resourceID, "/") {
		parts := strings.Split(resourceID, "/")
		return parts[len(parts)-1]
	}
	return resourceID
}
