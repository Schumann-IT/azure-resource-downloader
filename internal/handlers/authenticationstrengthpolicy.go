package handlers

import (
	"context"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// AuthenticationStrengthPolicyHandler handles Azure AD Authentication Strength Policies
type AuthenticationStrengthPolicyHandler struct {
	credential *azidentity.DefaultAzureCredential
	client     *msgraphsdk.GraphServiceClient
}

// NewAuthenticationStrengthPolicyHandler creates a new authentication strength policy handler
func NewAuthenticationStrengthPolicyHandler(credential *azidentity.DefaultAzureCredential) (*AuthenticationStrengthPolicyHandler, error) {
	// Create Graph client
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Graph client: %w", err)
	}

	return &AuthenticationStrengthPolicyHandler{
		credential: credential,
		client:     client,
	}, nil
}

// GetType returns the Azure resource type
func (h *AuthenticationStrengthPolicyHandler) GetType() string {
	return "Microsoft.Graph/authenticationStrengthPolicies"
}

// GetTerraformResourceType returns the Terraform resource type
func (h *AuthenticationStrengthPolicyHandler) GetTerraformResourceType() string {
	return "azuread_authentication_strength_policy"
}

// List returns the IDs of all authentication strength policies in the tenant.
func (h *AuthenticationStrengthPolicyHandler) List(ctx context.Context) ([]string, error) {
	policies, err := h.client.Policies().AuthenticationStrengthPolicies().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list authentication strength policies: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.AuthenticationMethod' permission in Microsoft Graph)", err)
	}

	var ids []string
	if policies != nil {
		for _, policy := range policies.GetValue() {
			if policy.GetId() != nil {
				ids = append(ids, *policy.GetId())
			}
		}
	}

	return ids, nil
}

// Fetch retrieves an authentication strength policy from Microsoft Graph
func (h *AuthenticationStrengthPolicyHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	// Extract policy ID from resource ID
	// Resource ID format: /policies/authenticationStrengthPolicies/{policyId}
	// or just the policy ID itself
	policyID := extractAuthStrengthPolicyID(resourceID)
	if policyID == "" {
		return nil, fmt.Errorf("invalid resource ID format: %s", resourceID)
	}

	policy, err := h.client.Policies().AuthenticationStrengthPolicies().ByAuthenticationStrengthPolicyId(policyID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get authentication strength policy: %w", err)
	}

	return policy, nil
}

// Transform converts the raw authentication strength policy into a cleaned version
func (h *AuthenticationStrengthPolicyHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	policy, ok := resource.(msgraphmodels.AuthenticationStrengthPolicyable)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected AuthenticationStrengthPolicy")
	}

	displayName := safeStringValue(policy.GetDisplayName())
	if displayName == "" {
		return nil, fmt.Errorf("authentication strength policy display name is nil")
	}

	properties := make(map[string]interface{})

	// Basic properties
	if policy.GetId() != nil {
		properties["id"] = *policy.GetId()
	}
	if policy.GetDisplayName() != nil {
		properties["displayName"] = *policy.GetDisplayName()
	}
	if policy.GetDescription() != nil {
		properties["description"] = *policy.GetDescription()
	}
	if policy.GetCreatedDateTime() != nil {
		properties["createdDateTime"] = policy.GetCreatedDateTime().String()
	}
	if policy.GetModifiedDateTime() != nil {
		properties["modifiedDateTime"] = policy.GetModifiedDateTime().String()
	}

	// Policy type
	if policyType := policy.GetPolicyType(); policyType != nil {
		properties["policyType"] = policyType.String()
	}

	// Requirements satisfied
	if requirementsSatisfied := policy.GetRequirementsSatisfied(); requirementsSatisfied != nil {
		properties["requirementsSatisfied"] = requirementsSatisfied.String()
	}

	// Allowed combinations (authentication method combinations)
	if allowedCombinations := policy.GetAllowedCombinations(); allowedCombinations != nil {
		combinations := make([]string, len(allowedCombinations))
		for i, combo := range allowedCombinations {
			combinations[i] = combo.String()
		}
		properties["allowedCombinations"] = combinations
	}

	// Combination configurations
	if combinationConfigurations := policy.GetCombinationConfigurations(); combinationConfigurations != nil {
		configs := make([]map[string]interface{}, 0, len(combinationConfigurations))
		for _, config := range combinationConfigurations {
			configMap := make(map[string]interface{})

			if id := config.GetId(); id != nil {
				configMap["id"] = *id
			}
			if appliesToCombinations := config.GetAppliesToCombinations(); appliesToCombinations != nil {
				combos := make([]string, len(appliesToCombinations))
				for i, combo := range appliesToCombinations {
					combos[i] = combo.String()
				}
				configMap["appliesToCombinations"] = combos
			}

			// Add OdataType to distinguish between different configuration types
			if odataType := config.GetOdataType(); odataType != nil {
				configMap["@odata.type"] = *odataType
			}

			configs = append(configs, configMap)
		}
		properties["combinationConfigurations"] = configs
	}

	policyID := safeStringValue(policy.GetId())

	return &models.TransformedResource{
		ID:          policyID,
		Type:        h.GetType(),
		Name:        displayName,
		DisplayName: displayName,
		Properties:  properties,
	}, nil
}

// extractAuthStrengthPolicyID extracts the policy ID from various resource ID formats
func extractAuthStrengthPolicyID(resourceID string) string {
	// Handle full path format: /policies/authenticationStrengthPolicies/{policyId}
	if strings.Contains(resourceID, "/") {
		parts := strings.Split(resourceID, "/")
		return parts[len(parts)-1]
	}
	// Handle direct policy ID
	return resourceID
}

// safeStringValue safely dereferences a string pointer
func safeStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
