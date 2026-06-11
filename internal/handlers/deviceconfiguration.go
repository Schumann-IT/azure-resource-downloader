package handlers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// secretResolutionScopes is the delegated Graph scope required to read encrypted
// OMA-URI values via getOmaSettingPlainTextValue. Requesting the explicit scope
// (rather than ".default") triggers incremental consent on first sign-in.
var secretResolutionScopes = []string{"https://graph.microsoft.com/DeviceManagementConfiguration.ReadWrite.All"}

// reusableCredential wraps an interactive credential so a single token acquired
// up front is reused for every request. This prevents repeated device-code
// prompts that would otherwise be triggered when the Intune backend returns a
// per-call Conditional Access claims challenge: the request options (including
// any claims) are ignored and the cached token is returned until near expiry.
type reusableCredential struct {
	inner  azcore.TokenCredential
	scopes []string

	mu    sync.Mutex
	token azcore.AccessToken
}

// GetToken returns the cached token if it is still valid, otherwise it acquires
// a new one from the wrapped credential using the fixed scopes.
func (c *reusableCredential) GetToken(ctx context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token.Token != "" && time.Until(c.token.ExpiresOn) > 5*time.Minute {
		return c.token, nil
	}

	token, err := c.inner.GetToken(ctx, policy.TokenRequestOptions{Scopes: c.scopes})
	if err != nil {
		return azcore.AccessToken{}, err
	}
	c.token = token
	return token, nil
}

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
	credential     *azidentity.DefaultAzureCredential
	client         *msgraphbeta.GraphServiceClient
	secretsClient  *msgraphbeta.GraphServiceClient
	resolveSecrets bool
}

// SecretResolutionOptions configures delegated resolution of encrypted OMA-URI
// secret values. Resolution requires a delegated (device-code) sign-in because
// the Intune backend rejects app-only tokens for getOmaSettingPlainTextValue.
type SecretResolutionOptions struct {
	Enabled  bool   // Whether to resolve encrypted OMA-URI values
	ClientID string // Public client (app registration) ID with delegated DeviceManagementConfiguration.ReadWrite.All
	TenantID string // Tenant for sign-in (defaults to AZURE_TENANT_ID, then "organizations")
}

// omaSettingsHolder is implemented by the custom configuration types that expose
// an OMA-URI settings collection (e.g. windows10CustomConfiguration).
type omaSettingsHolder interface {
	GetOmaSettings() []betamodels.OmaSettingable
}

// NewDeviceConfigurationHandler creates a new Intune device configuration
// profile handler. When secretOpts.Enabled is true, masked (encrypted) OMA-URI
// setting values are resolved to plaintext via getOmaSettingPlainTextValue.
//
// Resolving those secrets requires a delegated token because the Intune backend
// rejects app-only tokens for getOmaSettingPlainTextValue. Resolution therefore
// uses an interactive device-code sign-in (against the provided public client ID)
// while normal fetches keep using the provided (service principal) credential.
func NewDeviceConfigurationHandler(credential *azidentity.DefaultAzureCredential, secretOpts SecretResolutionOptions) (*DeviceConfigurationHandler, error) {
	client, err := msgraphbeta.NewGraphServiceClientWithCredentials(credential, []string{
		"https://graph.microsoft.com/.default",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create beta Graph client: %w", err)
	}

	h := &DeviceConfigurationHandler{
		credential:     credential,
		client:         client,
		resolveSecrets: secretOpts.Enabled,
	}

	if secretOpts.Enabled {
		if err := h.initSecretsClient(secretOpts); err != nil {
			logger.Default.Warn("Secret resolution disabled", "error", err)
			h.resolveSecrets = false
		}
	}

	return h, nil
}

// initSecretsClient performs an interactive device-code sign-in and builds the
// delegated Graph client used for secret resolution. Signing in eagerly here (a)
// surfaces auth/consent errors before the pipeline starts and (b) avoids
// concurrent device-code prompts from parallel pipeline workers.
func (h *DeviceConfigurationHandler) initSecretsClient(secretOpts SecretResolutionOptions) error {
	if secretOpts.ClientID == "" {
		return fmt.Errorf("--secrets-client-id is required (public client app with delegated DeviceManagementConfiguration.ReadWrite.All)")
	}

	tenantID := secretOpts.TenantID
	if tenantID == "" {
		tenantID = os.Getenv("AZURE_TENANT_ID")
	}

	deviceCodeCred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
		ClientID: secretOpts.ClientID,
		TenantID: tenantID,
		UserPrompt: func(_ context.Context, msg azidentity.DeviceCodeMessage) error {
			fmt.Fprintln(os.Stderr, msg.Message)
			return nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create device-code credential: %w", err)
	}

	// Wrap the interactive credential so the token from the single up-front
	// sign-in is reused for every getOmaSettingPlainTextValue call.
	secretsCred := &reusableCredential{inner: deviceCodeCred, scopes: secretResolutionScopes}

	// Force the interactive sign-in now so consent happens up front (once),
	// before the pipeline starts and before any concurrent workers run.
	if _, err := secretsCred.GetToken(context.Background(), policy.TokenRequestOptions{Scopes: secretResolutionScopes}); err != nil {
		return fmt.Errorf("device-code sign-in failed: %w", err)
	}

	secretsClient, err := msgraphbeta.NewGraphServiceClientWithCredentials(secretsCred, secretResolutionScopes)
	if err != nil {
		return fmt.Errorf("failed to create delegated Graph client: %w", err)
	}
	h.secretsClient = secretsClient
	return nil
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

		resp, err := h.secretsClient.DeviceManagement().DeviceConfigurations().ByDeviceConfigurationId(configID).
			GetOmaSettingPlainTextValueWithSecretReferenceValueId(secretRef).
			GetAsGetOmaSettingPlainTextValueWithSecretReferenceValueIdGetResponse(ctx, nil)
		if err != nil {
			logger.Default.Warn("Failed to resolve encrypted OMA setting value (signed-in user needs delegated DeviceManagementConfiguration.ReadWrite.All and Intune read rights)",
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
