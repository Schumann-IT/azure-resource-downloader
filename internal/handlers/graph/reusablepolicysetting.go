package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewReusablePolicySettingHandler creates a handler for Intune reusable policy
// settings (deviceManagement/reusablePolicySettings, Microsoft Graph beta).
// These reusable settings (e.g. firewall rule groups, certificates) are
// referenced by ID from Endpoint Security / Settings Catalog policies, so
// exporting them keeps those references resolvable. A plain GET already returns
// the full settingInstance tree.
func NewReusablePolicySettingHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/reusablePolicySettings",
		documentation: models.ResourceDocumentation{
			Purpose:          "An Intune reusable policy setting (e.g. reusable certificate/trusted-root settings) referenced by multiple policies.",
			EmbeddedPayloads: []string{"settingInstance"},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().ReusablePolicySettings()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list reusable policy settings: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().ReusablePolicySettings().ByDeviceManagementReusablePolicySettingId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get reusable policy setting: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(betamodels.DeviceManagementReusablePolicySettingable); ok {
				return safeStringValue(s.GetDisplayName())
			}
			return ""
		},
	}, nil
}
