package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// NewAuthenticationStrengthPolicyHandler creates a handler for Entra
// authentication strength policies (policies/authenticationStrengthPolicies,
// Microsoft Graph v1.0).
func NewAuthenticationStrengthPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/authenticationStrengthPolicies",
		documentation: models.ResourceDocumentation{
			Template:            referencedPromptTemplateText,
			Purpose:             "An Entra ID authentication strength (built-in or custom): a Conditional Access grant control listing the authentication-method combinations allowed to access a resource; requirementsSatisfied states whether it satisfies MFA.",
			KeySettings:         []string{"allowedCombinations", "policyType", "requirementsSatisfied", "combinationConfigurations"},
			RequiredPermissions: []string{"Policy.Read.All"},
			Lifecycle: []string{
				"Referenced by Conditional Access grant controls; built-in strengths can't be modified, custom ones are editable.",
				"A custom strength can't be deleted while a Conditional Access policy references it.",
				"Microsoft updates the built-in strengths when new methods become available, so built-in entries can change without an admin edit; up to 15 custom strengths, Entra ID P1 required.",
				"Users without a registered method of the strength are sent to registration, but phone sign-in, passkeys (FIDO2; registered beforehand in managed mode), certificate-based authentication and Windows Hello for Business can't be registered there; if the strength leaves a user no method they can register and use, the user is blocked from the resource.",
				"A strength doesn't restrict the initial authentication: Conditional Access is evaluated after it, so a user can still enter a password but must then satisfy the strength before continuing.",
				"The sign-in logs show which strength was enforced: the Requirement column on the Authentication Details tab names it, and the Conditional Access tab shows the policy whose grant controls required it.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/conditionalAccessPolicies",
				"Microsoft.Graph/authenticationMethodsPolicy (methods must be enabled there)",
			},
			SubtypeNote: "combinationConfigurations is polymorphic: fido2CombinationConfiguration (allowed AAGUIDs) and x509CertificateCombinationConfiguration (allowed issuer SKIs and policy OIDs) - identify the type from @odata.type.",
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/authenticationstrengthpolicy?view=graph-rest-1.0",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/authenticationstrengthroot-list-policies?view=graph-rest-1.0",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/authenticationmethodmodes?view=graph-rest-1.0",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/entra/identity/authentication/concept-authentication-strengths",
					"https://learn.microsoft.com/en-us/entra/identity/authentication/concept-authentication-strength-advanced-options",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.Policies().AuthenticationStrengthPolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list authentication strength policies: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.AuthenticationMethod' permission in Microsoft Graph)", err)
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
			item, err := client.Policies().AuthenticationStrengthPolicies().ByAuthenticationStrengthPolicyId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get authentication strength policy: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.AuthenticationMethod' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(msgraphmodels.AuthenticationStrengthPolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
