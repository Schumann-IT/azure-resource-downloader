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
		azureType: "Microsoft.Graph/deviceManagementIntents",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune security baseline / template intent and its configured setting values.",
			EmbeddedPayloads:    []string{"settings (settingsDelta / setting instance values)"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           "Legacy Endpoint Security templates (intents) are being replaced by Settings Catalog based policies; plan migration. Deleting an intent removes its settings enforcement at next check-in.",
			RelatedTypes:        []string{"Microsoft.Graph/deviceManagementConfigurationPolicies (Settings Catalog successor)", "Microsoft.Graph/groups (assignment target groups)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceintent-devicemanagementintent?view=graph-rest-beta",
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
