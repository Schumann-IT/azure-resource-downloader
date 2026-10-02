package arm

import (
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
)

// StorageAccountHandler handles Azure Storage Accounts
type StorageAccountHandler struct {
	credential     azcore.TokenCredential
	subscriptionID string
}

// NewStorageAccountHandler creates a new storage account handler
func NewStorageAccountHandler(credential azcore.TokenCredential, subscriptionID string) *StorageAccountHandler {
	return &StorageAccountHandler{
		credential:     credential,
		subscriptionID: subscriptionID,
	}
}

// GetType returns the Azure resource type
func (h *StorageAccountHandler) GetType() string {
	return "Microsoft.Storage/storageAccounts"
}

// GetDocumentationPrompt returns the dedicated LLM documentation prompt for this resource type.
func (h *StorageAccountHandler) GetDocumentationPrompt() string {
	return models.BuildDocumentationPrompt(h.Documentation())
}

// Documentation returns the per-type documentation metadata the prompt is built
// from. It satisfies models.Documented.
func (h *StorageAccountHandler) Documentation() models.ResourceDocumentation {
	return models.ResourceDocumentation{
		Template:  armPromptTemplateText,
		AzureType: h.GetType(),
		Purpose:   "An Azure Storage Account that provides blob, file, queue and table storage, with its security, networking and encryption configuration.",
		KeySettings: []string{
			"enableHttpsTrafficOnly (REST: supportsHttpsTrafficOnly)",
			"minimumTlsVersion",
			"allowBlobPublicAccess (absent means not set, which API versions up to 2022-09-01 interpret as true and later ones as false; true only lets containers be opened for anonymous read, which each container's own access level decides)",
			"allowSharedKeyAccess (absent means null, which permits Shared Key; false also rejects account and service SAS)",
			"networkRuleSet (REST: networkAcls; its IP and virtual network rules apply only when defaultAction is Deny)",
			"encryption",
			"sku.name",
			"kind",
		},
		RequiredPermissions: []string{"Reader (Azure RBAC role on the subscription)"},
		Lifecycle: []string{
			"A deleted storage account may be recoverable within 14 days (best effort; not if an account with the same name was created since; a deleted resource group must be recreated first; private endpoints aren't restored); blob soft delete doesn't cover account deletion; a lock protects the account, not its data.",
			"Azure Blob Storage stopped accepting TLS 1.0 and 1.1 on 3 February 2026, and the minimum TLS version applies to every service in the account; general-purpose v1 accounts not migrated by October 2026 are migrated to general-purpose v2 automatically (possibly at higher cost).",
			"Rotating or regenerating an access key breaks clients that use shared-key authentication with it and invalidates SAS tokens signed with it.",
			"publicNetworkAccess (not exported) takes precedence over networkRuleSet: defaultAction can stay Allow while public access is Disabled; trusted-service exceptions and resource instance rules stay in effect even then, and IP rules don't apply to requests from the same Azure region.",
			"Not exported: publicNetworkAccess, private endpoints, managed identity, blob and container soft delete and versioning (blob service settings), diagnostic settings (resource logs aren't collected without one) and resource locks (separate resources, also inherited from parent scopes).",
		},
		Links: models.ResourceLinks{
			EndpointDocs:  "https://learn.microsoft.com/en-us/rest/api/storagerp/storage-accounts",
			Permissions:   "https://learn.microsoft.com/en-us/azure/role-based-access-control/built-in-roles/general#reader",
			BestPractices: []string{"https://learn.microsoft.com/en-us/azure/storage/blobs/security-recommendations"},
			AdminCenter:   "https://portal.azure.com/#view/HubsExtension/BrowseResource/resourceType/Microsoft.Storage%2FStorageAccounts",
		},
	}
}

// List returns the IDs of all storage accounts in the subscription.
func (h *StorageAccountHandler) List(ctx context.Context) ([]string, error) {
	return azure.ListResourcesByType(ctx, h.credential, h.subscriptionID, h.GetType())
}

// Fetch retrieves a storage account from Azure
func (h *StorageAccountHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	// Parse resource ID
	idInfo, err := azure.ParseResourceID(resourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource ID: %w", err)
	}

	client, err := armstorage.NewAccountsClient(h.subscriptionID, h.credential, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage accounts client: %w", err)
	}

	resp, err := client.GetProperties(ctx, idInfo.ResourceGroup, idInfo.ResourceName, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage account: %w", err)
	}

	return resp.Account, nil
}

// Transform converts the raw storage account into a cleaned version
func (h *StorageAccountHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	account, ok := resource.(armstorage.Account)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected Storage Account")
	}

	if account.Name == nil {
		return nil, fmt.Errorf("storage account name is nil")
	}

	properties := make(map[string]interface{})

	// Basic properties
	if account.ID != nil {
		properties["id"] = *account.ID
	}
	if account.Name != nil {
		properties["name"] = *account.Name
	}
	if account.Location != nil {
		properties["location"] = *account.Location
	}
	if account.Type != nil {
		properties["type"] = *account.Type
	}
	if len(account.Tags) > 0 {
		properties["tags"] = account.Tags
	}

	// SKU
	if account.SKU != nil {
		sku := make(map[string]interface{})
		if account.SKU.Name != nil {
			sku["name"] = string(*account.SKU.Name)
		}
		if account.SKU.Tier != nil {
			sku["tier"] = string(*account.SKU.Tier)
		}
		properties["sku"] = sku
	}

	// Kind
	if account.Kind != nil {
		properties["kind"] = string(*account.Kind)
	}

	// Properties
	if account.Properties != nil {
		accountProps := make(map[string]interface{})

		if account.Properties.AccessTier != nil {
			accountProps["accessTier"] = string(*account.Properties.AccessTier)
		}
		if account.Properties.EnableHTTPSTrafficOnly != nil {
			accountProps["enableHttpsTrafficOnly"] = *account.Properties.EnableHTTPSTrafficOnly
		}
		if account.Properties.MinimumTLSVersion != nil {
			accountProps["minimumTlsVersion"] = string(*account.Properties.MinimumTLSVersion)
		}
		if account.Properties.AllowBlobPublicAccess != nil {
			accountProps["allowBlobPublicAccess"] = *account.Properties.AllowBlobPublicAccess
		}
		if account.Properties.AllowSharedKeyAccess != nil {
			accountProps["allowSharedKeyAccess"] = *account.Properties.AllowSharedKeyAccess
		}
		if account.Properties.NetworkRuleSet != nil {
			accountProps["networkRuleSet"] = account.Properties.NetworkRuleSet
		}
		if account.Properties.Encryption != nil {
			accountProps["encryption"] = account.Properties.Encryption
		}

		properties["properties"] = accountProps
	}

	return &models.TransformedResource{
		ID:          safeString(account.ID),
		Type:        h.GetType(),
		Name:        safeString(account.Name),
		DisplayName: safeString(account.Name),
		Properties:  properties,
	}, nil
}
