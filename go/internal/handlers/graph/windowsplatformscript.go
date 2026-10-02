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
		azureType:      "Microsoft.Graph/deviceManagementScripts",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose:     "An Intune Windows PowerShell platform script run on managed devices.",
			KeySettings: []string{"runAsAccount", "enforceSignatureCheck", "runAs32Bit"},
			EmbeddedPayloads: []string{
				"scriptContent (PowerShell script; base64 in Graph, decoded by the export's base64-decode transformer: inline by default, or into a .ps1 sidecar named after fileName in file mode)",
			},
			RequiredPermissions: []string{"DeviceManagementScripts.Read.All", "DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"Platform scripts run once per device or user and run again only when the script or its policy changes; a failed run is retried at the next three Intune Management Extension check-ins, and a run times out after 30 minutes. Scripts run before Win32 apps.",
				"Device-assigned scripts also run for each new user who signs in (not on multi-session SKUs); devices must be Microsoft Entra joined or hybrid joined, and scripts don't run on Windows Home, in S mode or on Surface Hub.",
				"Deleting a script does not undo changes it made; for recurring logic use Remediations (deviceHealthScripts).",
				"A script must be smaller than 200 KB (ASCII), and assignment filters aren't supported for platform scripts.",
				"Run results show in the script's Device status and User status reports (Monitor); on the device, AgentExecutor.log in C:\\ProgramData\\Microsoft\\IntuneManagementExtension\\Logs records each script run.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/deviceHealthScripts (Remediations, for recurring scripts)",
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-shared-devicemanagementscript?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-shared-devicemanagementscript-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-management/tools/run-powershell-scripts-windows",
				},
			},
		},
		probe: func(ctx context.Context) error {
			_, err := client.DeviceManagement().DeviceManagementScripts().Get(ctx, &betadevicemanagement.DeviceManagementScriptsRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.DeviceManagementScriptsRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list Windows platform scripts: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
		},
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
