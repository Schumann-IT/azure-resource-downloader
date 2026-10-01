package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsAutopilotDeploymentProfileHandler creates a handler for Windows
// Autopilot deployment profiles
// (deviceManagement/windowsAutopilotDeploymentProfiles, Microsoft Graph beta).
func NewWindowsAutopilotDeploymentProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/windowsAutopilotDeploymentProfiles",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose:             "A Windows Autopilot deployment profile that defines the out-of-box experience (OOBE) for provisioning Windows devices.",
			KeySettings: []string{
	"outOfBoxExperienceSetting",
	"deviceType",
	"deviceNameTemplate",
	"preprovisioningAllowed",
	"hardwareHashExtractionEnabled",
	"locale",
	"hybridAzureADJoinSkipConnectivityCheck",
},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
	"A profile assigned to groups can't be deleted; unassign all groups first.",
	"Profile changes reach a device already enrolled in Intune only after it is reset and re-enrolled; if several profiles target a device, the oldest one wins.",
	"Windows Autopilot device preparation is a separate solution (Settings Catalog based, Entra join only) that needs no device registration.",
},
			RelatedTypes: []string{
	"Microsoft.Graph/windowsAutopilotDeviceIdentities (registered devices)",
	"Microsoft.Graph/deviceEnrollmentConfigurations (Enrollment Status Page)",
	"Microsoft.Graph/groups (assignment target groups)",
	"Microsoft.Graph/deviceConfigurations (hybrid join needs a Domain Join profile assigned to the same group)",
	"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
},
			SubtypeNote: "azureADWindowsAutopilotDeploymentProfile (Entra join) adds no properties; activeDirectoryWindowsAutopilotDeploymentProfile is hybrid join and adds hybridAzureADJoinSkipConnectivityCheck and a Domain Join profile relationship - identify the join type from @odata.type.",
Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-windowsautopilotdeploymentprofile?view=graph-rest-beta",
				Permissions: "https://learn.microsoft.com/en-us/graph/api/intune-enrollment-azureadwindowsautopilotdeploymentprofile-list?view=graph-rest-beta",
			SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-outofboxexperiencesetting?view=graph-rest-beta",
BestPractices: []string{
		"https://learn.microsoft.com/en-us/autopilot/profiles",
		"https://learn.microsoft.com/en-us/autopilot/windows-autopilot-hybrid",
	},
},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsAutopilotDeploymentProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Autopilot deployment profiles: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsAutopilotDeploymentProfiles().ByWindowsAutopilotDeploymentProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Autopilot deployment profile: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().WindowsAutopilotDeploymentProfiles().ByWindowsAutopilotDeploymentProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/windowsAutopilotDeploymentProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.WindowsAutopilotDeploymentProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
