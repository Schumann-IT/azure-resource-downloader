package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewDeviceManagementIntentHandler creates a handler for legacy Intune
// Endpoint Security intents (deviceManagement/intents, Microsoft Graph beta).
// These are template-based policies that predate the Settings Catalog; the
// originating template is referenced by the policy's templateId.
//
// The configured settings are not part of the intent object: they live in the
// child collection settings, so Fetch retrieves them separately and attaches
// them to the model before serialization.
func NewDeviceManagementIntentHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/deviceManagementIntents",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune security baseline / template intent and its configured setting values.",
			EmbeddedPayloads: []string{
				"settings[].valueJson (JSON-encoded value of each setting, keyed by settings[].definitionId)",
			},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"Old-format endpoint security and security baseline profiles: new ones can no longer be created, and baselines released before May 2023 can't be upgraded to the new format; recreate them as Settings Catalog based policies (deviceManagementConfigurationPolicies).",
				"Deleting or unassigning stops enforcement, but settings that are no longer managed may stay on the device (CSP-dependent).",
				"Its settings conflict with any baseline, endpoint security policy or profile that sets the same setting differently on the same device (status Conflict, resolved manually); a new-format replacement doesn't inherit this profile's assignments, and this profile keeps applying until it is unassigned or deleted.",
				"Old-format baseline profiles report a posture per device (Matches default baseline, Matches custom settings, Misconfigured for Error, Pending or Conflict, Not applicable); data appears up to 24 hours after the first assignment and up to six hours after later changes.",
				"The least-privileged built-in role that can manage security baseline profiles is the Intune Policy and Profile Manager.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/deviceManagementConfigurationPolicies (new-format successor)",
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			KeySettings: []string{"templateId", "isMigratingToConfigurationPolicy", "settings[].definitionId"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceintent-devicemanagementintent?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-deviceintent-devicemanagementintent-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-security/security-baselines/overview",
					"https://learn.microsoft.com/en-us/intune/device-security/security-baselines/configure-baselines",
					"https://learn.microsoft.com/en-us/intune/device-security/security-baselines/ref-windows-mdm-settings",
					"https://learn.microsoft.com/en-us/intune/device-security/security-baselines/ref-defender-settings",
					"https://learn.microsoft.com/en-us/intune/device-security/security-baselines/ref-edge-settings",
					"https://learn.microsoft.com/en-us/intune/device-security/security-baselines/ref-windows-365-settings",
				},
				AdminCenter: "https://intune.microsoft.com/#view/Microsoft_Intune_Workflows/SecurityManagementMenu/~/overview",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().Intents()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device management intents: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().Intents().ByDeviceManagementIntentId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get device management intent: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}

			settings, err := listIntentSettings(ctx, client, itemID)
			if err != nil {
				return nil, err
			}
			item.SetSettings(settings)

			if assignments, err := client.DeviceManagement().Intents().ByDeviceManagementIntentId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceManagementIntents", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}

			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.DeviceManagementIntentable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}

// listIntentSettings pages through the settings child collection of a device
// management intent.
func listIntentSettings(ctx context.Context, client *msgraphbeta.GraphServiceClient, intentID string) ([]betamodels.DeviceManagementSettingInstanceable, error) {
	var settings []betamodels.DeviceManagementSettingInstanceable

	builder := client.DeviceManagement().Intents().ByDeviceManagementIntentId(intentID).Settings()
	for {
		resp, err := builder.Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list device management intent settings: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
		}
		if resp == nil {
			break
		}
		settings = append(settings, resp.GetValue()...)

		next := resp.GetOdataNextLink()
		if next == nil || *next == "" {
			break
		}
		builder = builder.WithUrl(*next)
	}

	return settings, nil
}
