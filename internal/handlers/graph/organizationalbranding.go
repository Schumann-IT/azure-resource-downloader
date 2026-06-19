package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betaorganization "github.com/microsoftgraph/msgraph-beta-sdk-go/organization"
)

// organizationalBrandingFallbackName names the organizational branding
// singleton output when the branding object carries no usable identifier.
const organizationalBrandingFallbackName = "Organizational Branding"

// NewOrganizationalBrandingHandler creates a handler for the Entra
// organizational (company) branding (organization/{id}/branding, Microsoft
// Graph beta), including its per-locale localizations (via $expand).
//
// This is a tenant **singleton** scoped to the organization: List resolves the
// organization ID, probes the default branding object and returns at most one
// ID, and Fetch retrieves the branding regardless of the requested ID. When no
// branding has been configured the Graph API returns an empty body, so List
// yields no IDs and nothing is exported.
//
// There is no Terraform resource for organizational branding in the project's
// providers, so no Terraform import is emitted.
func NewOrganizationalBrandingHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	getOrganizationID := func(ctx context.Context) (string, error) {
		resp, err := client.Organization().Get(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("failed to list organization: %w (hint: requires 'Organization.Read.All' permission in Microsoft Graph)", err)
		}
		if resp != nil {
			for _, item := range resp.GetValue() {
				if item.GetId() != nil && *item.GetId() != "" {
					return *item.GetId(), nil
				}
			}
		}
		return "", fmt.Errorf("no organization found in tenant")
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/organizationalBranding",
		terraformType: "",
		documentation: docMeta(
			"The Entra ID company branding shown on sign-in pages.",
			nil,
			[]string{"backgroundImage / bannerLogo / squareLogo (base64 images)", "signInPageText", "usernameHintText"},
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			orgID, err := getOrganizationID(ctx)
			if err != nil {
				return nil, err
			}
			branding, err := client.Organization().ByOrganizationId(orgID).Branding().Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get organizational branding: %w (hint: requires 'OrganizationalBranding.Read.All' permission in Microsoft Graph)", err)
			}
			if branding == nil {
				return nil, nil
			}
			if branding.GetId() != nil && *branding.GetId() != "" {
				return []string{*branding.GetId()}, nil
			}
			return []string{"organizationalBranding"}, nil
		},
		fetchItem: func(ctx context.Context, _ string) (serialization.Parsable, error) {
			orgID, err := getOrganizationID(ctx)
			if err != nil {
				return nil, err
			}
			requestConfig := &betaorganization.ItemBrandingRequestBuilderGetRequestConfiguration{
				QueryParameters: &betaorganization.ItemBrandingRequestBuilderGetQueryParameters{
					Expand: []string{"localizations"},
				},
			}
			item, err := client.Organization().ByOrganizationId(orgID).Branding().Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get organizational branding: %w (hint: requires 'OrganizationalBranding.Read.All' permission in Microsoft Graph)", err)
			}
			if item == nil {
				return nil, fmt.Errorf("organizational branding is not configured")
			}
			return item, nil
		},
		displayName: func(_ serialization.Parsable) string {
			return organizationalBrandingFallbackName
		},
	}, nil
}
