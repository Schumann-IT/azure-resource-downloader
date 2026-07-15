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
	return models.BuildDocumentationPrompt(models.ResourceDocumentation{
		AzureType:           h.GetType(),
		Purpose:             "An Azure Storage Account that provides blob, file, queue and table storage, with its security, networking and encryption configuration.",
		KeySettings:         []string{"enableHttpsTrafficOnly", "minimumTlsVersion", "allowBlobPublicAccess", "allowSharedKeyAccess", "networkRuleSet", "encryption"},
		RequiredPermissions: []string{"Reader (Azure RBAC role on the subscription)"},
		Lifecycle:           "Deleting a storage account is irreversible once retention lapses; enable soft delete/versioning, and rotate access keys regularly (rotation breaks clients using shared-key auth).",
		Links: models.ResourceLinks{
			EndpointDocs:  "https://learn.microsoft.com/en-us/rest/api/storagerp/storage-accounts",
			BestPractices: []string{"https://learn.microsoft.com/en-us/azure/storage/blobs/security-recommendations"},
		},
	})
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
