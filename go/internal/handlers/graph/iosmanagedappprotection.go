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

// NewIosManagedAppProtectionHandler creates a handler for Intune iOS app
// protection (MAM) policies (deviceAppManagement/iosManagedAppProtections,
// Microsoft Graph beta).
//
// Fetch uses $expand=apps so the targeted app list is included (it is not
// returned by a plain GET).
func NewIosManagedAppProtectionHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/iosManagedAppProtections",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune iOS App Protection (MAM) policy controlling data protection for managed apps.",
			KeySettings: []string{
				"dataBackupBlocked",
				"managedBrowserToOpenLinksRequired (managedBrowser and customBrowserProtocol act only when it is true)",
				"pinRequired (minimumPinLength, pinCharacterSet, fingerprintBlocked and faceIdBlocked act only when it is true)",
				"allowedOutboundDataTransferDestinations (disableProtectionOfManagedOutboundOpenInData can be true only with managedApps; filterOpenInToOnlyManagedApps acts only with managedApps and disableProtectionOfManagedOutboundOpenInData false)",
				"allowedOutboundClipboardSharingLevel",
				"appDataEncryptionType",
				"maximumAllowedDeviceThreatLevel (with mobileThreatDefenseRemediationAction and mobileThreatDefensePartnerPriority; no effect unless an MTD connector feeds app protection)",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Apps pick up policy changes at check-in, typically every 30 minutes (every 12 hours while the user isn't licensed or targeted); apps that haven't checked in for 90 days may be deregistered.",
				"The policy applies only to Intune-licensed users in targeted groups who sign in to the app; org data is removed only by an app selective wipe, applied the next time the app runs.",
				"Only apps that integrate the Intune App SDK or are wrapped with the Intune App Wrapping Tool can be protected.",
				"A policy targeting Intune managed devices (targetedAppManagementLevels mdm) applies only once a managed-devices app configuration sends the app IntuneMAMUPN and IntuneMAMOID (plus IntuneMAMDeviceID for third-party and line-of-business apps); Intune sends them itself for some Microsoft apps.",
				"If a second policy targets a user and app that a policy already covers, the first stays applied and the second shows a conflict; policies applied at the same time conflict, and conflicting settings take the most restrictive value (numeric fields the recommended value).",
				"Delivery shows in Apps > Monitor > App protection status, which lists app instances that checked in within the last 90 days; a device's applied settings can be checked in Microsoft Edge at about:intunehelp.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (assignment filters)",
				"Microsoft.Graph/mobileThreatDefenseConnectors (maximum allowed device threat level)",
				"Microsoft.Graph/mobileAppConfigurations (IntuneMAMUPN and IntuneMAMOID for targeting Intune managed devices)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-iosmanagedappprotection?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-mam-iosmanagedappprotection-list?view=graph-rest-beta",
				AdminCenter:  "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/AppsMenu/~/protection",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/app-management/protection/data-protection-framework",
					"https://learn.microsoft.com/en-us/intune/app-management/protection/create-policy",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().IosManagedAppProtections()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list iOS managed app protections: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadeviceappmanagement.IosManagedAppProtectionsIosManagedAppProtectionItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.IosManagedAppProtectionsIosManagedAppProtectionItemRequestBuilderGetQueryParameters{
					Expand: []string{"apps"},
				},
			}
			item, err := client.DeviceAppManagement().IosManagedAppProtections().ByIosManagedAppProtectionId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get iOS managed app protection: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().IosManagedAppProtections().ByIosManagedAppProtectionId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/iosManagedAppProtections", itemID, err)
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
