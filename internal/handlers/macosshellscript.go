package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewMacOSShellScriptHandler creates a handler for Intune macOS shell scripts
// (deviceManagement/deviceShellScripts, Microsoft Graph beta). The base64
// `scriptContent` is decoded by the base64-decode transformer (inline by
// default, or to a .sh sidecar file in file mode).
func NewMacOSShellScriptHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/deviceShellScripts",
		terraformType: "microsoft365_graph_beta_device_management_macos_platform_script",
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceShellScripts()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list macOS shell scripts: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceShellScripts().ByDeviceShellScriptId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get macOS shell script: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(betamodels.DeviceShellScriptable); ok {
				return safeStringValue(s.GetDisplayName())
			}
			return ""
		},
	}, nil
}
