package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewDeviceComplianceScriptHandler creates a handler for Intune Windows custom
// compliance scripts (deviceManagement/deviceComplianceScripts, Microsoft Graph
// beta). These are distinct from Remediations (deviceHealthScripts): each item
// carries a single base64 detection script (detectionScriptContent), decoded by
// the base64-decode transformer (inline by default, or to a *_detection.ps1
// sidecar file in file mode).
func NewDeviceComplianceScriptHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceComplianceScripts",
		documentation: docMeta(
			"An Intune custom compliance (device compliance) script used to evaluate custom compliance settings.",
			[]string{"runAsAccount", "enforceSignatureCheck"},
			[]string{"detectionScriptContent (base64 PowerShell)"},
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceComplianceScripts()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device compliance scripts: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceComplianceScripts().ByDeviceComplianceScriptId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get device compliance script: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().DeviceComplianceScripts().ByDeviceComplianceScriptId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceComplianceScripts", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(betamodels.DeviceComplianceScriptable); ok {
				return safeStringValue(s.GetDisplayName())
			}
			return ""
		},
	}, nil
}
