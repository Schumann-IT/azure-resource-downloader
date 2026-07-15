package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/logger"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// omaSettingsHolder is implemented by the custom configuration types that expose
// an OMA-URI settings collection (e.g. windows10CustomConfiguration).
type omaSettingsHolder interface {
	GetOmaSettings() []betamodels.OmaSettingable
}

// NewDeviceConfigurationHandler creates a handler for legacy Intune device
// configuration profiles (deviceManagement/deviceConfigurations, Microsoft
// Graph beta), including Custom (OMA-URI) profiles such as
// windows10CustomConfiguration, androidCustomConfiguration,
// iosCustomConfiguration and macOSCustomConfiguration.
//
// These are distinct from Settings Catalog policies
// (deviceManagement/configurationPolicies) which are handled by
// NewDeviceManagementConfigurationPolicyHandler.
//
// When resolveSecrets is true, masked (encrypted) OMA-URI setting values are
// resolved to plaintext via getOmaSettingPlainTextValue using the same
// delegated Graph client — the signed-in user must hold delegated
// DeviceManagementConfiguration.ReadWrite.All and the necessary Intune RBAC.
func NewDeviceConfigurationHandler(credential azcore.TokenCredential, resolveSecrets bool) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceConfigurations",
		documentation: models.ResourceDocumentation{
			Purpose:             "A legacy Intune device configuration profile (templates), including Custom OMA-URI profiles for Windows, iOS, Android and macOS.",
			KeySettings:         []string{"omaSettings", "encrypted/secret values"},
			EmbeddedPayloads:    []string{"omaSettings (custom OMA-URI values, decode encrypted/secret values)", "payload (base64 .mobileconfig for Apple custom profiles)", "configurationXml"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           []string{"Legacy template profiles are progressively superseded by the Settings Catalog (deviceManagementConfigurationPolicies); prefer Settings Catalog for new configurations.", "Unassigning or deleting removes settings at next check-in, though some CSP-backed settings persist by design."},
			RelatedTypes:        []string{"Microsoft.Graph/deviceManagementConfigurationPolicies (Settings Catalog successor)", "Microsoft.Graph/groups (assignment target groups)", "Microsoft.Graph/assignmentFilters (assignment filters)"},
			SubtypeNote:         "Legacy profiles are heavily polymorphic (windows10CustomConfiguration, macOSCustomConfiguration, windows10EndpointProtectionConfiguration, ...) - identify the concrete profile type from @odata.type first.",
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-deviceconfiguration?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/mem/intune/protect/security-baselines"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceConfigurations()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device configurations: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceConfigurations().ByDeviceConfigurationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get device configuration: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if resolveSecrets {
				resolveOmaSecrets(ctx, client, itemID, item)
			}
			if assignments, err := client.DeviceManagement().DeviceConfigurations().ByDeviceConfigurationId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceConfigurations", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.DeviceConfigurationable); ok {
				return safeStringValue(c.GetDisplayName())
			}
			return ""
		},
	}, nil
}

// resolveOmaSecrets resolves masked (encrypted) OMA-URI setting values to their
// plaintext form and writes them back into the model so they are serialized in the
// output. Failures for individual settings are logged and skipped (the masked
// value is retained) so a single secret never aborts the whole download.
func resolveOmaSecrets(ctx context.Context, client *msgraphbeta.GraphServiceClient, configID string, config betamodels.DeviceConfigurationable) {
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

		resp, err := client.DeviceManagement().DeviceConfigurations().ByDeviceConfigurationId(configID).
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
