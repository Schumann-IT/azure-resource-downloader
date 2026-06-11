package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

// ListResourcesByType lists the IDs of all ARM resources of the given type in
// the subscription. Azure's server-side filter is unreliable for resourceType,
// so results are filtered client-side. It is the shared helper that ARM
// resource handlers delegate to from their List method.
func ListResourcesByType(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID, resourceType string) ([]string, error) {
	client, err := armresources.NewClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resources client: %w", err)
	}

	var resourceIDs []string
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list resources: %w", err)
		}

		for _, resource := range page.Value {
			if resource.ID != nil && resource.Type != nil && *resource.Type == resourceType {
				resourceIDs = append(resourceIDs, *resource.ID)
			}
		}
	}

	return resourceIDs, nil
}

// ListResourceGroups lists the IDs of all resource groups in the subscription.
// Resource groups use a dedicated API rather than the generic resources pager.
func ListResourceGroups(ctx context.Context, cred *azidentity.DefaultAzureCredential, subscriptionID string) ([]string, error) {
	client, err := armresources.NewResourceGroupsClient(subscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource groups client: %w", err)
	}

	var resourceIDs []string
	pager := client.NewListPager(nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list resource groups: %w", err)
		}

		for _, rg := range page.Value {
			if rg.ID != nil {
				resourceIDs = append(resourceIDs, *rg.ID)
			}
		}
	}

	return resourceIDs, nil
}
