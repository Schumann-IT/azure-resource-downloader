package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// authenticationMethodsPolicyFallbackName names the authentication methods
// policy singleton output when the policy carries no display name.
const authenticationMethodsPolicyFallbackName = "Authentication Methods Policy"

// NewAuthenticationMethodsPolicyHandler creates a handler for the Entra
// authentication methods policy (policies/authenticationMethodsPolicy,
// Microsoft Graph v1.0), including the per-method configurations. This is a
// tenant **singleton**: List probes the object and returns at most one
// pseudo-ID, and Fetch retrieves the singleton regardless of the requested ID.
func NewAuthenticationMethodsPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	getSingleton := func(ctx context.Context) (msgraphmodels.AuthenticationMethodsPolicyable, error) {
		policy, err := client.Policies().AuthenticationMethodsPolicy().Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get authentication methods policy: %w (hint: requires 'Policy.Read.All' permission in Microsoft Graph)", err)
		}
		return policy, nil
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/authenticationMethodsPolicy",
		documentation: models.ResourceDocumentation{
			Purpose:             "The tenant Entra ID authentication methods policy controlling which authentication methods are enabled and how.",
			KeySettings:         []string{"authenticationMethodConfigurations", "registrationEnforcement"},
			RequiredPermissions: []string{"Policy.Read.All"},
			Lifecycle:           []string{"Tenant-wide singleton controlling which authentication methods are enabled for MFA/SSPR/passwordless; changes take effect tenant-wide within minutes.", "Microsoft is migrating the legacy MFA and SSPR policies into this policy."},
			RelatedTypes:        []string{"Microsoft.Graph/conditionalAccessPolicies", "Microsoft.Graph/authenticationStrengthPolicies"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/authenticationmethodspolicy?view=graph-rest-1.0",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			policy, err := getSingleton(ctx)
			if err != nil {
				return nil, err
			}
			if policy == nil || policy.GetId() == nil || *policy.GetId() == "" {
				return []string{"authenticationMethodsPolicy"}, nil
			}
			return []string{*policy.GetId()}, nil
		},
		fetchItem: func(ctx context.Context, _ string) (serialization.Parsable, error) {
			return getSingleton(ctx)
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(msgraphmodels.AuthenticationMethodsPolicyable); ok {
				if name := safeStringValue(p.GetDisplayName()); name != "" {
					return name
				}
			}
			return authenticationMethodsPolicyFallbackName
		},
	}, nil
}
