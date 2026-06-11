package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
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
		azureType:     "Microsoft.Graph/termsOfUseAgreements",
		terraformType: "microsoft365_graph_identity_and_access_conditional_access_terms_of_use",
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
