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
//
// The Terraform resource type is platform-specific in
// terraform-provider-microsoft365 (ios/android managed-device app
// configuration policy variants); the iOS variant is emitted as the default —
// verify the import against the policy's @odata.type.
func NewMobileAppConfigurationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/mobileAppConfigurations",
		terraformType: "microsoft365_graph_beta_device_and_app_management_ios_managed_device_app_configuration_policy",
		documentation: docMeta(
			"An Intune managed device app configuration policy (app configuration for managed iOS/Android devices).",
			nil,
			[]string{"encodedSettingXml (base64)", "settings"},
			models.ResourceLinks{},
		),
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
