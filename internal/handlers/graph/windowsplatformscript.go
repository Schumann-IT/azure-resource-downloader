package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsPlatformScriptHandler creates a handler for Intune Windows platform
// scripts (deviceManagement/deviceManagementScripts, Microsoft Graph beta).
// The base64 `scriptContent` is decoded by the base64-decode transformer
// (inline by default, or to a .ps1 sidecar file in file mode).
func NewWindowsPlatformScriptHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/deviceManagementScripts",
		terraformType: "microsoft365_graph_beta_device_management_windows_platform_script",
		documentation: docMeta(
			"An Intune Windows PowerShell platform script run on managed devices.",
			[]string{"runAsAccount", "enforceSignatureCheck", "runAs32Bit"},
			[]string{"scriptContent (base64 PowerShell)"},
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceManagementScripts()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows platform scripts: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceManagementScripts().ByDeviceManagementScriptId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows platform script: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().DeviceManagementScripts().ByDeviceManagementScriptId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceManagementScripts", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(betamodels.DeviceManagementScriptable); ok {
				return safeStringValue(s.GetDisplayName())
			}
			return ""
		},
	}, nil
}
