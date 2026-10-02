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
			Template: recordPromptTemplateText,
			Purpose:  "An Intune Mobile Threat Defense connector to an MTD partner (a third-party vendor, Microsoft Defender for Endpoint or Windows Security Center) whose device threat data feeds compliance and app protection evaluation.",
			KeySettings: []string{
				"androidEnabled",
				"iosEnabled",
				"macEnabled",
				"windowsEnabled",
				"partnerState",
				"partnerUnresponsivenessThresholdInDays",
				"androidDeviceBlockedOnMissingPartnerData, iosDeviceBlockedOnMissingPartnerData, macDeviceBlockedOnMissingPartnerData, windowsDeviceBlockedOnMissingPartnerData",
				"androidMobileApplicationManagementEnabled, iosMobileApplicationManagementEnabled, windowsMobileApplicationManagementEnabled",
				"partnerUnsupportedOsVersionBlocked",
				"allowPartnerToCollectIOSApplicationMetadata, allowPartnerToCollectIOSPersonalApplicationMetadata, allowPartnerToCollectIosCertificateMetadata, allowPartnerToCollectIosPersonalCertificateMetadata (inventory data shared with the partner; the personal-device options add unmanaged apps and certificates)",
				"lastHeartbeatDateTime",
			},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
				"A non-Microsoft partner's connector is added under Tenant administration > Connectors and tokens > Mobile Threat Defense after the Intune connector is set up in the partner console (Defender for Endpoint's is set up under Endpoint security > Microsoft Defender for Endpoint); its toggles and threshold are admin-set settings, and the Mobile Threat Defense pane shows Connection status and Last synchronized.",
				"Adding or changing the connector needs the Endpoint Security Manager role or a custom role with Read and Modify on the Mobile Threat Defense permission.",
				"If the partner sends no heartbeat for partnerUnresponsivenessThresholdInDays the connector becomes unresponsive and Intune ignores the partner's compliance state; the *DeviceBlockedOnMissingPartnerData settings decide whether devices can be compliant before partner data arrives.",
				"partnerState: notSetUp means setup is incomplete, available means set up but no platform toggle is on, enabled means at least one toggle is on, unavailable means the connector is deprovisioned and the partner must have Intune provision it again, error means the connector has an error code.",
				"With two or more partners enabled for compliance on one platform, every device of that platform must run each partner's app and is noncompliant if one fails to submit a scan; Defender for Endpoint beside a third-party partner can instead be checked by separate compliance policies per group.",
				"Microsoft Defender for Endpoint and a non-Microsoft partner can both be enabled for app protection (MAM) evaluation on a platform; an app uses only one of them, chosen by each app protection policy's mobileThreatDefensePartnerPriority (Defender when unset).",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/deviceCompliancePolicies (threat-level based compliance)",
				"Microsoft.Graph/iosManagedAppProtections (maximum allowed threat level)",
				"Microsoft.Graph/androidManagedAppProtections (maximum allowed threat level)",
			},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-mobilethreatdefenseconnector?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-onboarding-mobilethreatdefenseconnector-list?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-security/mobile-threat-defense/enable-connector",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().MobileThreatDefenseConnectors()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list mobile threat defense connectors: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
				return nil, fmt.Errorf("failed to get mobile threat defense connector: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
