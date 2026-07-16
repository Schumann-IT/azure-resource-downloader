package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewVppTokenHandler creates a handler for Apple Volume Purchase Program (VPP)
// tokens (deviceAppManagement/vppTokens, Microsoft Graph beta), used to license
// store apps to macOS/iOS devices. The token secret itself is masked by the
// service; only metadata is exported.
func NewVppTokenHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/vppTokens",
		documentation: models.ResourceDocumentation{
			Template:            credentialPromptTemplateText,
			Purpose:             "An Apple Volume Purchase Program (VPP / Apps and Books) token used by Intune to sync purchased apps.",
			KeySettings:         []string{"expirationDateTime", "appleId", "state", "automaticallyUpdateApps"},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle:           []string{"Apple VPP tokens expire yearly and must be renewed in Apple Business Manager; an expired token blocks app license assignment and installs.", "The token secret is masked by the service."},
			RelatedTypes:        []string{"Microsoft.Graph/mobileApps (VPP-licensed apps)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-vpptoken?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().VppTokens()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list VPP tokens: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceAppManagement().VppTokens().ByVppTokenId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get VPP token: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			t, ok := item.(betamodels.VppTokenable)
			if !ok {
				return ""
			}
			if name := safeStringValue(t.GetDisplayName()); name != "" {
				return name
			}
			if org := safeStringValue(t.GetOrganizationName()); org != "" {
				return org
			}
			return safeStringValue(t.GetAppleId())
		},
	}, nil
}
