package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsInformationProtectionPolicyHandler creates a handler for Windows
// Information Protection policies for devices without MDM enrollment
// (deviceAppManagement/windowsInformationProtectionPolicies, Microsoft Graph
// beta). WIP is deprecated by Microsoft but may still exist in tenants.
func NewWindowsInformationProtectionPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/windowsInformationProtectionPolicies",
		documentation: docMeta(
			"A Windows Information Protection (WIP) policy (without enrollment) controlling work/personal data separation.",
			[]string{"enforcementLevel", "protectedApps", "exemptApps"},
			nil,
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().WindowsInformationProtectionPolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows Information Protection policies: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceAppManagement().WindowsInformationProtectionPolicies().ByWindowsInformationProtectionPolicyId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows Information Protection policy: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().WindowsInformationProtectionPolicies().ByWindowsInformationProtectionPolicyId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/windowsInformationProtectionPolicies", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.ManagedAppPolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
