package handlers

import (
	"context"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	msgraphsdk "github.com/microsoftgraph/msgraph-sdk-go"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// ConditionalAccessPolicyHandler handles Azure AD Conditional Access Policies
type ConditionalAccessPolicyHandler struct {
	credential azcore.TokenCredential
	client     *msgraphsdk.GraphServiceClient
}

// NewConditionalAccessPolicyHandler creates a new conditional access policy handler
func NewConditionalAccessPolicyHandler(credential azcore.TokenCredential) (*ConditionalAccessPolicyHandler, error) {
	// Create Graph client
	client, err := msgraphsdk.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Graph client: %w", err)
	}

	return &ConditionalAccessPolicyHandler{
		credential: credential,
		client:     client,
	}, nil
}

// GetType returns the Azure resource type
func (h *ConditionalAccessPolicyHandler) GetType() string {
	return "Microsoft.Graph/conditionalAccessPolicies"
}

// GetTerraformResourceType returns the Terraform resource type
func (h *ConditionalAccessPolicyHandler) GetTerraformResourceType() string {
	return "azuread_conditional_access_policy"
}

// List returns the IDs of all conditional access policies in the tenant.
func (h *ConditionalAccessPolicyHandler) List(ctx context.Context) ([]string, error) {
	policies, err := h.client.Identity().ConditionalAccess().Policies().Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list conditional access policies: %w (hint: requires 'Policy.Read.All' or 'Policy.ReadWrite.ConditionalAccess' permission in Microsoft Graph)", err)
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

// Fetch retrieves a conditional access policy from Microsoft Graph
func (h *ConditionalAccessPolicyHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	// Extract policy ID from resource ID
	// Resource ID format: /identity/conditionalAccess/policies/{policyId}
	// or just the policy ID itself
	policyID := extractPolicyID(resourceID)
	if policyID == "" {
		return nil, fmt.Errorf("invalid resource ID format: %s", resourceID)
	}

	policy, err := h.client.Identity().ConditionalAccess().Policies().ByConditionalAccessPolicyId(policyID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get conditional access policy: %w", err)
	}

	return policy, nil
}

// Transform converts the raw conditional access policy into a cleaned version
func (h *ConditionalAccessPolicyHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	policy, ok := resource.(msgraphmodels.ConditionalAccessPolicyable)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected ConditionalAccessPolicy")
	}

	displayName := safeStringPtr(policy.GetDisplayName())
	if displayName == "" {
		return nil, fmt.Errorf("conditional access policy display name is nil")
	}

	properties := make(map[string]interface{})

	// Basic properties
	if policy.GetId() != nil {
		properties["id"] = *policy.GetId()
	}
	if policy.GetDisplayName() != nil {
		properties["displayName"] = *policy.GetDisplayName()
	}
	if policy.GetCreatedDateTime() != nil {
		properties["createdDateTime"] = policy.GetCreatedDateTime().String()
	}
	if policy.GetModifiedDateTime() != nil {
		properties["modifiedDateTime"] = policy.GetModifiedDateTime().String()
	}
	if policy.GetState() != nil {
		properties["state"] = policy.GetState().String()
	}

	// Conditions
	if conditions := policy.GetConditions(); conditions != nil {
		conditionsMap := make(map[string]interface{})

		// User and Group conditions
		if users := conditions.GetUsers(); users != nil {
			usersMap := make(map[string]interface{})
			if includeUsers := users.GetIncludeUsers(); includeUsers != nil {
				usersMap["includeUsers"] = includeUsers
			}
			if excludeUsers := users.GetExcludeUsers(); excludeUsers != nil {
				usersMap["excludeUsers"] = excludeUsers
			}
			if includeGroups := users.GetIncludeGroups(); includeGroups != nil {
				usersMap["includeGroups"] = includeGroups
			}
			if excludeGroups := users.GetExcludeGroups(); excludeGroups != nil {
				usersMap["excludeGroups"] = excludeGroups
			}
			if includeRoles := users.GetIncludeRoles(); includeRoles != nil {
				usersMap["includeRoles"] = includeRoles
			}
			if excludeRoles := users.GetExcludeRoles(); excludeRoles != nil {
				usersMap["excludeRoles"] = excludeRoles
			}
			conditionsMap["users"] = usersMap
		}

		// Application conditions
		if applications := conditions.GetApplications(); applications != nil {
			appsMap := make(map[string]interface{})
			if includeApps := applications.GetIncludeApplications(); includeApps != nil {
				appsMap["includeApplications"] = includeApps
			}
			if excludeApps := applications.GetExcludeApplications(); excludeApps != nil {
				appsMap["excludeApplications"] = excludeApps
			}
			if includeUserActions := applications.GetIncludeUserActions(); includeUserActions != nil {
				appsMap["includeUserActions"] = includeUserActions
			}
			conditionsMap["applications"] = appsMap
		}

		// Platforms
		if platforms := conditions.GetPlatforms(); platforms != nil {
			platformsMap := make(map[string]interface{})
			if includePlatforms := platforms.GetIncludePlatforms(); includePlatforms != nil {
				platformsMap["includePlatforms"] = includePlatforms
			}
			if excludePlatforms := platforms.GetExcludePlatforms(); excludePlatforms != nil {
				platformsMap["excludePlatforms"] = excludePlatforms
			}
			conditionsMap["platforms"] = platformsMap
		}

		// Locations
		if locations := conditions.GetLocations(); locations != nil {
			locationsMap := make(map[string]interface{})
			if includeLocations := locations.GetIncludeLocations(); includeLocations != nil {
				locationsMap["includeLocations"] = includeLocations
			}
			if excludeLocations := locations.GetExcludeLocations(); excludeLocations != nil {
				locationsMap["excludeLocations"] = excludeLocations
			}
			conditionsMap["locations"] = locationsMap
		}

		// Client App Types
		if clientAppTypes := conditions.GetClientAppTypes(); clientAppTypes != nil {
			types := make([]string, len(clientAppTypes))
			for i, cat := range clientAppTypes {
				types[i] = cat.String()
			}
			conditionsMap["clientAppTypes"] = types
		}

		// Sign-in risk levels
		if riskLevels := conditions.GetSignInRiskLevels(); riskLevels != nil {
			levels := make([]string, len(riskLevels))
			for i, rl := range riskLevels {
				levels[i] = rl.String()
			}
			conditionsMap["signInRiskLevels"] = levels
		}

		// User risk levels
		if userRiskLevels := conditions.GetUserRiskLevels(); userRiskLevels != nil {
			levels := make([]string, len(userRiskLevels))
			for i, url := range userRiskLevels {
				levels[i] = url.String()
			}
			conditionsMap["userRiskLevels"] = levels
		}

		properties["conditions"] = conditionsMap
	}

	// Grant Controls
	if grantControls := policy.GetGrantControls(); grantControls != nil {
		grantMap := make(map[string]interface{})
		if operator := grantControls.GetOperator(); operator != nil {
			grantMap["operator"] = *operator
		}
		if builtInControls := grantControls.GetBuiltInControls(); builtInControls != nil {
			controls := make([]string, len(builtInControls))
			for i, ctrl := range builtInControls {
				controls[i] = ctrl.String()
			}
			grantMap["builtInControls"] = controls
		}
		if customAuthFactors := grantControls.GetCustomAuthenticationFactors(); customAuthFactors != nil {
			grantMap["customAuthenticationFactors"] = customAuthFactors
		}
		if termsOfUse := grantControls.GetTermsOfUse(); termsOfUse != nil {
			grantMap["termsOfUse"] = termsOfUse
		}
		// Authentication Strength
		if authStrength := grantControls.GetAuthenticationStrength(); authStrength != nil {
			authStrengthMap := make(map[string]interface{})
			if id := authStrength.GetId(); id != nil {
				authStrengthMap["id"] = *id
			}
			if displayName := authStrength.GetDisplayName(); displayName != nil {
				authStrengthMap["displayName"] = *displayName
			}
			if description := authStrength.GetDescription(); description != nil {
				authStrengthMap["description"] = *description
			}
			if policyType := authStrength.GetPolicyType(); policyType != nil {
				authStrengthMap["policyType"] = policyType.String()
			}
			if requirementsSatisfied := authStrength.GetRequirementsSatisfied(); requirementsSatisfied != nil {
				authStrengthMap["requirementsSatisfied"] = requirementsSatisfied.String()
			}
			grantMap["authenticationStrength"] = authStrengthMap
		}
		properties["grantControls"] = grantMap
	}

	// Session Controls
	if sessionControls := policy.GetSessionControls(); sessionControls != nil {
		sessionMap := make(map[string]interface{})

		if appEnforcedRestrictions := sessionControls.GetApplicationEnforcedRestrictions(); appEnforcedRestrictions != nil {
			if isEnabled := appEnforcedRestrictions.GetIsEnabled(); isEnabled != nil {
				sessionMap["applicationEnforcedRestrictions"] = map[string]interface{}{
					"isEnabled": *isEnabled,
				}
			}
		}

		if cloudAppSecurity := sessionControls.GetCloudAppSecurity(); cloudAppSecurity != nil {
			casMap := make(map[string]interface{})
			if isEnabled := cloudAppSecurity.GetIsEnabled(); isEnabled != nil {
				casMap["isEnabled"] = *isEnabled
			}
			if cloudAppSecurityType := cloudAppSecurity.GetCloudAppSecurityType(); cloudAppSecurityType != nil {
				casMap["cloudAppSecurityType"] = cloudAppSecurityType.String()
			}
			sessionMap["cloudAppSecurity"] = casMap
		}

		if signInFrequency := sessionControls.GetSignInFrequency(); signInFrequency != nil {
			sifMap := make(map[string]interface{})
			if value := signInFrequency.GetValue(); value != nil {
				sifMap["value"] = *value
			}
			if frequencyType := signInFrequency.GetTypeEscaped(); frequencyType != nil {
				sifMap["type"] = frequencyType.String()
			}
			if isEnabled := signInFrequency.GetIsEnabled(); isEnabled != nil {
				sifMap["isEnabled"] = *isEnabled
			}
			sessionMap["signInFrequency"] = sifMap
		}

		if persistentBrowser := sessionControls.GetPersistentBrowser(); persistentBrowser != nil {
			pbMap := make(map[string]interface{})
			if mode := persistentBrowser.GetMode(); mode != nil {
				pbMap["mode"] = mode.String()
			}
			if isEnabled := persistentBrowser.GetIsEnabled(); isEnabled != nil {
				pbMap["isEnabled"] = *isEnabled
			}
			sessionMap["persistentBrowser"] = pbMap
		}

		properties["sessionControls"] = sessionMap
	}

	policyID := safeStringPtr(policy.GetId())

	return &models.TransformedResource{
		ID:          policyID,
		Type:        h.GetType(),
		Name:        displayName,
		DisplayName: displayName,
		Properties:  properties,
	}, nil
}

// extractPolicyID extracts the policy ID from various resource ID formats
func extractPolicyID(resourceID string) string {
	// Handle full path format: /identity/conditionalAccess/policies/{policyId}
	if strings.Contains(resourceID, "/") {
		parts := strings.Split(resourceID, "/")
		return parts[len(parts)-1]
	}
	// Handle direct policy ID
	return resourceID
}

// safeStringPtr safely dereferences a string pointer
func safeStringPtr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
