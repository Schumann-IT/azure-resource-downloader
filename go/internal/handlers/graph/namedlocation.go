package graph

import (
	"azure-resource-downloader/internal/models"
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
		azureType: "Microsoft.Graph/namedLocations",
		documentation: models.ResourceDocumentation{
			Template: referencedPromptTemplateText,
			Purpose:  "An Entra ID named location (IP ranges, countries/regions or a compliant network) used as a condition in Conditional Access.",
			KeySettings: []string{
				"ipRanges",
				"isTrusted",
				"countriesAndRegions",
				"countryLookupMethod",
				"includeUnknownCountriesAndRegions",
			},
			RequiredPermissions: []string{"Policy.Read.All"},
			Lifecycle: []string{
				"Referenced by Conditional Access location conditions; a location marked trusted can't be deleted until the trusted mark is removed.",
				"Deleted locations are soft-deleted and can be restored within 30 days.",
				"Limits: at most 195 IP-based named locations with up to 2,000 ranges each and CIDR masks larger than /8; trusted locations also improve ID Protection risk calculation.",
				"countryLookupMethod authenticatorAppGps needs Microsoft Authenticator on the user's mobile device, which shares its GPS location every hour (also for report-only policies) and denies jailbroken devices; GPS location doesn't work when only passwordless methods are set.",
				"IP ranges are matched against the public IP address a sign-in comes from, not a device's private intranet address; behind a cloud proxy or VPN that is the proxy's address, because the X-Forwarded-For header isn't used (Global Secure Access source IP restoration avoids this).",
			},
			RelatedTypes: []string{"Microsoft.Graph/conditionalAccessPolicies"},
			SubtypeNote:  "Polymorphic: ipNamedLocation (ipRanges, isTrusted), countryNamedLocation (countriesAndRegions, countryLookupMethod, includeUnknownCountriesAndRegions; no trusted flag) and compliantNetworkNamedLocation (Global Secure Access compliant network) - identify the concrete type from @odata.type.",
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/namedlocation?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/conditionalaccessroot-list-namedlocations?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/entra/identity/conditional-access/concept-assignment-network",
				},
			},
		},
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
