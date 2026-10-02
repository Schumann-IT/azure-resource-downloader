package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsAutopilotDeviceIdentityHandler creates a handler for Windows
// Autopilot device identities
// (deviceManagement/windowsAutopilotDeviceIdentities, Microsoft Graph beta).
//
// NOTE: this is registered device data rather than configuration and can be a
// large collection in big tenants. Identities often have no display name, so
// the serial number (or the object ID) is used as the file name fallback.
func NewWindowsAutopilotDeviceIdentityHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/windowsAutopilotDeviceIdentities",
		documentation: models.ResourceDocumentation{
			Template:            recordPromptTemplateText,
			OmitGroupAxes:       true,
			Purpose:             "A Windows Autopilot device identity (hardware hash registration) for zero-touch provisioning.",
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
				"Deregister a device when it permanently leaves the organization (for example repair or end of life): delete its Intune device record first, because only devices not enrolled in Intune can be deleted from Autopilot.",
				"Don't delete the Entra device object manually; for hybrid-joined devices delete the on-premises AD computer object instead.",
				"A registered device without an assigned profile still receives the default Windows Autopilot profile; remove the registration if the device shouldn't go through Autopilot.",
				"A hardware hash registered in one tenant can't be imported into another (ZtdDeviceAssignedToAnotherTenant); after a motherboard replacement the registered hash no longer matches and a new hash must be uploaded.",
				"In Windows Autopilot devices the profile status moves from Unassigned through Assigning to Assigned; deploy the device only once it reads Assigned and Date assigned (deploymentProfileAssignedDateTime) is set.",
				"An assigned user only pre-fills the sign-in UPN and greeting on supported OEMs (not with AD FS) and doesn't change which policies or apps apply; a device name set here is ignored for hybrid join, where the Domain Join profile names the device.",
				"deviceAccountPassword (Surface Hub) is a secret: redact it if present.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/windowsAutopilotDeploymentProfiles",
				"Microsoft.Graph/groups (dynamic device groups on ZTDId, OrderID (the group tag) or PurchaseOrderId)",
			},
			KeySettings: []string{
				"groupTag",
				"purchaseOrderIdentifier",
				"serialNumber",
				"deploymentProfileAssignmentStatus",
				"enrollmentState",
				"azureAdDeviceId",
				"managedDeviceId",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-windowsautopilotdeviceidentity?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-enrollment-windowsautopilotdeviceidentity-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/autopilot/registration-overview",
					"https://learn.microsoft.com/en-us/autopilot/enrollment-autopilot",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsAutopilotDeviceIdentities()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Autopilot device identities: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsAutopilotDeviceIdentities().ByWindowsAutopilotDeviceIdentityId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Autopilot device identity: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			identity, ok := item.(betamodels.WindowsAutopilotDeviceIdentityable)
			if !ok {
				return ""
			}
			if name := safeStringValue(identity.GetDisplayName()); name != "" {
				return name
			}
			if serial := safeStringValue(identity.GetSerialNumber()); serial != "" {
				return serial
			}
			return safeStringValue(identity.GetId())
		},
	}, nil
}
