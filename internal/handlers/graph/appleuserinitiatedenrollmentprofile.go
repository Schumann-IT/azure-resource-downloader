package graph

import (
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
//
// terraform-provider-microsoft365 models only the profile assignment
// (apple_user_initiated_enrollment_profile_assignment), not the profile
// itself, so no Terraform import is emitted.
func NewAppleUserInitiatedEnrollmentProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/appleUserInitiatedEnrollmentProfiles",
		terraformType: "",
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
