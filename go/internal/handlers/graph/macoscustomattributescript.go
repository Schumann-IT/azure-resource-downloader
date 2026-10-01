package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewMacOSCustomAttributeScriptHandler creates a handler for Intune macOS
// custom attribute shell scripts
// (deviceManagement/deviceCustomAttributeShellScripts, Microsoft Graph beta).
// The base64 `scriptContent` is decoded by the base64-decode transformer
// (inline by default, or to a .sh sidecar file in file mode).
func NewMacOSCustomAttributeScriptHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/deviceCustomAttributeShellScripts",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose:     "An Intune macOS custom attribute shell script that collects a custom attribute from managed Macs.",
			KeySettings: []string{"customAttributeType", "runAsAccount"},
			EmbeddedPayloads: []string{
				"scriptContent (shell script; base64 in Graph, decoded by the export's base64-decode transformer: inline by default, or into a .sh sidecar named after fileName in file mode)",
			},
			RequiredPermissions: []string{"DeviceManagementScripts.Read.All"},
			Lifecycle: []string{
				"Custom attribute scripts run on managed Macs about every 8 hours (there is no admin-set schedule) and report the echoed value; the value must match customAttributeType (integer, string, or ISO-8601 dateTime) and be 20 KB or less.",
				"Deploying one installs the Intune management agent for macOS.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-devices-devicecustomattributeshellscript?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-devices-devicecustomattributeshellscript-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-management/tools/run-shell-scripts-macos#custom-attributes-for-macos",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceCustomAttributeShellScripts()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list macOS custom attribute scripts: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceCustomAttributeShellScripts().ByDeviceCustomAttributeShellScriptId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get macOS custom attribute script: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
			}
			requestConfig := &betadevicemanagement.DeviceCustomAttributeShellScriptsDeviceCustomAttributeShellScriptItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.DeviceCustomAttributeShellScriptsDeviceCustomAttributeShellScriptItemRequestBuilderGetQueryParameters{
					Expand: []string{"assignments"},
				},
			}
			if expanded, err := client.DeviceManagement().DeviceCustomAttributeShellScripts().ByDeviceCustomAttributeShellScriptId(itemID).Get(ctx, requestConfig); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceCustomAttributeShellScripts", itemID, err)
			} else if expanded != nil {
				item.SetAssignments(expanded.GetAssignments())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(betamodels.DeviceCustomAttributeShellScriptable); ok {
				return safeStringValue(s.GetDisplayName())
			}
			return ""
		},
	}, nil
}
