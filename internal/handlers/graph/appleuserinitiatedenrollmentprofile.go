package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewAppleUserInitiatedEnrollmentProfileHandler creates a handler for Apple
// user-initiated enrollment profiles
// (deviceManagement/appleUserInitiatedEnrollmentProfiles, Microsoft Graph
// beta).
func NewAppleUserInitiatedEnrollmentProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/appleUserInitiatedEnrollmentProfiles",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Apple user-initiated enrollment profile that controls account-driven vs. device enrollment for iOS/macOS.",
			KeySettings:         []string{"platform", "enrollmentType", "priority"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           "Profile changes affect only future user-initiated enrollments; already-enrolled devices keep their enrollment type.",
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-appleuserinitiatedenrollmentprofile?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().AppleUserInitiatedEnrollmentProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Apple user-initiated enrollment profiles: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().AppleUserInitiatedEnrollmentProfiles().ByAppleUserInitiatedEnrollmentProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Apple user-initiated enrollment profile: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().AppleUserInitiatedEnrollmentProfiles().ByAppleUserInitiatedEnrollmentProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/appleUserInitiatedEnrollmentProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.AppleUserInitiatedEnrollmentProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
