package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewDeviceCategoryHandler creates a handler for Intune device categories
// (deviceManagement/deviceCategories, Microsoft Graph beta).
func NewDeviceCategoryHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceCategories",
		documentation: models.ResourceDocumentation{
			Template:            recordPromptTemplateText,
			OmitGroupAxes:       true,
			Purpose:             "An Intune device category used to group and target devices at enrollment.",
			RequiredPermissions: []string{"DeviceManagementManagedDevices.Read.All"},
			Lifecycle: []string{
	"Users pick a category in Company Portal (Windows users on the Company Portal website) unless the prompt is hidden in Company Portal customization; admins can also set it.",
	"Deleting a category shows its devices as Unassigned; dynamic device groups built on deviceCategory pick up changes automatically, so update group rules that reference a renamed or deleted category.",
},
			RelatedTypes: []string{
	"Microsoft.Graph/groups (dynamic device groups on device.deviceCategory)",
	"Microsoft.Graph/intuneBrandingProfiles (disableDeviceCategorySelection hides the prompt)",
	"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
},
Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-devicecategory?view=graph-rest-beta",
				Permissions: "https://learn.microsoft.com/en-us/graph/api/intune-onboarding-devicecategory-list?view=graph-rest-beta",
			AdminCenter: "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/DevicesMenu/~/deviceCategories",
BestPractices: []string{"https://learn.microsoft.com/en-us/intune/device-management/create-device-categories"},
},
		},
		probe: func(ctx context.Context) error {
			_, err := client.DeviceManagement().DeviceCategories().Get(ctx, &betadevicemanagement.DeviceCategoriesRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.DeviceCategoriesRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list device categories: %w (hint: requires 'DeviceManagementManagedDevices.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceCategories()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device categories: %w (hint: requires 'DeviceManagementManagedDevices.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceCategories().ByDeviceCategoryId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get device category: %w (hint: requires 'DeviceManagementManagedDevices.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.DeviceCategoryable); ok {
				return safeStringValue(c.GetDisplayName())
			}
			return ""
		},
	}, nil
}
