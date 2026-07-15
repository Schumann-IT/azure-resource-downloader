package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsAutopilotDeviceIdentityHandler creates a handler for Windows
// Autopilot device identities
// (deviceManagement/windowsAutopilotDeviceIdentities, Microsoft Graph beta).
//
// NOTE: this is registered device data rather than configuration and can be a
// large collection in big tenants. Identities often have no display name, so
// the serial number (or the object ID) is used as the file name fallback.
func NewWindowsAutopilotDeviceIdentityHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/windowsAutopilotDeviceIdentities",
		documentation: docMeta(
			"A Windows Autopilot device identity (hardware hash registration) for zero-touch provisioning.",
			nil,
			nil,
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsAutopilotDeviceIdentities()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Autopilot device identities: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsAutopilotDeviceIdentities().ByWindowsAutopilotDeviceIdentityId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Autopilot device identity: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			identity, ok := item.(betamodels.WindowsAutopilotDeviceIdentityable)
			if !ok {
				return ""
			}
			if name := safeStringValue(identity.GetDisplayName()); name != "" {
				return name
			}
			if serial := safeStringValue(identity.GetSerialNumber()); serial != "" {
				return serial
			}
			return safeStringValue(identity.GetId())
		},
	}, nil
}
