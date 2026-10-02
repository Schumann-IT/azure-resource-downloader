package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewDeviceManagementConfigurationPolicyHandler creates a handler for Intune
// Settings Catalog configuration policies
// (deviceManagement/configurationPolicies, Microsoft Graph beta).
//
// Fetch uses $expand=settings so the full polymorphic setting tree is returned
// (settings are not included in a plain GET). Settings Catalog policies use
// `name` instead of `displayName`.
func NewDeviceManagementConfigurationPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/deviceManagementConfigurationPolicies",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune configuration policy of the unified settings platform: Settings Catalog policies plus template-based policies such as endpoint security policies and security baselines, told apart by templateReference.templateFamily (none = plain Settings Catalog).",
			EmbeddedPayloads: []string{
				"settings[].settingInstance (nested setting instances keyed by settingDefinitionId; secret values carry valueState notEncrypted or encryptedValueToken)",
			},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"Deprecated templates move here: the macOS Endpoint protection and Extensions templates (2408) and the Windows Administrative Templates (2412) are configured in the Settings Catalog.",
				"Setting a value back to Not configured leaves the device value in place but stops enforcing it; when a new security baseline version ships, profiles on an older version become read-only until updated.",
				"Configuration policies of every type have equal precedence: if another configuration policy (settings catalog, endpoint security, baseline or template) sets the same setting differently, it shows as Conflict and may not apply until an admin resolves it; Defender Antivirus exclusions merge instead.",
				"Windows settings are user- or device-scoped: a device-scoped setting assigned to a user group applies to all users of each device that user signs in to, a user-scoped setting assigned to a device group applies to every user of that device, and a setting assigned in both scopes takes the user-scope value.",
				"Each policy, endpoint security policies and security baselines included, reports device and user check-in status (Succeeded, Error, Conflict, Pending, Not applicable) and per-setting status; Devices > Monitor > Assignment failures lists policies that failed with an error or conflict.",
				"Settings Catalog policies and security baselines can be created and changed with at least the Intune Policy and Profile Manager role.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/reusablePolicySettings (referenced by ID)",
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (assignment filters; not supported for security baselines)",
				"Microsoft.Graph/deviceManagementIntents (old-format endpoint security and baseline predecessor)",
				"Microsoft.Graph/groupPolicyConfigurations (ADMX settings moved to the Settings Catalog)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			KeySettings: []string{
				"platforms",
				"technologies",
				"templateReference (templateFamily, templateDisplayName, templateDisplayVersion)",
				"settingCount",
			},
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfigv2-devicemanagementconfigurationpolicy?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-deviceconfigv2-devicemanagementconfigurationpolicy-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfigv2-devicemanagementconfigurationsettingdefinition?view=graph-rest-beta",
				AdminCenter:     "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/DevicesMenu/~/configuration",
			},
			SubtypeNote: "Endpoint Privilege Management policies (elevation settings and elevation rules policies) need EPM licensed in the tenant, and elevation rules take effect only on devices that also receive an elevation settings policy that enables EPM. Policies whose technologies include microsoftSense (the Defender for Endpoint SENSE agent channel) also apply to devices that Defender for Endpoint security settings management manages without Intune enrollment; those devices accept device-group assignments only and ignore assignment filters.",
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().ConfigurationPolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list configuration policies: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadevicemanagement.ConfigurationPoliciesDeviceManagementConfigurationPolicyItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.ConfigurationPoliciesDeviceManagementConfigurationPolicyItemRequestBuilderGetQueryParameters{
					Expand: []string{"settings"},
				},
			}
			item, err := client.DeviceManagement().ConfigurationPolicies().ByDeviceManagementConfigurationPolicyId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get configuration policy: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().ConfigurationPolicies().ByDeviceManagementConfigurationPolicyId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceManagementConfigurationPolicies", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.DeviceManagementConfigurationPolicyable); ok {
				return safeStringValue(p.GetName())
			}
			return ""
		},
	}, nil
}
