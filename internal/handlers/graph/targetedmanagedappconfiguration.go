package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadeviceappmanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/deviceappmanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewTargetedManagedAppConfigurationHandler creates a handler for Intune app
// configuration policies for managed apps (MAM)
// (deviceAppManagement/targetedManagedAppConfigurations, Microsoft Graph
// beta). The key/value settings (customSettings) are part of the object;
// Fetch additionally uses $expand=apps so the targeted app list is included.
func NewTargetedManagedAppConfigurationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/targetedManagedAppConfigurations",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune App Configuration policy targeting managed apps (MAM) without device enrollment.",
			KeySettings:         []string{"customSettings", "appGroupType"},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle:           []string{"App configuration applies at the managed app next check-in; deleting the policy stops delivering the settings (apps keep the last received values until reinstalled)."},
			RelatedTypes:        []string{"Microsoft.Graph/mobileApps (targeted apps)", "Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-targetedmanagedappconfiguration?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().TargetedManagedAppConfigurations()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list targeted managed app configurations: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
				}
				if resp == nil {
					break
				}
				for _, item := range resp.GetValue() {
					if item.GetId() != nil {
						ids = append(ids, *item.GetId())
					}
				}
				next := resp.GetOdataNextLink()
				if next == nil || *next == "" {
					break
				}
				builder = builder.WithUrl(*next)
			}
			return ids, nil
		},
		fetchItem: func(ctx context.Context, itemID string) (serialization.Parsable, error) {
			requestConfig := &betadeviceappmanagement.TargetedManagedAppConfigurationsTargetedManagedAppConfigurationItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.TargetedManagedAppConfigurationsTargetedManagedAppConfigurationItemRequestBuilderGetQueryParameters{
					Expand: []string{"apps"},
				},
			}
			item, err := client.DeviceAppManagement().TargetedManagedAppConfigurations().ByTargetedManagedAppConfigurationId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get targeted managed app configuration: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().TargetedManagedAppConfigurations().ByTargetedManagedAppConfigurationId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/targetedManagedAppConfigurations", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.ManagedAppPolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
