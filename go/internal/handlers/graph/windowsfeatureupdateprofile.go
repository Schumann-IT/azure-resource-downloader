package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsFeatureUpdateProfileHandler creates a handler for Windows feature
// update profiles (deviceManagement/windowsFeatureUpdateProfiles, Microsoft
// Graph beta).
func NewWindowsFeatureUpdateProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/windowsFeatureUpdateProfiles",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Windows feature update profile that controls the targeted Windows feature update version.",
			KeySettings: []string{
				"featureUpdateVersion",
				"rolloutSettings (gradual rollouts use offer groups of at least 100 devices; devices assigned after the final offer date get the offer immediately)",
				"installLatestWindows10OnWindows11IneligibleDevice",
				"installFeatureUpdatesOptional",
				"endOfSupportDate",
			},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"Holds devices on the specified feature version and never downgrades; track endOfSupportDate, versions shown as not supported no longer deploy.",
				"When no feature update policy applies any more, the device stays enrolled in Windows Autopatch and gets no feature update until it is assigned a new profile or is unenrolled.",
				"Requires Intune Plan 1 and a Windows license with the Autopatch entitlement (Pro, Enterprise or Education, not LTSC), Entra joined or hybrid joined devices sending at least Required diagnostic data, and update rings with feature update deferral 0.",
				"Devices also need the Microsoft Account Sign-In Assistant service (wlidsvc) enabled and running; assignment filters aren't supported for feature update policies.",
				"Status shows in Reports > Windows Updates (Windows Feature Update Report) and alerts in Devices > Monitor > Feature update failures; client-side data (install steps, client alerts) needs the tenant setting Enable features that require Windows diagnostic data in processor configuration; service-side data doesn't.",
				"Managing these policies needs at least the Policy and Profile Manager role or a custom role with Device configurations permissions plus read access to managed devices (e.g. Organization/Read, Managed devices/Read); viewing their reports needs a role such as Read Only Operator or Help Desk Operator.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/deviceConfigurations (update rings: featureUpdatesDeferralPeriodInDays, featureUpdatesPaused)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-softwareupdate-windowsfeatureupdateprofile?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-softwareupdate-windowsfeatureupdateprofile-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-softwareupdate-windowsupdaterolloutsettings?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-updates/windows/manage-feature-updates",
					"https://learn.microsoft.com/en-us/intune/device-updates/windows/configure-feature-update-policy",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsFeatureUpdateProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows feature update profiles: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsFeatureUpdateProfiles().ByWindowsFeatureUpdateProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows feature update profile: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().WindowsFeatureUpdateProfiles().ByWindowsFeatureUpdateProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/windowsFeatureUpdateProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.WindowsFeatureUpdateProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
