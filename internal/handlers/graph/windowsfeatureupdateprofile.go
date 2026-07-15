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
		azureType: "Microsoft.Graph/windowsFeatureUpdateProfiles",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune Windows feature update profile that controls the targeted Windows feature update version.",
			KeySettings:         []string{"featureUpdateVersion", "rolloutSettings", "installLatestWindows10OrHigher"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           "Holds devices on the specified Windows feature version; deleting or unassigning releases devices to other update policies. Track end-of-service dates of the pinned version.",
			RelatedTypes:        []string{"Microsoft.Graph/windowsQualityUpdateProfiles", "Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-softwareupdate-windowsfeatureupdateprofile?view=graph-rest-beta",
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
