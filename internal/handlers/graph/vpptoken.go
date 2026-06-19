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
//
// The deploymenttheory/microsoft365 provider has no VPP token resource, so no
// Terraform import is emitted. Tokens are named by their admin friendly name,
// falling back to the organization name and then the Apple ID.
func NewVppTokenHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/vppTokens",
		terraformType: "",
		documentation: docMeta(
			"An Apple Volume Purchase Program (VPP / Apps and Books) token used by Intune to sync purchased apps.",
			[]string{"expirationDateTime", "appleId", "state", "automaticallyUpdateApps"},
			nil,
			models.ResourceLinks{},
		),
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
