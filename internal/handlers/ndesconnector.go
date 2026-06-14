package handlers

import (
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
//
// The deploymenttheory/microsoft365 provider has no NDES connector resource, so
// no Terraform import is emitted. Connectors are named by their friendly name,
// falling back to the item ID.
func NewNdesConnectorHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/ndesConnectors",
		terraformType: "",
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
