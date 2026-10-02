package graph

import (
	"azure-resource-downloader/internal/models"
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
func NewOnPremisesSynchronizationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/onPremisesSynchronization",
		documentation: models.ResourceDocumentation{
			Template: singletonPromptTemplateText,
			Purpose:  "The tenant's Microsoft Entra on-premises directory synchronization settings (Microsoft Entra Connect): synchronization feature flags and configuration such as accidental-deletion prevention.",
			KeySettings: []string{
				"features (e.g. blockSoftMatchEnabled, blockCloudObjectTakeoverThroughHardMatchEnabled, passwordSyncEnabled)",
				"configuration (e.g. accidentalDeletionPrevention)",
			},
			RequiredPermissions: []string{"OnPremDirectorySynchronization.Read.All"},
			Lifecycle: []string{
				"Features and configuration can also be changed in the cloud through Microsoft Graph (PATCH /directory/onPremisesSynchronization/{id}); synchronizeUpnForManagedUsers can't be disabled once enabled.",
				"While features.blockSoftMatchEnabled is on, newly hybrid-joined devices fail to soft-match their synchronized computer objects (InvalidSoftMatch error); the flag has to be turned off temporarily for those joins to proceed.",
				"features.passwordWritebackEnabled isn't in use and can't be updated (password writeback is set up in Microsoft Entra Connect), and user writeback isn't currently supported.",
				"Quarantined duplicate UPN or proxy address conflicts are reported only once, in the Identity Synchronization Error Report email to the tenant's technical notification contact; open conflicts show in the Microsoft 365 admin center (users only) and through Microsoft Entra PowerShell.",
				"Reading it with delegated permissions requires the Global Administrator role (the only supported role).",
			},
			RelatedTypes: []string{"Microsoft.Graph/organization (same tenant; onPremisesSyncEnabled)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/onpremisesdirectorysynchronization?view=graph-rest-1.0",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/onpremisesdirectorysynchronization-get?view=graph-rest-1.0",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/entra/identity/hybrid/connect/how-to-connect-syncservice-features",
				},
			},
		},
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
