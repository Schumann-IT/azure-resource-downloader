package arm

import (
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// ResourceGroupHandler handles Azure Resource Groups
type ResourceGroupHandler struct {
	credential     azcore.TokenCredential
	subscriptionID string
}

// NewResourceGroupHandler creates a new resource group handler
func NewResourceGroupHandler(credential azcore.TokenCredential, subscriptionID string) *ResourceGroupHandler {
	return &ResourceGroupHandler{
		credential:     credential,
		subscriptionID: subscriptionID,
	}
}

// GetType returns the Azure resource type
func (h *ResourceGroupHandler) GetType() string {
	return "Microsoft.Resources/resourceGroups"
}

// GetDocumentationPrompt returns the dedicated LLM documentation prompt for this resource type.
func (h *ResourceGroupHandler) GetDocumentationPrompt() string {
	return models.BuildDocumentationPrompt(models.ResourceDocumentation{
		AzureType:           h.GetType(),
		Purpose:             "An Azure Resource Group, the logical container that holds related Azure resources and governs their lifecycle, location and tags.",
		KeySettings:         []string{"location", "tags", "provisioningState"},
		RequiredPermissions: []string{"Reader (Azure RBAC role on the subscription)"},
		Lifecycle:           "Deleting a resource group irreversibly deletes ALL contained resources; use resource locks and consistent tagging for governance.",
		Links: models.ResourceLinks{
			EndpointDocs: "https://learn.microsoft.com/en-us/rest/api/resources/resource-groups",
		},
	})
}

// List returns the IDs of all resource groups in the subscription.
func (h *ResourceGroupHandler) List(ctx context.Context) ([]string, error) {
	return azure.ListResourceGroups(ctx, h.credential, h.subscriptionID)
}

// Fetch retrieves a resource group from Azure
func (h *ResourceGroupHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	// Parse resource ID to get resource group name
	idInfo, err := azure.ParseResourceID(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource ID: %w", err)
	}

	client, err := armresources.NewResourceGroupsClient(h.subscriptionID, h.credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource groups client: %w", err)
	}

	resp, err := client.Get(ctx, idInfo.ResourceGroup, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource group: %w", err)
	}

	return resp.ResourceGroup, nil
}

// Transform converts the raw resource group into a cleaned version
func (h *ResourceGroupHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	rg, ok := resource.(armresources.ResourceGroup)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected ResourceGroup")
	}

	if rg.Name == nil {
		return nil, fmt.Errorf("resource group name is nil")
	}

	properties := make(map[string]interface{})

	if rg.ID != nil {
		properties["id"] = *rg.ID
	}
	if rg.Name != nil {
		properties["name"] = *rg.Name
	}
	if rg.Location != nil {
		properties["location"] = *rg.Location
	}
	if len(rg.Tags) > 0 {
		properties["tags"] = rg.Tags
	}
	if rg.Type != nil {
		properties["type"] = *rg.Type
	}
	if rg.Properties != nil && rg.Properties.ProvisioningState != nil {
		properties["provisioningState"] = *rg.Properties.ProvisioningState
	}

	return &models.TransformedResource{
		ID:          safeString(rg.ID),
		Type:        h.GetType(),
		Name:        safeString(rg.Name),
		DisplayName: safeString(rg.Name),
		Properties:  properties,
	}, nil
}

// safeString safely dereferences a string pointer
func safeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
