package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewMobileThreatDefenseConnectorHandler creates a handler for Mobile Threat
// Defense connectors (deviceManagement/mobileThreatDefenseConnectors, Microsoft
// Graph beta), which wire Intune to MTD partners (e.g. Microsoft Defender for
// Endpoint) across Windows, macOS, iOS and Android.
func NewMobileThreatDefenseConnectorHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/mobileThreatDefenseConnectors",
		documentation: models.ResourceDocumentation{
			Template:            recordPromptTemplateText,
			Purpose:             "An Intune Mobile Threat Defense connector integrating a third-party MTD partner.",
			KeySettings:         []string{"androidEnabled", "iosEnabled", "windowsEnabled", "partnerState"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           []string{"Connector health depends on the MTD partner subscription; deactivating it or letting the partner contract lapse changes compliance evaluation for devices reporting threat levels."},
			RelatedTypes:        []string{"Microsoft.Graph/deviceCompliancePolicies (threat-level based compliance)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-mobilethreatdefenseconnector?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().MobileThreatDefenseConnectors()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list mobile threat defense connectors: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().MobileThreatDefenseConnectors().ByMobileThreatDefenseConnectorId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get mobile threat defense connector: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.MobileThreatDefenseConnectorable); ok {
				return safeStringValue(c.GetId())
			}
			return ""
		},
	}, nil
}
