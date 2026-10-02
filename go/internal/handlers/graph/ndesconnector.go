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
			Template:            recordPromptTemplateText,
			Purpose:             "An on-premises Certificate Connector for Microsoft Intune registration (Graph: 'OnPrem Ndes connector'), used for SCEP certificate issuance through NDES.",
			KeySettings:         []string{"state", "lastConnectionDateTime", "connectorVersion", "machineName", "enrolledDateTime"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"The Certificate Connector for Microsoft Intune updates itself automatically (autoupdate.msappproxy.net, port 443) unless that is blocked; each release is supported for six months, and out-of-support versions may stop working.",
				"Since 29 July 2021 it replaces the PFX Certificate Connector and the Microsoft Intune Connector.",
				"In Tenant administration > Connectors and tokens > Certificate connectors, a deprecated connector version shows a Warning, then an Error after the six-month grace period (neither is a state value); logs: Event Viewer > Application and Service Logs > Microsoft > Intune > Certificate Connectors.",
				"Up to 100 connector instances per tenant, each on its own Windows Server; all should run the same version, and a request goes to any instance with the needed feature. For SCEP it isn't supported on the issuing CA server, and it shouldn't share a server with the Intune Connector for Active Directory.",
				"Strong mapping in the connector needs Windows Server 2019 or later; SCEP with a Microsoft CA needs NDES on the connector's server, and the connector is configured with an Intune Administrator account that has an Intune license.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/deviceConfigurations (SCEP and PKCS certificate profiles)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-ndesconnector?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-deviceconfig-ndesconnector-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/fundamentals/certificates/connector/overview",
					"https://learn.microsoft.com/en-us/intune/fundamentals/certificates/scep-infrastructure",
				},
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
