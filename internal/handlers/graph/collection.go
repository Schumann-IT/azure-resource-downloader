package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	kjson "github.com/microsoft/kiota-serialization-json-go"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
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
	azureType string
	// documentation holds the per-type metadata used to build this type's
	// dedicated documentation prompt. Each constructor sets it so every resource
	// type carries its own prompt; AzureType is filled in from the
	// handler at prompt-build time.
	documentation models.ResourceDocumentation
	// worksWithCLICredential opts a specific Graph type out of requiring a
	// dedicated app registration. It defaults to false: every Microsoft Graph
	// collection here reads admin/Intune data via delegated scopes the Azure CLI
	// first-party app cannot consent to (e.g. Policy.Read.All,
	// DeviceManagementConfiguration.Read.All), so by default these types need a
	// dedicated app (--client-id/--tenant-id). A constructor may set this true
	// for a type that is readable with the plain az login session.
	worksWithCLICredential bool
	// hasAssignments marks a type that has an assignments concept (its fetchItem
	// populates assignments). It is surfaced via HasAssignments so the export can
	// record it per type, and defaults false for types with no assignments.
	hasAssignments bool
	listIDs        func(ctx context.Context) ([]string, error)
	fetchItem      func(ctx context.Context, itemID string) (serialization.Parsable, error)
	displayName    func(item serialization.Parsable) string
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

// newGraphClient creates a Microsoft Graph v1.0 (stable) client for the given
// credential, requesting the default scope so the token carries all consented
// delegated permissions.
func newGraphClient(credential azcore.TokenCredential) (*msgraphsdk.GraphServiceClient, error) {
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Graph client: %w", err)
	}
	return client, nil
}

// GetType returns the Azure resource type
func (h *GraphCollectionHandler) GetType() string {
	return h.azureType
}

// GetDocumentationPrompt returns the dedicated LLM documentation prompt for
// this resource type, tailored via the per-type metadata set by the type's
// constructor.
func (h *GraphCollectionHandler) GetDocumentationPrompt() string {
	doc := h.documentation
	doc.AzureType = h.azureType
	return models.BuildDocumentationPrompt(doc)
}

// RequiresDedicatedApp reports whether this Microsoft Graph type needs a
// dedicated app registration (device-code sign-in with delegated scopes the
// Azure CLI first-party app cannot provide). It is true for every Graph
// collection unless the constructor opts out via worksWithCLICredential.
func (h *GraphCollectionHandler) RequiresDedicatedApp() bool {
	return !h.worksWithCLICredential
}

// RequiredPermissions returns the delegated Microsoft Graph permissions this
// type needs, as declared in its documentation metadata (for user messages).
func (h *GraphCollectionHandler) RequiredPermissions() []string {
	return h.documentation.RequiredPermissions
}

// HasAssignments reports whether this Microsoft Graph type has an assignments
// concept, as declared by its constructor. It satisfies models.AssignmentCapable.
func (h *GraphCollectionHandler) HasAssignments() bool {
	return h.hasAssignments
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

// serializeParsableToMap serializes a Kiota Parsable object to a generic map by
// round-tripping through its JSON representation. This captures the full nested
// tree (including polymorphic @odata.type discriminated children) without having
// to manually handle every model type.
func serializeParsableToMap(parsable serialization.Parsable) (map[string]interface{}, error) {
	writer := kjson.NewJsonSerializationWriter()
	defer func() { _ = writer.Close() }()

	if err := writer.WriteObjectValue("", parsable); err != nil {
		return nil, fmt.Errorf("failed to write object value: %w", err)
	}

	content, err := writer.GetSerializedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get serialized content: %w", err)
	}

	properties := make(map[string]interface{})
	if err := json.Unmarshal(content, &properties); err != nil {
		return nil, fmt.Errorf("failed to unmarshal serialized content: %w", err)
	}

	return properties, nil
}

// warnAssignmentsFetchFailed logs a warning when the /assignments child
// collection of an item could not be fetched. Assignment reads are
// best-effort: the item is still exported, just without its assignments, so a
// permission or transient failure never aborts the download.
func warnAssignmentsFetchFailed(resourceType, itemID string, err error) {
	logger.Default.Warn("Failed to fetch assignments; exporting item without assignments",
		"type", resourceType,
		"id", itemID,
		"reason", azure.ErrorSummary(err))
	logger.Default.Debug("Assignments fetch failed",
		"type", resourceType,
		"id", itemID,
		"error", err)
}

// safeStringValue safely dereferences a string pointer
func safeStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
