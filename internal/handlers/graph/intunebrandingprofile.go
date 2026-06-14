package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewIntuneBrandingProfileHandler creates a handler for Intune branding
// profiles (deviceManagement/intuneBrandingProfiles, Microsoft Graph beta).
// Note: branding profiles use `profileName` instead of `displayName`.
func NewIntuneBrandingProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/intuneBrandingProfiles",
		terraformType: "microsoft365_graph_beta_device_management_intune_branding_profile",
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().IntuneBrandingProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Intune branding profiles: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().IntuneBrandingProfiles().ByIntuneBrandingProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Intune branding profile: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().IntuneBrandingProfiles().ByIntuneBrandingProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/intuneBrandingProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.IntuneBrandingProfileable); ok {
				return safeStringValue(p.GetProfileName())
			}
			return ""
		},
	}, nil
}
