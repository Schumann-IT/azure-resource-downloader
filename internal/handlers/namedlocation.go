package handlers

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewNamedLocationHandler creates a handler for Entra conditional access named
// locations (identity/conditionalAccess/namedLocations, Microsoft Graph beta).
// The collection is polymorphic (ipNamedLocation / countryNamedLocation).
func NewNamedLocationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/namedLocations",
		terraformType: "microsoft365_graph_beta_identity_and_access_named_location",
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.Identity().ConditionalAccess().NamedLocations()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list named locations: %w (hint: requires 'Policy.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.Identity().ConditionalAccess().NamedLocations().ByNamedLocationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get named location: %w (hint: requires 'Policy.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if l, ok := item.(betamodels.NamedLocationable); ok {
				return safeStringValue(l.GetDisplayName())
			}
			return ""
		},
	}, nil
}
