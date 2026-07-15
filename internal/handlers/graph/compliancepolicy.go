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

// NewCompliancePolicyHandler creates a handler for Settings Catalog based
// Intune compliance policies (deviceManagement/compliancePolicies, Microsoft
// Graph beta) — currently used for Linux compliance.
//
// Fetch uses $expand=settings,scheduledActionsForRule($expand=scheduledActionConfigurations)
// so the full setting tree and the noncompliance action rules are included
// (neither is returned by a plain GET). Like Settings Catalog configuration
// policies, these use `name` instead of `displayName`.
func NewCompliancePolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/compliancePolicies",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune (settings-catalog based) device compliance policy, e.g. for Linux.",
			EmbeddedPayloads:    []string{"settings (settingInstance values)"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           []string{"Compliance re-evaluates at the next device check-in; deleting or unassigning the policy can change device compliance state and Conditional Access outcomes."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)", "Microsoft.Graph/assignmentFilters (assignment filters)", "Microsoft.Graph/notificationMessageTemplates (noncompliance notifications)", "Microsoft.Graph/conditionalAccessPolicies (compliance-based grants)"},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfigv2-devicemanagementcompliancepolicy?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/mem/intune/protect/device-compliance-get-started"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().CompliancePolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list compliance policies: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadevicemanagement.CompliancePoliciesDeviceManagementCompliancePolicyItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.CompliancePoliciesDeviceManagementCompliancePolicyItemRequestBuilderGetQueryParameters{
					Expand: []string{"settings", "scheduledActionsForRule($expand=scheduledActionConfigurations)"},
				},
			}
			item, err := client.DeviceManagement().CompliancePolicies().ByDeviceManagementCompliancePolicyId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get compliance policy: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().CompliancePolicies().ByDeviceManagementCompliancePolicyId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/compliancePolicies", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.DeviceManagementCompliancePolicyable); ok {
				return safeStringValue(p.GetName())
			}
			return ""
		},
	}, nil
}
