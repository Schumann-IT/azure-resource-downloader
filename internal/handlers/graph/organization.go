package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// NewOrganizationHandler creates a handler for the Entra organization
// (tenant) information (organization, Microsoft Graph v1.0). The collection
// holds exactly one object per tenant.
//
// There is no Terraform resource for the organization object, so no Terraform
// import is emitted.
func NewOrganizationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/organization",
		terraformType: "",
		listIDs: func(ctx context.Context) ([]string, error) {
			resp, err := client.Organization().Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to list organization: %w (hint: requires 'Organization.Read.All' permission in Microsoft Graph)", err)
			}
			var ids []string
			if resp != nil {
				for _, item := range resp.GetValue() {
					if item.GetId() != nil {
						ids = append(ids, *item.GetId())
					}
				}
			}
			return ids, nil
		},
		fetchItem: func(ctx context.Context, itemID string) (serialization.Parsable, error) {
			item, err := client.Organization().ByOrganizationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get organization: %w (hint: requires 'Organization.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if o, ok := item.(msgraphmodels.Organizationable); ok {
				return safeStringValue(o.GetDisplayName())
			}
			return ""
		},
	}, nil
}
