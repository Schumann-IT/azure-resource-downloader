package handlers

import (
	"context"
	"fmt"
	"strings"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// DeviceConfigurationHandler handles legacy Intune device configuration profiles
// (deviceManagement/deviceConfigurations), including Custom (OMA-URI) profiles
// such as windows10CustomConfiguration, androidCustomConfiguration,
// iosCustomConfiguration and macOSCustomConfiguration.
//
// NOTE: These are distinct from Settings Catalog policies
// (deviceManagement/configurationPolicies) which are handled by
// DeviceManagementConfigurationPolicyHandler. This handler uses the Microsoft
// Graph BETA SDK to expose the full polymorphic setting tree.
type DeviceConfigurationHandler struct {
	credential     azcore.TokenCredential
	client         *msgraphbeta.GraphServiceClient
	resolveSecrets bool
}

// omaSettingsHolder is implemented by the custom configuration types that expose
// an OMA-URI settings collection (e.g. windows10CustomConfiguration).
type omaSettingsHolder interface {
	GetOmaSettings() []betamodels.OmaSettingable
}

// NewDeviceConfigurationHandler creates a new Intune device configuration
// profile handler. When resolveSecrets is true, masked (encrypted) OMA-URI
// setting values are resolved to plaintext via getOmaSettingPlainTextValue.
//
// Because the tool now authenticates as the running (privileged) user, secret
// resolution reuses the same delegated Graph client as normal fetches — no
// separate sign-in is required. The signed-in user must hold delegated
// DeviceManagementConfiguration.ReadWrite.All and the necessary Intune RBAC.
func NewDeviceConfigurationHandler(credential azcore.TokenCredential, resolveSecrets bool) (*DeviceConfigurationHandler, error) {
	client, err := msgraphbeta.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create beta Graph client: %w", err)
	}

	return &DeviceConfigurationHandler{
		credential:     credential,
		client:         client,
		resolveSecrets: resolveSecrets,
	}, nil
}

// GetType returns the Azure resource type
func (h *DeviceConfigurationHandler) GetType() string {
	return "Microsoft.Graph/deviceConfigurations"
}

// GetTerraformResourceType returns the Terraform resource type
func (h *DeviceConfigurationHandler) GetTerraformResourceType() string {
	return "microsoft365_graph_beta_device_management_device_configuration"
}

// List returns the IDs of all legacy Intune device configuration profiles
// (including Custom OMA-URI profiles) in the tenant. This endpoint uses the
// Microsoft Graph beta API and is paged via @odata.nextLink.
func (h *DeviceConfigurationHandler) List(ctx context.Context) ([]string, error) {
	var ids []string

	builder := h.client.DeviceManagement().DeviceConfigurations()
	for {
		configs, err := builder.Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list device configurations: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
		}
		if configs == nil {
			break
		}

		for _, config := range configs.GetValue() {
			if config.GetId() != nil {
				ids = append(ids, *config.GetId())
			}
		}

		nextLink := configs.GetOdataNextLink()
		if nextLink == nil || *nextLink == "" {
			break
		}
		builder = builder.WithUrl(*nextLink)
	}

	return ids, nil
}

// Fetch retrieves a device configuration profile from Microsoft Graph beta.
//
// The polymorphic body (including omaSettings for custom profiles) is returned
// by a plain GET, so no $expand is required.
func (h *DeviceConfigurationHandler) Fetch(ctx context.Context, resourceID string) (interface{}, error) {
	configID := extractDeviceConfigurationID(resourceID)
	if configID == "" {
		return nil, fmt.Errorf("invalid resource ID format: %s", resourceID)
	}

	config, err := h.client.DeviceManagement().DeviceConfigurations().ByDeviceConfigurationId(configID).Get(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get device configuration: %w (hint: this requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
	}

	if h.resolveSecrets {
		h.resolveOmaSecrets(ctx, configID, config)
	}

	return config, nil
}

// resolveOmaSecrets resolves masked (encrypted) OMA-URI setting values to their
// plaintext form and writes them back into the model so they are serialized in the
// output. Failures for individual settings are logged and skipped (the masked
// value is retained) so a single secret never aborts the whole download.
func (h *DeviceConfigurationHandler) resolveOmaSecrets(ctx context.Context, configID string, config betamodels.DeviceConfigurationable) {
	holder, ok := config.(omaSettingsHolder)
	if !ok {
		return
	}

	for _, setting := range holder.GetOmaSettings() {
		if setting == nil {
			continue
		}
		if encrypted := setting.GetIsEncrypted(); encrypted == nil || !*encrypted {
			continue
		}
		secretRef := setting.GetSecretReferenceValueId()
		if secretRef == nil || *secretRef == "" {
			continue
		}

		resp, err := h.client.DeviceManagement().DeviceConfigurations().ByDeviceConfigurationId(configID).
			GetOmaSettingPlainTextValueWithSecretReferenceValueId(secretRef).
			GetAsGetOmaSettingPlainTextValueWithSecretReferenceValueIdGetResponse(ctx, nil)
		if err != nil {
			logger.Default.Warn("Failed to resolve encrypted OMA setting value (signed-in user needs delegated DeviceManagementConfiguration.ReadWrite.All and Intune read rights)",
				"config_id", configID,
				"oma_uri", safeStringValue(setting.GetOmaUri()),
				"reason", azure.ErrorSummary(err))
			logger.Default.Debug("OMA secret resolution failed",
				"config_id", configID,
				"oma_uri", safeStringValue(setting.GetOmaUri()),
				"error", err)
			continue
		}
		if resp == nil || resp.GetValue() == nil {
			continue
		}

		applyPlaintextToOmaSetting(setting, *resp.GetValue())
	}
}

// applyPlaintextToOmaSetting writes the resolved plaintext into the appropriate
// concrete OMA setting value field.
func applyPlaintextToOmaSetting(setting betamodels.OmaSettingable, plaintext string) {
	switch s := setting.(type) {
	case betamodels.OmaSettingStringable:
		s.SetValue(&plaintext)
	case betamodels.OmaSettingStringXmlable:
		s.SetValue([]byte(plaintext))
	}
}

// Transform converts the raw device configuration into a cleaned version.
//
// The profile body is deeply nested and polymorphic (@odata.type discriminated),
// so it is serialized to a generic map via the shared serializeParsableToMap
// helper rather than hand-coding every variant.
func (h *DeviceConfigurationHandler) Transform(resource interface{}) (*models.TransformedResource, error) {
	config, ok := resource.(betamodels.DeviceConfigurationable)
	if !ok {
		return nil, fmt.Errorf("invalid resource type, expected DeviceConfiguration")
	}

	displayName := safeStringValue(config.GetDisplayName())
	if displayName == "" {
		return nil, fmt.Errorf("device configuration display name is nil")
	}

	properties, err := serializeParsableToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize device configuration: %w", err)
	}

	configID := safeStringValue(config.GetId())

	return &models.TransformedResource{
		ID:          configID,
		Type:        h.GetType(),
		Name:        displayName,
		DisplayName: displayName,
		Properties:  properties,
	}, nil
}

// extractDeviceConfigurationID extracts the profile ID from various resource ID formats
func extractDeviceConfigurationID(resourceID string) string {
	// Handle full path format: /deviceManagement/deviceConfigurations/{id}
	if strings.Contains(resourceID, "/") {
		parts := strings.Split(resourceID, "/")
		return parts[len(parts)-1]
	}
	// Handle direct profile ID
	return resourceID
}
