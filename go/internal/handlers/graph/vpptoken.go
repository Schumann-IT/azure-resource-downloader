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
			Template: credentialPromptTemplateText,
			Purpose:  "An Apple Volume Purchase Program (VPP / Apps and Books) token used by Intune to sync purchased apps.",
			KeySettings: []string{
				"expirationDateTime",
				"appleId",
				"state",
				"automaticallyUpdateApps (token-level switch: a VPP app assignment's Prevent automatic updates works only when it is true)",
				"lastSyncStatus",
				"lastSyncDateTime",
				"locationName",
				"vppTokenAccountType",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"Each token is valid for one year; renew it by downloading it again from Apple Business Manager or Apple School Manager and updating the existing token in Intune. It shows 'invalid' when it has expired or the Managed Apple ID changed.",
				"Deleting a token also deletes its apps and assignments and revokes their licenses without uninstalling the apps; a location token works with only one MDM tenant at a time.",
				"Token pitfalls: importing the same location token into another MDM after importing it to Intune can lose license assignments and user records, and because DDM doesn't yet support available app assignments, re-uploading a token that has available app assignments (to manage it with DDM) loses them.",
				"Intune syncs tokens with Apple daily (manual sync any time); Tenant status shows Warning within seven days of expiry or after a day without sync, Unhealthy once expired or after three days; duplicateLocationId means another token has the same location, and this one won't sync until the duplicate is removed.",
				"Treat the token value as a secret and redact it if present.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/mobileApps (VPP-licensed apps)",
				"Microsoft.Graph/depOnboardingSettings (ADE iOS profiles' companyPortalVppTokenId)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-vpptoken?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/intune-onboarding-vpptoken-list?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/intune/app-management/deployment/manage-vpp-apple"},
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
