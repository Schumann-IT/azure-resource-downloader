package graph

import (
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
//
// There is no Terraform resource for the authentication methods policy in the
// project's providers, so no Terraform import is emitted.
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
		azureType:     "Microsoft.Graph/authenticationMethodsPolicy",
		terraformType: "",
		documentation: docMeta(
			"The tenant Entra ID authentication methods policy controlling which authentication methods are enabled and how.",
			[]string{"authenticationMethodConfigurations", "registrationEnforcement"},
			nil,
		),
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
