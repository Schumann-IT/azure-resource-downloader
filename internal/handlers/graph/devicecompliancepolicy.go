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

// NewDeviceCompliancePolicyHandler creates a handler for classic Intune device
// compliance policies (deviceManagement/deviceCompliancePolicies, Microsoft
// Graph beta). The collection is polymorphic per platform
// (windows10CompliancePolicy, macOSCompliancePolicy, iosCompliancePolicy, ...).
//
// Fetch uses $expand=scheduledActionsForRule($expand=scheduledActionConfigurations)
// so the noncompliance action rules are included (they are not returned by a
// plain GET).
func NewDeviceCompliancePolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceCompliancePolicies",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune device compliance policy that defines the rules a device must meet to be considered compliant.",
			KeySettings:         []string{"passwordRequired", "osMinimumVersion", "storageRequireEncryption", "scheduledActionsForRule (grace period and actions)"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           []string{"Compliance re-evaluates at each device check-in; deleting or unassigning the policy changes device compliance state and can alter Conditional Access outcomes.", "scheduledActionsForRule defines grace periods and noncompliance actions."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)", "Microsoft.Graph/assignmentFilters (assignment filters)", "Microsoft.Graph/notificationMessageTemplates (noncompliance notifications)", "Microsoft.Graph/conditionalAccessPolicies (compliance-based grants)"},
			SubtypeNote:         "Classic compliance policies are platform-polymorphic (windows10CompliancePolicy, iosCompliancePolicy, androidDeviceOwnerCompliancePolicy, ...) - identify the platform from @odata.type first.",
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-devicecompliancepolicy?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/mem/intune/protect/device-compliance-get-started"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceCompliancePolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device compliance policies: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadevicemanagement.DeviceCompliancePoliciesDeviceCompliancePolicyItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.DeviceCompliancePoliciesDeviceCompliancePolicyItemRequestBuilderGetQueryParameters{
					Expand: []string{"scheduledActionsForRule($expand=scheduledActionConfigurations)"},
				},
			}
			item, err := client.DeviceManagement().DeviceCompliancePolicies().ByDeviceCompliancePolicyId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get device compliance policy: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().DeviceCompliancePolicies().ByDeviceCompliancePolicyId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceCompliancePolicies", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.DeviceCompliancePolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
