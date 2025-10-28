package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

// Client wraps Azure SDK clients
type Client struct {
	credential      *azidentity.DefaultAzureCredential
	subscriptionID  string
	resourcesClient *armresources.Client
}

// NewClient creates a new Azure client
// If subscriptionID is empty, it will attempt to use the default subscription from the Azure CLI
func NewClient(ctx context.Context, subscriptionID string) (*Client, error) {
	// Use DefaultAzureCredential which handles multiple auth methods (az login, env vars, managed identity, etc.)
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create credential: %w", err)
	}

	// If no subscription ID is provided, try to get the default one
	if subscriptionID == "" {
		defaultSub, err := getDefaultSubscription(ctx, cred)
		if err != nil {
			return nil, fmt.Errorf("no subscription specified and failed to get default subscription: %w\nHint: use 'az login' to authenticate or specify --subscription flag", err)
		}
		subscriptionID = defaultSub
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

// getDefaultSubscription retrieves the default subscription from Azure
func getDefaultSubscription(ctx context.Context, cred *azidentity.DefaultAzureCredential) (string, error) {
	client, err := armsubscriptions.NewClient(cred, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	// List subscriptions and use the first one
	// Note: Azure CLI typically sets a default subscription which is marked as IsDefault=true
	pager := client.NewListPager(nil)

	var defaultSubscriptionID string
	var firstSubscriptionID string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list subscriptions: %w", err)
		}

		for _, sub := range page.Value {
			if sub.SubscriptionID == nil {
				continue
			}

			// Store the first subscription as fallback
			if firstSubscriptionID == "" {
				firstSubscriptionID = *sub.SubscriptionID
			}

			// Check if this is marked as the default subscription
			if sub.State != nil && *sub.State == armsubscriptions.SubscriptionStateEnabled {
				// If this subscription is enabled and we don't have a default yet, use it
				if defaultSubscriptionID == "" {
					defaultSubscriptionID = *sub.SubscriptionID
				}
			}
		}
	}

	// Prefer the default subscription, otherwise use the first one found
	if defaultSubscriptionID != "" {
		return defaultSubscriptionID, nil
	}

	if firstSubscriptionID != "" {
		return firstSubscriptionID, nil
	}

	return "", fmt.Errorf("no subscriptions found in the account")
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
