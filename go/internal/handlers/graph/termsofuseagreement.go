package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betaidentitygovernance "github.com/microsoftgraph/msgraph-beta-sdk-go/identitygovernance"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewTermsOfUseAgreementHandler creates a handler for Entra terms of use
// agreements (identityGovernance/termsOfUse/agreements, Microsoft Graph beta).
func NewTermsOfUseAgreementHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/termsOfUseAgreements",
		documentation: models.ResourceDocumentation{
			Template: referencedPromptTemplateText,
			Purpose:  "An Entra ID Terms of Use agreement presented via Conditional Access.",
			KeySettings: []string{
				"isViewingBeforeAcceptanceRequired",
				"isPerDeviceAcceptanceRequired",
				"userReacceptRequiredFrequency",
				"termsExpiration",
			},

			RequiredPermissions: []string{"Agreement.Read.All"},
			Lifecycle: []string{
				"Enforced through Conditional Access grant controls; a new document version forces re-acceptance only when 'Require reaccept' is set, and re-acceptance can also be scheduled (userReacceptRequiredFrequency).",
				"Acceptances are kept for the life of the terms of use; deleting it, or the tenant losing Entra ID P1/P2, deletes all of its acceptance records.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/conditionalAccessPolicies (terms-of-use grants)",
				"Microsoft.Graph/termsAndConditions (Intune terms and conditions; both must be accepted when both apply)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/agreement?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/termsofusecontainer-list-agreements?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/entra/identity/conditional-access/terms-of-use"},
			},
		},
		probe: func(ctx context.Context) error {
			_, err := client.IdentityGovernance().TermsOfUse().Agreements().Get(ctx, &betaidentitygovernance.TermsOfUseAgreementsRequestBuilderGetRequestConfiguration{
				QueryParameters: &betaidentitygovernance.TermsOfUseAgreementsRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list terms of use agreements: %w (hint: requires 'Agreement.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.IdentityGovernance().TermsOfUse().Agreements()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list terms of use agreements: %w (hint: requires 'Agreement.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.IdentityGovernance().TermsOfUse().Agreements().ByAgreementId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get terms of use agreement: %w (hint: requires 'Agreement.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if a, ok := item.(betamodels.Agreementable); ok {
				return safeStringValue(a.GetDisplayName())
			}
			return ""
		},
	}, nil
}
