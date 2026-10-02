package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsRemediationScriptHandler creates a handler for Intune Remediations
// (deviceManagement/deviceHealthScripts, Microsoft Graph beta). Each item
// carries a base64 detection + remediation script pair, decoded by the
// base64-decode transformer (inline by default, or to *_detection.ps1 /
// *_remediation.ps1 sidecar files in file mode).
func NewWindowsRemediationScriptHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/deviceHealthScripts",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Windows remediation script package (detection + remediation).",
			KeySettings: []string{
				"runAsAccount",
				"enforceSignatureCheck (on: scripts run under the device's PowerShell execution policy, Restricted by default on Windows clients, must be UTF-8 without BOM, and signed scripts need their certificate in Trusted Publishers; off: Bypass policy)",
				"runAs32Bit",
				"isGlobalScript",
				"assignments[].runSchedule",
				"assignments[].runRemediationScript",
			},
			EmbeddedPayloads: []string{
				"detectionScriptContent (PowerShell detection script; base64 in Graph, decoded by the export's base64-decode transformer: inline by default, or into <name>_detection.ps1 in file mode)",
				"remediationScriptContent (PowerShell remediation script; base64 in Graph, decoded by the export's base64-decode transformer: inline by default, or into <name>_remediation.ps1 in file mode)",
			},
			RequiredPermissions: []string{"DeviceManagementScripts.Read.All"},
			Lifecycle: []string{
				"Detection runs on the schedule set per assignment (runSchedule: once, hourly or daily); the remediation script runs only when detection exits with code 1 and runRemediationScript is enabled for that assignment.",
				"Requires Windows Enterprise E3/E5, Education A3/A5 or Windows VDA per user (licensing is confirmed once per tenant), Entra joined or hybrid joined devices and the Intune Management Extension; isGlobalScript marks read-only Microsoft-provided packages.",
				"A detection or remediation script's output is limited to 2,048 characters.",
				"Detection and remediation status shows under Devices > Manage devices > Scripts and remediations and per device in Device status (output exportable as CSV); devices fetch packages every 8 hours, and recurring results are reported on change and at least every 7 days.",
				"Managing Remediations needs Intune role permissions in the Device configurations category.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (remediations support filters)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-devices-devicehealthscript?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/intune-devices-devicehealthscript-list?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/intune/device-management/tools/deploy-remediations"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceHealthScripts()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list remediation scripts: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceHealthScripts().ByDeviceHealthScriptId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get remediation script: %w (hint: requires 'DeviceManagementScripts.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().DeviceHealthScripts().ByDeviceHealthScriptId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceHealthScripts", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(betamodels.DeviceHealthScriptable); ok {
				return safeStringValue(s.GetDisplayName())
			}
			return ""
		},
	}, nil
}
