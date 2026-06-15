package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// NewGroupHandler creates a handler for Entra groups (groups, Microsoft Graph
// v1.0), including dynamic groups with their membership rules.
//
// NOTE: this exports the full directory group list, which can be very large in
// big tenants.
func NewGroupHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/groups",
		terraformType: "azuread_group",
		documentation: docMeta(
			"An Entra ID group (security or Microsoft 365), often used as an assignment target for policies and apps.",
			[]string{"groupTypes", "membershipRule (for dynamic groups)", "securityEnabled", "mailEnabled"},
			nil,
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.Groups()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list groups: %w (hint: requires 'Group.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.Groups().ByGroupId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get group: %w (hint: requires 'Group.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if g, ok := item.(msgraphmodels.Groupable); ok {
				return safeStringValue(g.GetDisplayName())
			}
			return ""
		},
	}, nil
}
