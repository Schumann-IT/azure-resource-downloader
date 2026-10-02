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

// NewAndroidManagedAppProtectionHandler creates a handler for Intune Android
// app protection (MAM) policies
// (deviceAppManagement/androidManagedAppProtections, Microsoft Graph beta).
//
// Fetch uses $expand=apps so the targeted app list is included (it is not
// returned by a plain GET).
func NewAndroidManagedAppProtectionHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/androidManagedAppProtections",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Android App Protection (MAM) policy controlling data protection for managed apps.",
			KeySettings: []string{
				"dataBackupBlocked",
				"screenCaptureBlocked",
				"pinRequired (minimumPinLength, pinCharacterSet, fingerprintBlocked and biometricAuthenticationBlocked act only when it is true)",
				"allowedOutboundDataTransferDestinations",
				"allowedOutboundClipboardSharingLevel",
				"requiredAndroidSafetyNetDeviceAttestationType (the Play integrity verdict setting despite its SafetyNet name; requiredAndroidSafetyNetEvaluationType acts only once it is set)",
				"minimumRequiredPatchVersion",
				"maximumAllowedDeviceThreatLevel (with mobileThreatDefenseRemediationAction and mobileThreatDefensePartnerPriority; no effect unless an MTD connector feeds app protection)",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Apps pick up policy changes at check-in, typically every 30 minutes (every 12 hours while the user isn't licensed or targeted); apps that haven't checked in for 90 days may be deregistered.",
				"The policy applies only to Intune-licensed users in targeted groups who sign in to the app; org data is removed only by an app selective wipe, applied the next time the app runs.",
				"Only apps that integrate the Intune App SDK or are wrapped with the Intune App Wrapping Tool can be protected; the device needs the Company Portal app (enrollment isn't required) and Android 10.0 or later.",
				"If a second policy targets a user and app that a policy already covers, the first stays applied and the second shows a conflict; policies applied at the same time conflict, and conflicting settings take the most restrictive value (numeric fields the recommended value).",
				"Delivery shows in Apps > Monitor > App protection status, which lists app instances that checked in within the last 90 days; a device's applied settings can be checked in Microsoft Edge at about:intunehelp.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (assignment filters)",
				"Microsoft.Graph/mobileThreatDefenseConnectors (maximum allowed device threat level)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-androidmanagedappprotection?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-mam-androidmanagedappprotection-list?view=graph-rest-beta",
				AdminCenter:  "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/AppsMenu/~/protection",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/app-management/protection/data-protection-framework",
					"https://learn.microsoft.com/en-us/intune/app-management/protection/create-policy",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().AndroidManagedAppProtections()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Android managed app protections: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadeviceappmanagement.AndroidManagedAppProtectionsAndroidManagedAppProtectionItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.AndroidManagedAppProtectionsAndroidManagedAppProtectionItemRequestBuilderGetQueryParameters{
					Expand: []string{"apps"},
				},
			}
			item, err := client.DeviceAppManagement().AndroidManagedAppProtections().ByAndroidManagedAppProtectionId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get Android managed app protection: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().AndroidManagedAppProtections().ByAndroidManagedAppProtectionId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/androidManagedAppProtections", itemID, err)
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
