package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewDeviceCategoryHandler creates a handler for Intune device categories
// (deviceManagement/deviceCategories, Microsoft Graph beta).
func NewDeviceCategoryHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/deviceCategories",
		terraformType: "microsoft365_graph_beta_device_management_device_category",
		documentation: docMeta(
			"An Intune device category used to group and target devices at enrollment.",
			nil,
			nil,
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceCategories()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device categories: %w (hint: requires 'DeviceManagementManagedDevices.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceCategories().ByDeviceCategoryId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get device category: %w (hint: requires 'DeviceManagementManagedDevices.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.DeviceCategoryable); ok {
				return safeStringValue(c.GetDisplayName())
			}
			return ""
		},
	}, nil
}
