package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// NewAuthenticationStrengthPolicyHandler creates a handler for Entra
// authentication strength policies (policies/authenticationStrengthPolicies,
// Microsoft Graph v1.0).
func NewAuthenticationStrengthPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/authenticationStrengthPolicies",
		terraformType: "azuread_authentication_strength_policy",
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.Policies().AuthenticationStrengthPolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list authentication strength policies: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.AuthenticationMethod' permission in Microsoft Graph)", err)
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
			item, err := client.Policies().AuthenticationStrengthPolicies().ByAuthenticationStrengthPolicyId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get authentication strength policy: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.AuthenticationMethod' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(msgraphmodels.AuthenticationStrengthPolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
