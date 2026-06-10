package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	kjson "github.com/microsoft/kiota-serialization-json-go"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// DeviceManagementConfigurationPolicyHandler handles Intune Settings Catalog
// configuration policies (deviceManagement/configurationPolicies).
//
// NOTE: This endpoint only exists in the Microsoft Graph BETA API, so this
// handler uses the beta SDK (github.com/microsoftgraph/msgraph-beta-sdk-go)
// rather than the stable v1.0 SDK used by the other handlers.
type DeviceManagementConfigurationPolicyHandler struct {
	credential *azidentity.DefaultAzureCredential
	client     *msgraphbeta.GraphServiceClient
}

// NewDeviceManagementConfigurationPolicyHandler creates a new Intune Settings
// Catalog configuration policy handler.
func NewDeviceManagementConfigurationPolicyHandler(credential *azidentity.DefaultAzureCredential) (*DeviceManagementConfigurationPolicyHandler, error) {
	// Create beta Graph client
	client, err := msgraphbeta.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create beta Graph client: %w", err)
	}

	return &DeviceManagementConfigurationPolicyHandler{
		credential: credential,
		client:     client,
	}, nil
}

// GetType returns the Azure resource type
func (h *DeviceManagementConfigurationPolicyHandler) GetType() string {
	return "Microsoft.Graph/deviceManagementConfigurationPolicies"
}

// GetTerraformResourceType returns the Terraform resource type
func (h *DeviceManagementConfigurationPolicyHandler) GetTerraformResourceType() string {
	return "microsoft365_graph_beta_device_management_settings_catalog_configuration_policy"
}

// Fetch retrieves a configuration policy (including its full settings tree) from
// Microsoft Graph beta.
func (h *DeviceManagementConfigurationPolicyHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	// Resource ID format: /deviceManagement/configurationPolicies/{id}
	// or just the policy ID itself
	policyID := extractConfigurationPolicyID(resourceID)
	if policyID == "" {
		return nil, fmt.Errorf("invalid resource ID format: %s", resourceID)
	}

	// $expand=settings so the full setting tree is returned (settings are not
	// included in a plain GET).
	requestConfig := &betadevicemanagement.ConfigurationPoliciesDeviceManagementConfigurationPolicyItemRequestBuilderGetRequestConfiguration{
		QueryParameters: &betadevicemanagement.ConfigurationPoliciesDeviceManagementConfigurationPolicyItemRequestBuilderGetQueryParameters{
			Expand: []string{"settings"},
		},
	}

	policy, err := h.client.DeviceManagement().ConfigurationPolicies().ByDeviceManagementConfigurationPolicyId(policyID).Get(ctx, requestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration policy: %w (hint: this requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
	}

	return policy, nil
}

// Transform converts the raw configuration policy into a cleaned version.
//
// The settings catalog setting tree is deeply nested and polymorphic, so rather
// than hand-coding every @odata.type variant we serialize the whole object to
// JSON via the Kiota serializer and unmarshal it into a generic map. The
// cleaning transformer can then tidy it further based on config.
func (h *DeviceManagementConfigurationPolicyHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	policy, ok := resource.(betamodels.DeviceManagementConfigurationPolicyable)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected DeviceManagementConfigurationPolicy")
	}

	displayName := safeStringValue(policy.GetName())
	if displayName == "" {
		return nil, fmt.Errorf("configuration policy name is nil")
	}

	properties, err := serializeParsableToMap(policy)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize configuration policy: %w", err)
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

// serializeParsableToMap serializes a Kiota Parsable object to a generic map by
// round-tripping through its JSON representation. This captures the full nested
// tree (including polymorphic @odata.type discriminated children) without having
// to manually handle every model type.
func serializeParsableToMap(policy betamodels.DeviceManagementConfigurationPolicyable) (map[string]interface{}, error) {
	writer := kjson.NewJsonSerializationWriter()
	defer writer.Close()

	if err := writer.WriteObjectValue("", policy); err != nil {
		return nil, fmt.Errorf("failed to write object value: %w", err)
	}

	content, err := writer.GetSerializedContent()
	if err != nil {
		return nil, fmt.Errorf("failed to get serialized content: %w", err)
	}

	properties := make(map[string]interface{})
	if err := json.Unmarshal(content, &properties); err != nil {
		return nil, fmt.Errorf("failed to unmarshal serialized content: %w", err)
	}

	return properties, nil
}

// extractConfigurationPolicyID extracts the policy ID from various resource ID formats
func extractConfigurationPolicyID(resourceID string) string {
	// Handle full path format: /deviceManagement/configurationPolicies/{id}
	if strings.Contains(resourceID, "/") {
		parts := strings.Split(resourceID, "/")
		return parts[len(parts)-1]
	}
	// Handle direct policy ID
	return resourceID
}
