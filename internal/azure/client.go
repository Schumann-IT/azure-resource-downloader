package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// Client wraps Azure SDK clients
type Client struct {
	credential      *azidentity.DefaultAzureCredential
	subscriptionID  string
	resourcesClient *armresources.Client
}

// NewClient creates a new Azure client
func NewClient(ctx context.Context, subscriptionID string) (*Client, error) {
	if subscriptionID == "" {
		return nil, fmt.Errorf("subscription ID is required")
	}

	// Use DefaultAzureCredential which handles multiple auth methods
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// Create resources client for generic resource operations
	resourcesClient, err := armresources.NewClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resources client: %w", err)
	}

	return &Client{
		credential:      cred,
		subscriptionID:  subscriptionID,
		resourcesClient: resourcesClient,
	}, nil
}

// GetResource retrieves a generic Azure resource by ID
func (c *Client) GetResource(ctx context.Context, resourceID, apiVersion string) (map[string]interface{}, error) {
	// Parse resource ID to get the resource details
	result, err := c.resourcesClient.GetByID(ctx, resourceID, apiVersion, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	// Convert to map for generic processing
	resourceMap := make(map[string]interface{})

	if result.ID != nil {
		resourceMap["id"] = *result.ID
	}
	if result.Name != nil {
		resourceMap["name"] = *result.Name
	}
	if result.Type != nil {
		resourceMap["type"] = *result.Type
	}
	if result.Location != nil {
		resourceMap["location"] = *result.Location
	}
	if result.Tags != nil {
		resourceMap["tags"] = result.Tags
	}
	if result.Properties != nil {
		resourceMap["properties"] = result.Properties
	}

	return resourceMap, nil
}

// GetSubscriptionID returns the subscription ID
func (c *Client) GetSubscriptionID() string {
	return c.subscriptionID
}

// GetCredential returns the Azure credential
func (c *Client) GetCredential() *azidentity.DefaultAzureCredential {
	return c.credential
}
