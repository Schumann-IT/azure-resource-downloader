package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewNdesConnectorHandler creates a handler for Intune certificate (NDES/SCEP)
// connectors (deviceManagement/ndesConnectors, Microsoft Graph beta). These
// expose the connection state/metadata of the on-premises NDES connector used
// for SCEP certificate issuance; relevant to certificate-based Windows config.
func NewNdesConnectorHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/ndesConnectors",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune NDES (SCEP) connector used to issue certificates via a Network Device Enrollment Service.",
			KeySettings:         []string{"state", "lastConnectionDateTime"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           []string{"Reflects the state of the on-premises Certificate Connector; keep the connector software current and renew its certificates before expiry to avoid SCEP issuance outages."},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-ndesconnector?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().NdesConnectors()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list NDES connectors: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().NdesConnectors().ByNdesConnectorId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get NDES connector: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			c, ok := item.(betamodels.NdesConnectorable)
			if !ok {
				return ""
			}
			if name := safeStringValue(c.GetDisplayName()); name != "" {
				return name
			}
			return safeStringValue(c.GetId())
		},
	}, nil
}
