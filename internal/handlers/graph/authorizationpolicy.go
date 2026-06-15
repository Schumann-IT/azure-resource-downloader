package graph

import (
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
//
// There is no Terraform resource for the authorization policy in the project's
// providers, so no Terraform import is emitted.
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
		azureType:     "Microsoft.Graph/authorizationPolicy",
		terraformType: "",
		documentation: docMeta(
			"The tenant Entra ID authorization policy controlling default user permissions and self-service capabilities.",
			[]string{"defaultUserRolePermissions", "allowedToUseSSPR", "guestUserRoleId"},
			nil,
		),
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
