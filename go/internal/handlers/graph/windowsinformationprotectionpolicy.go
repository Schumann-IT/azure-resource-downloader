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
		azureType:      "Microsoft.Graph/windowsInformationProtectionPolicies",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "A Windows Information Protection (WIP) policy (without enrollment) controlling work/personal data separation.",
			KeySettings: []string{
				"enforcementLevel",
				"protectedApps",
				"exemptApps",
				"revokeOnMdmHandoffDisabled",
				"mdmEnrollmentUrl",
				"daysWithoutContactBeforeUnenroll",
				"dataRecoveryCertificate",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Microsoft began deprecating Windows Information Protection in July 2022 and removed it starting with Windows 11, version 24H2; Microsoft recommends Microsoft Purview Information Protection and Microsoft Purview Data Loss Prevention instead.",
				"Unassigning the policy (Learn's way to turn WIP off) makes devices remove WIP file protection and the WIP configuration.",
				"WIP without enrollment requires Microsoft Entra ID P1 or P2 and supports only one user per device.",
			},
			EmbeddedPayloads: []string{"dataRecoveryCertificate.certificate (base64 Data Recovery Agent certificate)"},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-windowsinformationprotectionpolicy?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-mam-windowsinformationprotectionpolicy-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/information-protection/windows-information-protection/how-to-disable-wip",
				},
			},
		},
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
