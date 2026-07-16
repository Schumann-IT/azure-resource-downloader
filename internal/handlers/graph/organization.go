package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// NewOrganizationHandler creates a handler for the Entra organization
// (tenant) information (organization, Microsoft Graph v1.0). The collection
// holds exactly one object per tenant.
func NewOrganizationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/organization",
		documentation: models.ResourceDocumentation{
			Template:            singletonPromptTemplateText,
			Purpose:             "The Entra ID tenant (organization) profile and tenant-wide settings.",
			KeySettings:         []string{"verifiedDomains", "securityComplianceNotificationMails", "privacyProfile"},
			RequiredPermissions: []string{"Organization.Read.All"},
			Lifecycle:           []string{"Tenant information object; some properties are Microsoft-managed.", "Verified domains and technical notification contacts should be reviewed periodically."},
			RelatedTypes:        []string{"Microsoft.Graph/organizationalBranding"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/organization?view=graph-rest-1.0",
			},
		},
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
