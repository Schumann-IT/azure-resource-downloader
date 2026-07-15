package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// NewConditionalAccessPolicyHandler creates a handler for Entra conditional
// access policies (identity/conditionalAccess/policies, Microsoft Graph v1.0).
func NewConditionalAccessPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/conditionalAccessPolicies",
		documentation: docMeta(
			"An Entra ID Conditional Access policy that enforces access controls based on signals (users, apps, conditions).",
			[]string{"state", "grantControls.builtInControls", "conditions.users"},
			[]string{"conditions (users, applications, platforms, locations, risk levels)", "grantControls", "sessionControls"},
			models.ResourceLinks{},
		),
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
