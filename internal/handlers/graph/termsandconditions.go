package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
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
		azureType: "Microsoft.Graph/termsAndConditions",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune Terms and Conditions policy presented to users at enrollment.",
			EmbeddedPayloads:    []string{"bodyText", "acceptanceStatement"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           []string{"Incrementing the version forces users to re-accept at next Company Portal sign-in; acceptance statuses are recorded per user."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-companyterms-termsandconditions?view=graph-rest-beta",
			},
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
