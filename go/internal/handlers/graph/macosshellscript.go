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
		azureType:      "Microsoft.Graph/deviceShellScripts",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune macOS shell script run on managed Macs.",
			KeySettings: []string{
				"runAsAccount (user: runs for every user signed in at run time and needs one signed in; otherwise as root)",
				"executionFrequency",
				"retryCount",
				"blockExecutionNotifications",
			},
			EmbeddedPayloads: []string{
				"scriptContent (shell script; base64 in Graph, decoded by the export's base64-decode transformer: inline by default, or into a .sh sidecar named after fileName in file mode)",
			},
			RequiredPermissions: []string{"DeviceManagementScripts.Read.All"},
			Lifecycle: []string{
				"Without executionFrequency the script runs once; with a frequency it also runs after restarts and may run more often. Failures are retried only when retryCount is set, and runs longer than 60 minutes are stopped and reported as failed.",
				"Requires the Intune management agent for macOS, which checks in about every 8 hours separately from MDM; deleting a script does not undo changes it already made on devices.",
				"Requires a direct internet connection; connection through a proxy isn't supported.",
				"The script must start with a #! line for an installed shell and be smaller than 1 MB; assignment filters aren't supported for shell scripts.",
				"Run results show in the script's Device status and User status reports and change only when the result changes (otherwise the timestamp refreshes every 7 days); agent logs are in /Library/Logs/Microsoft/Intune and ~/Library/Logs/Microsoft/Intune.",
				"Managing shell scripts needs an Intune role with Device configurations permissions.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-devices-deviceshellscript?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/intune-devices-deviceshellscript-list?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/intune/device-management/tools/run-shell-scripts-macos"},
			},
		},
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
			requestConfig := &betadevicemanagement.DeviceShellScriptsDeviceShellScriptItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.DeviceShellScriptsDeviceShellScriptItemRequestBuilderGetQueryParameters{
					Expand: []string{"assignments"},
				},
			}
			if expanded, err := client.DeviceManagement().DeviceShellScripts().ByDeviceShellScriptId(itemID).Get(ctx, requestConfig); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceShellScripts", itemID, err)
			} else if expanded != nil {
				item.SetAssignments(expanded.GetAssignments())
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
