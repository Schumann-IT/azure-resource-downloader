package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
)

// deviceManagementSettingsName names the Intune tenant settings singleton
// output file.
const deviceManagementSettingsName = "Intune Tenant Settings"

// NewDeviceManagementSettingsHandler creates a handler for the Intune tenant
// settings (the deviceManagement root object, Microsoft Graph beta). This is a
// tenant **singleton**: List probes the object and returns at most one
// pseudo-ID, and Fetch retrieves the singleton regardless of the requested ID.
func NewDeviceManagementSettingsHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	getSingleton := func(ctx context.Context) (serialization.Parsable, string, error) {
		settings, err := client.DeviceManagement().Get(ctx, nil)
		if err != nil {
			return nil, "", fmt.Errorf("failed to get Intune tenant settings: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
		}
		id := ""
		if settings != nil && settings.GetId() != nil {
			id = *settings.GetId()
		}
		return settings, id, nil
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceManagement",
		documentation: models.ResourceDocumentation{
			Template:            singletonPromptTemplateText,
			Purpose:             "Tenant-wide Intune device management settings and configuration.",
			KeySettings:         []string{"settings", "intuneAccountId"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           []string{"Tenant-wide Intune settings singleton; changes apply to the whole tenant immediately."},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-devicemanagementsettings?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			_, id, err := getSingleton(ctx)
			if err != nil {
				return nil, err
			}
			if id == "" {
				id = "deviceManagement"
			}
			return []string{id}, nil
		},
		fetchItem: func(ctx context.Context, _ string) (serialization.Parsable, error) {
			settings, _, err := getSingleton(ctx)
			return settings, err
		},
		displayName: func(_ serialization.Parsable) string {
			return deviceManagementSettingsName
		},
	}, nil
}
