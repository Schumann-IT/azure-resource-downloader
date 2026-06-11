package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsQualityUpdateProfileHandler creates a handler for Windows quality
// update profiles (deviceManagement/windowsQualityUpdateProfiles, Microsoft
// Graph beta).
func NewWindowsQualityUpdateProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/windowsQualityUpdateProfiles",
		terraformType: "microsoft365_graph_beta_device_management_windows_quality_update_policy",
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsQualityUpdateProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows quality update profiles: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsQualityUpdateProfiles().ByWindowsQualityUpdateProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows quality update profile: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.WindowsQualityUpdateProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
