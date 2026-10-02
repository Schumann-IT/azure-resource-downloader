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
	return models.BuildDocumentationPrompt(h.Documentation())
}

// Documentation returns the per-type documentation metadata the prompt is built
// from. It satisfies models.Documented.
func (h *ResourceGroupHandler) Documentation() models.ResourceDocumentation {
	return models.ResourceDocumentation{
		Template:            armPromptTemplateText,
		AzureType:           h.GetType(),
		Purpose:             "An Azure resource group: a container for related Azure resources that share a lifecycle. Its location sets where the group's metadata is stored and through which region ARM control-plane operations on its resources are routed (resources can sit in other regions); its tags aren't inherited by its resources.",
		KeySettings:         []string{"location", "tags"},
		RequiredPermissions: []string{"Reader (Azure RBAC role on the subscription)"},
		Lifecycle: []string{
			"Deleting a resource group deletes ALL contained resources and the group itself can't be recovered; some recently deleted resources might still be restored (soft delete where the type supports it and it was enabled, or an Azure support case, never guaranteed); resource locks on the group or any resource in it, and backup data, must be removed first.",
			"Deleting the group needs only the resource group delete permission, not delete permission on each contained resource, and it overrides delete actions excluded through a role's notActions.",
			"The location can't be changed after creation; use resource locks and consistent tagging for governance.",
			"At most 50 tags per resource group; the deployment history keeps 800 deployments and is pruned automatically, but a CanNotDelete lock on the group stops the pruning, so deployments fail once the history reaches 800.",
			"Resource locks (separate resources, also inherited from the subscription) and managedBy (the resource that manages the group) aren't exported: the YAML can't show whether the group is locked or managed by another resource.",
			"The activity log is the only record of who created the resource group; it also records changes to and deletions of the group and its resources, and Azure keeps it 90 days (longer only where a diagnostic setting exports it).",
		},
		Links: models.ResourceLinks{
			EndpointDocs: "https://learn.microsoft.com/en-us/rest/api/resources/resource-groups",
			Permissions:  "https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/general#reader",
		},
	}
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
