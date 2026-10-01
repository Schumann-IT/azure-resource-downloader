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

// NewWindowsManagedAppProtectionHandler creates a handler for Intune Windows
// app protection (MAM) policies
// (deviceAppManagement/windowsManagedAppProtections, Microsoft Graph beta).
//
// Fetch uses $expand=apps so the targeted app list is included (it is not
// returned by a plain GET).
func NewWindowsManagedAppProtectionHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/windowsManagedAppProtections",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune app protection (MAM) policy for Windows that protects org data accessed through Microsoft Edge on personal, unmanaged Windows devices.",
			KeySettings: []string{
				"allowedInboundDataTransferSources",
				"allowedOutboundDataTransferDestinations",
				"allowedOutboundClipboardSharingLevel",
				"printBlocked",
				"maximumAllowedDeviceThreatLevel",
				"minimumRequiredOsVersion",
				"periodOfflineBeforeWipeIsEnforced",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Windows MAM supports only unmanaged devices: MAM enrollment is blocked on a managed device, and the settings stop applying if the device becomes managed later.",
				"It works together with an app configuration policy, Windows Security app threat defense and App Protection Conditional Access; the Windows Security Center threat defense connector needs Windows 11 23H2 or later.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (assignment filters)",
				"Microsoft.Graph/mobileThreatDefenseConnectors (Windows Security Center threat level)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-windowsmanagedappprotection?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/intune-mam-windowsmanagedappprotection-list?view=graph-rest-beta",
				AdminCenter:   "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/AppsMenu/~/protection",
				BestPractices: []string{"https://learn.microsoft.com/en-us/intune/app-management/protection/enable-mam-windows"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().WindowsManagedAppProtections()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows managed app protections: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadeviceappmanagement.WindowsManagedAppProtectionsWindowsManagedAppProtectionItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.WindowsManagedAppProtectionsWindowsManagedAppProtectionItemRequestBuilderGetQueryParameters{
					Expand: []string{"apps"},
				},
			}
			item, err := client.DeviceAppManagement().WindowsManagedAppProtections().ByWindowsManagedAppProtectionId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows managed app protection: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().WindowsManagedAppProtections().ByWindowsManagedAppProtectionId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/windowsManagedAppProtections", itemID, err)
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
