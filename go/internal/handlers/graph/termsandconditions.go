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

// NewTermsAndConditionsHandler creates a handler for Intune Terms & Conditions
// (deviceManagement/termsAndConditions, Microsoft Graph beta).
func NewTermsAndConditionsHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/termsAndConditions",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Terms and Conditions policy presented to users at enrollment.",

			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
				"Users must accept in Company Portal before they can enroll devices or access protected resources; editing with 'Require users to re-accept' increments version and assigned users must accept again.",
				"Acceptance reports show the user, accepted version and time (up to 36 hours latency); acceptance is per user, not per device. Microsoft Entra terms of use offers stricter options, and users must accept both when both are configured.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/termsOfUseAgreements (Entra terms of use; both must be accepted when both apply)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			KeySettings: []string{"title", "bodyText", "acceptanceStatement", "version"},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-companyterms-termsandconditions?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/intune-companyterms-termsandconditions-list?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/intune/device-enrollment/create-terms-and-conditions"},
			},
		},
		probe: func(ctx context.Context) error {
			_, err := client.DeviceManagement().TermsAndConditions().Get(ctx, &betadevicemanagement.TermsAndConditionsRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.TermsAndConditionsRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list terms and conditions: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().TermsAndConditions()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list terms and conditions: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().TermsAndConditions().ByTermsAndConditionsId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get terms and conditions: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().TermsAndConditions().ByTermsAndConditionsId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/termsAndConditions", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if t, ok := item.(betamodels.TermsAndConditionsable); ok {
				return safeStringValue(t.GetDisplayName())
			}
			return ""
		},
	}, nil
}
