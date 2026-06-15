package azure

import (
	"context"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
)

// Client wraps Azure SDK clients
type Client struct {
	credential      azcore.TokenCredential
	subscriptionID  string
	tenantID        string
	resourcesClient *armresources.Client
}

// NewClient creates a new Azure client. By default it authenticates as the user
// signed in via the Azure CLI (az login). If clientID is set, it instead performs
// an interactive device-code sign-in against that dedicated app registration,
// which can carry Microsoft Graph scopes the first-party Azure CLI app cannot
// obtain (e.g. Policy.Read.All, DeviceManagementConfiguration.ReadWrite.All).
// The same delegated token is used for both ARM and Microsoft Graph requests.
// If subscriptionID is empty, it attempts to resolve a default subscription for
// that user.
func NewClient(ctx context.Context, subscriptionID, clientID, tenantID string) (*Client, error) {
	cred, err := newCredential(clientID, tenantID)
	if err != nil {
		return nil, err
	}

	// If no subscription ID is provided, try to get the default one. Failing to
	// resolve a subscription is NOT fatal: the signed-in user may only have
	// tenant-level (Microsoft Graph) access. We warn and continue with an empty
	// subscription so Graph resources can still be downloaded; ARM resource
	// types are skipped later.
	if subscriptionID == "" {
		defaultSub, err := getDefaultSubscription(ctx, cred)
		if err != nil {
			logger.Default.Warn("No Azure subscription available; ARM resources cannot be downloaded and will be skipped (Microsoft Graph resources are unaffected)",
				"reason", ErrorSummary(err))
			logger.Default.Debug("Default subscription resolution failed", "error", err)
		} else {
			subscriptionID = defaultSub
		}
	}

	// Create the generic ARM resources client only when a subscription exists.
	var resourcesClient *armresources.Client
	if subscriptionID != "" {
		resourcesClient, err = armresources.NewClient(subscriptionID, cred, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create resources client: %w", err)
		}
	}

	return &Client{
		credential:      cred,
		subscriptionID:  subscriptionID,
		tenantID:        tenantID,
		resourcesClient: resourcesClient,
	}, nil
}

// newCredential builds the token credential used for all ARM and Microsoft Graph
// requests. With no clientID it reuses the existing Azure CLI session (az login).
// When clientID is provided it starts an interactive device-code sign-in against
// the given app registration so that delegated scopes unavailable to the Azure
// CLI first-party app can be requested.
func newCredential(clientID, tenantID string) (azcore.TokenCredential, error) {
	if clientID == "" {
		cred, err := azidentity.NewAzureCLICredential(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to use Azure CLI credentials: %w (hint: run 'az login' first)", err)
		}
		return cred, nil
	}

	// A single-tenant app registration requires an explicit tenant; without it
	// the device-code endpoint rejects the request with AADSTS50059.
	if tenantID == "" {
		return nil, fmt.Errorf("--tenant-id is required when --client-id is set (device-code sign-in needs a tenant; pass --tenant-id or set AZURE_RD_TENANT_ID)")
	}

	opts := &azidentity.DeviceCodeCredentialOptions{
		ClientID: clientID,
		TenantID: tenantID,
		UserPrompt: func(_ context.Context, msg azidentity.DeviceCodeMessage) error {
			logger.Default.Info("Device-code sign-in required", "instructions", msg.Message)
			return nil
		},
	}
	cred, err := azidentity.NewDeviceCodeCredential(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to start device-code sign-in: %w (hint: ensure the app registration allows public client flows)", err)
	}
	return cred, nil
}

// getDefaultSubscription retrieves the default subscription from Azure
func getDefaultSubscription(ctx context.Context, cred azcore.TokenCredential) (string, error) {
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
	if c.resourcesClient == nil {
		return nil, fmt.Errorf("cannot get ARM resource %q: no subscription available", resourceID)
	}

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

// GetTenantDomain resolves the Entra tenant's default domain (e.g.
// "contoso.onmicrosoft.com") using the ARM Tenants API. When the signed-in
// identity can access multiple tenants, the tenant matching the configured
// tenant ID (or, failing that, the active subscription's tenant) is preferred;
// otherwise the first tenant with a default domain is returned.
func (c *Client) GetTenantDomain(ctx context.Context) (string, error) {
	tenantsClient, err := armsubscriptions.NewTenantsClient(c.credential, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create tenants client: %w", err)
	}

	// Disambiguate when several tenants are accessible: prefer an explicit
	// tenant, then the active subscription's tenant.
	targetTenantID := c.tenantID
	if targetTenantID == "" && c.subscriptionID != "" {
		if id, err := c.getSubscriptionTenantID(ctx); err != nil {
			logger.Default.Debug("Could not resolve subscription tenant ID", "error", err)
		} else {
			targetTenantID = id
		}
	}

	pager := tenantsClient.NewListPager(nil)
	firstDomain := ""
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return "", fmt.Errorf("failed to list tenants: %w", err)
		}

		for _, tenant := range page.Value {
			if tenant == nil || tenant.DefaultDomain == nil || *tenant.DefaultDomain == "" {
				continue
			}
			domain := *tenant.DefaultDomain

			if targetTenantID != "" && tenant.TenantID != nil && strings.EqualFold(*tenant.TenantID, targetTenantID) {
				return domain, nil
			}
			if firstDomain == "" {
				firstDomain = domain
			}
		}
	}

	if firstDomain != "" {
		return firstDomain, nil
	}
	return "", fmt.Errorf("no tenant default domain found")
}

// getSubscriptionTenantID returns the tenant ID that owns the active
// subscription, used to pick the correct tenant when several are accessible.
func (c *Client) getSubscriptionTenantID(ctx context.Context) (string, error) {
	client, err := armsubscriptions.NewClient(c.credential, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	resp, err := client.Get(ctx, c.subscriptionID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get subscription: %w", err)
	}
	if resp.TenantID == nil {
		return "", fmt.Errorf("subscription %q has no tenant ID", c.subscriptionID)
	}
	return *resp.TenantID, nil
}

// GetCredential returns the Azure credential
func (c *Client) GetCredential() azcore.TokenCredential {
	return c.credential
}
