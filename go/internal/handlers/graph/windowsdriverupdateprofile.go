package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsDriverUpdateProfileHandler creates a handler for Windows driver
// update profiles (deviceManagement/windowsDriverUpdateProfiles, Microsoft
// Graph beta).
func NewWindowsDriverUpdateProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/windowsDriverUpdateProfiles",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Windows driver update profile that controls how driver updates are approved and deployed.",
			KeySettings: []string{
				"approvalType (fixed once the policy is created)",
				"deploymentDeferralInDays (0 to 30 days, counted from when a driver is added to the policy; automatic approval only)",
			},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"Pausing applies to individual driver updates, is best effort and doesn't roll back completed installs; driver policies can't roll back drivers.",
				"If a device is in several driver policies, an Approved status in any of them wins, so assign each device to one policy only.",
				"With automatic approval only recommended drivers are approved (after deploymentDeferralInDays); other drivers wait in Needs review, so review approvals in both modes.",
				"Requires Intune Plan 1 and a Windows license with the Autopatch entitlement, Entra joined or hybrid joined devices and update rings that allow Windows drivers; assignment filters aren't supported.",
				"Devices must run Windows Pro, Pro Education, Enterprise or Education (not LTSC), send at least Required diagnostic data and have the Microsoft Account Sign-In Assistant service (wlidsvc) enabled and running.",
				"Update rings' quality update deadline, grace period and user experience settings apply to approved drivers, but their quality update deferral doesn't; driver policies don't apply during Autopilot, when Windows may still install critical drivers an admin hasn't approved.",
				"Status shows in Reports > Windows Updates (Windows Driver updates summary and Windows Driver Update Report) and failures in Devices > Monitor > Driver update policies with alerts; driver reporting needs the tenant setting Enable features that require Windows diagnostic data in processor configuration.",
				"Managing these policies needs at least the Policy and Profile Manager role or a custom role with Device configurations permissions plus read access to managed devices (e.g. Organization/Read, Managed devices/Read); viewing their reports needs a role such as Read Only Operator or Help Desk Operator.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/deviceConfigurations (update rings must not exclude drivers)",
				"Microsoft.Graph/deviceManagementConfigurationPolicies (the Settings Catalog setting 'Exclude WU Drivers in Quality Update' must allow drivers)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-softwareupdate-windowsdriverupdateprofile?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-softwareupdate-windowsdriverupdateprofile-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-updates/windows/configure-driver-update-policy",
					"https://learn.microsoft.com/en-us/intune/device-updates/windows/driver-updates-faq",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsDriverUpdateProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows driver update profiles: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsDriverUpdateProfiles().ByWindowsDriverUpdateProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows driver update profile: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().WindowsDriverUpdateProfiles().ByWindowsDriverUpdateProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/windowsDriverUpdateProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.WindowsDriverUpdateProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
