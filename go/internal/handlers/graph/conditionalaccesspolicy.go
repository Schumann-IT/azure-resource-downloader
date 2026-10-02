package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	_ "embed"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	graphidentity "github.com/microsoftgraph/msgraph-sdk-go/identity"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// conditionalAccessPromptTemplateText overrides the default documentation
// prompt template for Conditional Access policies: a policy targets users,
// groups, roles, applications and sign-in conditions through conditions.*, not
// through Intune-style assignments, so its targeting gets a Conditions section
// and the document carries no assignments block or markers. See
// models.ResourceDocumentation.Template.
//
//go:embed conditional_access_prompt.tmpl
var conditionalAccessPromptTemplateText string

// NewConditionalAccessPolicyHandler creates a handler for Entra conditional
// access policies (identity/conditionalAccess/policies, Microsoft Graph v1.0).
func NewConditionalAccessPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/conditionalAccessPolicies",
		documentation: models.ResourceDocumentation{
			Purpose: "An Entra ID Conditional Access policy that enforces access controls based on signals (users, apps, conditions).",
			KeySettings: []string{
				"state",
				"conditions.users",
				"conditions.applications",
				"conditions.locations (AllTrusted also covers the MFA trusted IPs of the legacy multifactor authentication settings, if configured)",
				"grantControls.builtInControls",
				"grantControls.authenticationStrength",
				"sessionControls (persistentBrowser requires All resources as the target)",
			},

			RequiredPermissions: []string{"Policy.Read.All"},
			Lifecycle: []string{
				"Validate new policies in report-only mode for at least a week before enforcing, and always exclude emergency-access (break-glass) accounts.",
				"Report-only results are logged in the Conditional Access and Report-only tabs of the sign-in log details; the Conditional Access insights and reporting workbook additionally needs the sign-in logs sent to a Log Analytics workspace.",
				"Report-only isn't always invisible to users: a report-only policy that requires a compliant device can prompt macOS, iOS and Android users to select a device certificate, and one using GPS-based named locations prompts users to share their location, where not sharing may result in a block.",
				"In general every enabled policy that applies to a sign-in must be satisfied (for example MFA from one and a compliant device from another), and a policy that applies with Block access blocks the sign-in whatever the others grant.",
				"Policy changes can take up to a day (about two hours where optimised) to reach resource providers such as Exchange Online and SharePoint Online; revoke a user's sessions to apply a change immediately.",
				"Deleted policies are soft-deleted and can be restored within 30 days.",
				"Policies whose display name starts with 'Microsoft-managed:' are created by Microsoft in report-only mode and turned on by Microsoft if left there, normally no less than 30 days later; Microsoft may update them, admins can change only their state and exclusions, and they can't be renamed or deleted.",
				"Requires Microsoft Entra ID P1 (risk-based conditions need P2); when licenses expire, policies stay in force but can no longer be edited.",
				"The approved client app grant (approvedApplication) is retired: since 30 June 2026 policies that use it are read-only, stay enforced while enabled and can still be disabled or deleted; new policies use the app protection policy grant (compliantApplication).",
				"Custom controls (grantControls.customAuthenticationFactors) are deprecated: since September 2026 new custom controls can't be created and existing ones can't be edited; full retirement is scheduled for early 2027, and external MFA is the replacement.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/namedLocations",
				"Microsoft.Graph/authenticationStrengthPolicies",
				"Microsoft.Graph/termsOfUseAgreements",
				"Microsoft.Graph/groups (include/exclude targets)",
				"Microsoft.Graph/deviceCompliancePolicies (compliantDevice grant)",
				"Microsoft.Graph/compliancePolicies (compliantDevice grant)",
				"Microsoft.Graph/iosManagedAppProtections (compliantApplication grant)",
				"Microsoft.Graph/androidManagedAppProtections (compliantApplication grant)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/conditionalaccesspolicy?view=graph-rest-1.0",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/conditionalaccessroot-list-policies?view=graph-rest-1.0",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/entra/identity/conditional-access/plan-conditional-access",
					"https://learn.microsoft.com/en-us/entra/identity/conditional-access/concept-conditional-access-policy-common",
				},
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/conditionalaccessconditionset?view=graph-rest-1.0",
				AdminCenter:     "https://entra.microsoft.com/#view/Microsoft_AAD_ConditionalAccess/ConditionalAccessBlade/~/Policies",
			},
			Template: conditionalAccessPromptTemplateText,
		},
		probe: func(ctx context.Context) error {
			_, err := client.Identity().ConditionalAccess().Policies().Get(ctx, &graphidentity.ConditionalAccessPoliciesRequestBuilderGetRequestConfiguration{
				QueryParameters: &graphidentity.ConditionalAccessPoliciesRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list conditional access policies: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.ConditionalAccess' permission in Microsoft Graph)", err)
			}
			return nil
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.Identity().ConditionalAccess().Policies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list conditional access policies: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.ConditionalAccess' permission in Microsoft Graph)", err)
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
			item, err := client.Identity().ConditionalAccess().Policies().ByConditionalAccessPolicyId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get conditional access policy: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.ConditionalAccess' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(msgraphmodels.ConditionalAccessPolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
