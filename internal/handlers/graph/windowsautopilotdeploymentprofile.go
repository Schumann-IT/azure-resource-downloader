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
		azureType: "Microsoft.Graph/windowsAutopilotDeploymentProfiles",
		documentation: models.ResourceDocumentation{
			Purpose:             "A Windows Autopilot deployment profile that defines the out-of-box experience (OOBE) for provisioning Windows devices.",
			KeySettings:         []string{"outOfBoxExperienceSettings", "deviceType", "enrollmentStatusScreenSettings"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           []string{"Profile changes apply at the device next Autopilot deployment (not retroactively); deleting a profile unassigns it from registered devices."},
			RelatedTypes:        []string{"Microsoft.Graph/windowsAutopilotDeviceIdentities (registered devices)", "Microsoft.Graph/deviceEnrollmentConfigurations (Enrollment Status Page)", "Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-windowsautopilotdeploymentprofile?view=graph-rest-beta",
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
