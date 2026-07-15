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
		azureType: "Microsoft.Graph/deviceHealthScripts",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune Windows remediation script package (detection + remediation).",
			KeySettings:         []string{"runAsAccount", "enforceSignatureCheck", "runAs32Bit"},
			EmbeddedPayloads:    []string{"detectionScriptContent (base64 PowerShell)", "remediationScriptContent (base64 PowerShell)"},
			RequiredPermissions: []string{"DeviceManagementScripts.Read.All"},
			Lifecycle:           []string{"Remediations run detection on a schedule and remediate on failure; deleting a script stops the schedule but does not revert previous remediations."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-devices-devicehealthscript?view=graph-rest-beta",
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
