package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewMobileAppConfigurationHandler creates a handler for Intune app
// configuration policies for managed devices
// (deviceAppManagement/mobileAppConfigurations, Microsoft Graph beta). The
// collection is polymorphic per platform (iosMobileAppConfiguration,
// androidManagedStoreAppConfiguration, ...); the settings payload is part of
// the object, so no $expand is required.
func NewMobileAppConfigurationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/mobileAppConfigurations",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune managed device app configuration policy (app configuration for managed iOS/Android devices).",
			EmbeddedPayloads: []string{
				"encodedSettingXml (iosMobileAppConfiguration: base64 XML from the 'Enter XML data' format)",
				"payloadJson (androidManagedStoreAppConfiguration and androidForWorkMobileAppConfiguration: JSON app configuration payload)",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Settings are delivered when the targeted app is installed and read by the app when it checks for them (typically at first run); on iOS/iPadOS this works only for MDM-managed apps, not DDM-managed apps.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/mobileApps (targetedMobileApps)",
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (assignment filters)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			SubtypeNote: "Platform-polymorphic managed-device app configuration (iosMobileAppConfiguration, androidManagedStoreAppConfiguration, androidForWorkMobileAppConfiguration) - identify the platform from @odata.type.",
			KeySettings: []string{
				"targetedMobileApps",
				"settings (appConfigKey, appConfigKeyType, appConfigKeyValue)",
				"permissionActions",
				"profileApplicability",
				"connectedAppsEnabled",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-apps-manageddevicemobileappconfiguration?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-apps-manageddevicemobileappconfiguration-list?view=graph-rest-beta",
				AdminCenter:  "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/AppsMenu/~/configuration",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/app-management/configuration/configure-managed-ios",
					"https://learn.microsoft.com/en-us/intune/app-management/configuration/configure-managed-android",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().MobileAppConfigurations()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list mobile app configurations: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceAppManagement().MobileAppConfigurations().ByManagedDeviceMobileAppConfigurationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get mobile app configuration: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().MobileAppConfigurations().ByManagedDeviceMobileAppConfigurationId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/mobileAppConfigurations", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.ManagedDeviceMobileAppConfigurationable); ok {
				return safeStringValue(c.GetDisplayName())
			}
			return ""
		},
	}, nil
}
