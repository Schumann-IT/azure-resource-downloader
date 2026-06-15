package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// onPremisesSynchronizationName names the Entra Connect synchronization
// configuration output (the object itself carries no display name).
const onPremisesSynchronizationName = "Entra Connect Synchronization"

// NewOnPremisesSynchronizationHandler creates a handler for the Entra Connect
// (on-premises directory) synchronization configuration
// (directory/onPremisesSynchronization, Microsoft Graph v1.0). The collection
// holds at most one object per tenant; cloud-only tenants yield an empty list.
//
// There is no Terraform resource for the synchronization configuration, so no
// Terraform import is emitted.
func NewOnPremisesSynchronizationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/onPremisesSynchronization",
		terraformType: "",
		documentation: docMeta(
			"The tenant Entra ID on-premises directory synchronization (Azure AD Connect) configuration and features.",
			[]string{"features", "configuration"},
			nil,
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			resp, err := client.Directory().OnPremisesSynchronization().Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to list on-premises synchronization configuration: %w (hint: requires 'OnPremDirectorySynchronization.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.Directory().OnPremisesSynchronization().ByOnPremisesDirectorySynchronizationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get on-premises synchronization configuration: %w (hint: requires 'OnPremDirectorySynchronization.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if s, ok := item.(msgraphmodels.OnPremisesDirectorySynchronizationable); ok && s != nil {
				return onPremisesSynchronizationName
			}
			return ""
		},
	}, nil
}
