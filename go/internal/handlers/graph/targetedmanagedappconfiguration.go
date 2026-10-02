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
		azureType:      "Microsoft.Graph/targetedManagedAppConfigurations",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune app configuration policy for managed apps, delivered through the app protection (MAM) channel to Intune SDK-integrated apps regardless of the device's enrollment state.",
			KeySettings: []string{
				"customSettings (name/value pairs defined by the app's vendor; values in {{ }} are tokens Intune fills per user)",
				"appGroupType",
				"targetedAppManagementLevels",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Managed apps check for app configuration every 30 minutes when an app protection policy also targets the user, otherwise every 720 minutes.",
				"On Windows only Microsoft Edge for Windows can be configured, with Settings catalog settings as well as name/value pairs; on Android, Intune requires Android 10.0 or later.",
				"On iOS/iPadOS, delivered values can be checked in the Intune diagnostic log: open about:intunehelp in Microsoft Edge, share the logs and search IntuneMAMDiagnostics.txt for ApplicationConfiguration.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (managed-app filters)",
				"Microsoft.Graph/iosManagedAppProtections (an app protection policy for the user shortens the check-in interval)",
				"Microsoft.Graph/androidManagedAppProtections (same)",
				"Microsoft.Graph/windowsManagedAppProtections (same)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-targetedmanagedappconfiguration?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-mam-targetedmanagedappconfiguration-list?view=graph-rest-beta",
				AdminCenter:  "https://intune.microsoft.com/#view/Microsoft_Intune_Apps/ConfigPoliciesMenu/~/appConfigurationPolicies",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/app-management/configuration/configure-managed-apps",
				},
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
