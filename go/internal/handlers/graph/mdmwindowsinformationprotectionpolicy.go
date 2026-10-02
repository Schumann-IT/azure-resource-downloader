package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewMdmWindowsInformationProtectionPolicyHandler creates a handler for
// Windows Information Protection policies for MDM-enrolled devices
// (deviceAppManagement/mdmWindowsInformationProtectionPolicies, Microsoft
// Graph beta). WIP is deprecated by Microsoft but may still exist in tenants.
func NewMdmWindowsInformationProtectionPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/mdmWindowsInformationProtectionPolicies",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An MDM-enrolled Windows Information Protection (WIP) policy controlling data separation between work and personal data.",
			KeySettings: []string{
				"enforcementLevel (noProtection is the Off mode: WIP neither protects nor audits data, and switching a policy to Off makes devices try to decrypt WIP-tagged files on locally attached drives)",
				"protectedApps",
				"exemptApps",
				"enterpriseProtectedDomainNames",
				"dataRecoveryCertificate",
				"revokeOnUnenrollDisabled",
				"enterpriseIPRanges",
				"enterpriseNetworkDomainNames",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Microsoft began deprecating Windows Information Protection in July 2022 and removed it starting with Windows 11, version 24H2; Microsoft recommends Microsoft Purview Information Protection and Microsoft Purview Data Loss Prevention instead.",
				"Unassigning the policy (Learn's way to turn WIP off) makes devices remove WIP file protection and the WIP configuration.",
				"If one user and device are targeted by both an MDM WIP policy and a WIP without enrollment (MAM) policy, the MDM policy applies on Microsoft Entra joined devices and the MAM policy is preferred on workplace-joined personal devices; Windows Home supports only WIP without enrollment.",
			},
			EmbeddedPayloads: []string{"dataRecoveryCertificate.certificate (base64 Data Recovery Agent certificate)"},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/windowsInformationProtectionPolicies (WIP without enrollment; the MDM policy wins on Microsoft Entra joined devices targeted by both)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-mdmwindowsinformationprotectionpolicy?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-mam-mdmwindowsinformationprotectionpolicy-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/information-protection/windows-information-protection/how-to-disable-wip",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().MdmWindowsInformationProtectionPolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list MDM Windows Information Protection policies: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceAppManagement().MdmWindowsInformationProtectionPolicies().ByMdmWindowsInformationProtectionPolicyId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get MDM Windows Information Protection policy: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().MdmWindowsInformationProtectionPolicies().ByMdmWindowsInformationProtectionPolicyId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/mdmWindowsInformationProtectionPolicies", itemID, err)
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
