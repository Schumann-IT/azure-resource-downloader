package graph

import (
	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
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
// default branding has been configured Graph answers 404
// (Request_ResourceNotFound), so List yields no IDs: the type lists as empty
// (covered, nothing exported) rather than as "could not be listed".
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
		azureType: "Microsoft.Graph/organizationalBranding",
		documentation: models.ResourceDocumentation{
			Purpose:             "The Entra ID company branding shown on sign-in pages.",
			EmbeddedPayloads:    []string{"backgroundImage / bannerLogo / squareLogo (base64 images)", "signInPageText", "usernameHintText"},
			RequiredPermissions: []string{"OrganizationalBranding.Read.All", "Organization.Read.All"},
			Lifecycle:           []string{"Default Entra sign-in branding plus per-locale overrides; changes can take up to an hour to appear on the sign-in page."},
			RelatedTypes:        []string{"Microsoft.Graph/organization", "Microsoft.Graph/intuneBrandingProfiles (Company Portal branding)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/organizationalbranding?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			orgID, err := getOrganizationID(ctx)
			if err != nil {
				return nil, err
			}
			return brandingListIDs(client.Organization().ByOrganizationId(orgID).Branding().Get(ctx, nil))
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
			if status, ok := azure.HTTPStatus(err); ok && status == http.StatusNotFound {
				// Listed a moment ago, gone now: the branding was removed
				// between list and fetch. Not a permission problem, so no hint.
				return nil, fmt.Errorf("organizational branding is not configured: %w", err)
			}
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

// brandingListIDs decides what listing the default branding yields, given the
// Graph response. A 404 means no default branding is configured: the type
// lists as empty (no IDs, no error), which counts as covered. Any other error
// fails the listing, with the permission hint (ErrorSummary keeps the hint
// only for a 401/403). A nil body also lists as empty; otherwise the
// branding's ID, or a fixed pseudo-ID when it carries none.
func brandingListIDs(branding betamodels.OrganizationalBrandingable, err error) ([]string, error) {
	if err != nil {
		if status, ok := azure.HTTPStatus(err); ok && status == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get organizational branding: %w (hint: requires 'OrganizationalBranding.Read.All' permission in Microsoft Graph)", err)
	}
	if branding == nil {
		return nil, nil
	}
	if branding.GetId() != nil && *branding.GetId() != "" {
		return []string{*branding.GetId()}, nil
	}
	return []string{"organizationalBranding"}, nil
}
