package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// authorizationPolicyFallbackName names the authorization policy singleton
// output when the policy carries no display name.
const authorizationPolicyFallbackName = "Authorization Policy"

// NewAuthorizationPolicyHandler creates a handler for the Entra authorization
// policy (policies/authorizationPolicy, Microsoft Graph v1.0), which covers
// tenant defaults such as user consent, guest invites and SSPR enablement.
// This is a tenant **singleton**: List probes the object and returns at most
// one pseudo-ID, and Fetch retrieves the singleton regardless of the requested
// ID.
func NewAuthorizationPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	getSingleton := func(ctx context.Context) (msgraphmodels.AuthorizationPolicyable, error) {
		policy, err := client.Policies().AuthorizationPolicy().Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get authorization policy: %w (hint: requires 'Policy.Read.All' permission in Microsoft Graph)", err)
		}
		return policy, nil
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/authorizationPolicy",
		documentation: models.ResourceDocumentation{
			Purpose:             "The tenant Entra ID authorization policy controlling default user permissions and self-service capabilities.",
			KeySettings:         []string{"defaultUserRolePermissions", "allowedToUseSSPR", "guestUserRoleId"},
			RequiredPermissions: []string{"Policy.Read.All"},
			Lifecycle:           []string{"Tenant-wide singleton controlling default user role permissions, guest access levels and consent defaults; changes apply tenant-wide immediately and should be reviewed regularly."},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/authorizationpolicy?view=graph-rest-1.0",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			policy, err := getSingleton(ctx)
			if err != nil {
				return nil, err
			}
			if policy == nil || policy.GetId() == nil || *policy.GetId() == "" {
				return []string{"authorizationPolicy"}, nil
			}
			return []string{*policy.GetId()}, nil
		},
		fetchItem: func(ctx context.Context, _ string) (serialization.Parsable, error) {
			return getSingleton(ctx)
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(msgraphmodels.AuthorizationPolicyable); ok {
				if name := safeStringValue(p.GetDisplayName()); name != "" {
					return name
				}
			}
			return authorizationPolicyFallbackName
		},
	}, nil
}
