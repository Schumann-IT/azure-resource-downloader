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

// NewDepOnboardingSettingHandler creates a handler for Apple Automated Device
// Enrollment (ADE/DEP) tokens (deviceManagement/depOnboardingSettings,
// Microsoft Graph beta).
//
// The enrollment profiles tied to a token are not part of the token object:
// they live in the child collection enrollmentProfiles, so Fetch retrieves
// them separately and attaches them to the model before serialization.
func NewDepOnboardingSettingHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/depOnboardingSettings",
		documentation: models.ResourceDocumentation{
			Template:            credentialPromptTemplateText,
			Purpose:             "Apple Automated Device Enrollment (DEP/ABM) onboarding tokens used by Intune to sync Apple-enrolled devices.",
			KeySettings: []string{
	"tokenExpirationDateTime",
	"appleIdentifier",
	"tokenType",
	"tokenName",
	"syncedDeviceCount",
	"lastSuccessfulSyncDateTime",
	"lastSyncErrorCode",
	"enrollmentProfiles",
},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
	"Renew the token yearly: download a new server token in Apple Business Manager or Apple School Manager and use Renew token in Intune with the Apple ID that created the original token; also renew when that account's password changes or its owner leaves.",
	"Downloading a new token in Apple Business Manager invalidates the one Intune uses; an expired or invalid token causes sync and automated-enrollment errors.",
	"A token can be deleted only after its devices and enrollment profiles are removed; deleting devices from a token removes them from Intune management.",
},
			EmbeddedPayloads: []string{"enrollmentProfiles[].managementCertificates[].certificate (base64 management certificate)"},
RelatedTypes: []string{
	"Microsoft.Graph/applePushNotificationCertificate (prerequisite for automated device enrollment)",
	"Microsoft.Graph/groups (enrollment profiles' enrollmentTimeAzureAdGroupIds)",
	"Microsoft.Graph/vppTokens (iOS enrollment profiles' companyPortalVppTokenId)",
	"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
},
SubtypeNote: "enrollmentProfiles (attached child collection) is polymorphic: depIOSEnrollmentProfile, depMacOSEnrollmentProfile and the tvOS/visionOS variants inherit depEnrollmentBaseProfile - identify each profile's type from @odata.type.",
Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-deponboardingsetting?view=graph-rest-beta",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/intune-enrollment-deponboardingsetting-list?view=graph-rest-beta",
			SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-depenrollmentbaseprofile?view=graph-rest-beta",
BestPractices: []string{
		"https://learn.microsoft.com/en-us/intune/device-enrollment/apple/setup-apple-token",
		"https://learn.microsoft.com/en-us/intune/device-enrollment/apple/manage-devices-tokens-apple",
	},
},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DepOnboardingSettings()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list DEP onboarding settings: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DepOnboardingSettings().ByDepOnboardingSettingId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get DEP onboarding setting: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}

			profiles, err := listDepEnrollmentProfiles(ctx, client, itemID)
			if err != nil {
				return nil, err
			}
			item.SetEnrollmentProfiles(profiles)

			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			setting, ok := item.(betamodels.DepOnboardingSettingable)
			if !ok {
				return ""
			}
			if name := safeStringValue(setting.GetTokenName()); name != "" {
				return name
			}
			if appleID := safeStringValue(setting.GetAppleIdentifier()); appleID != "" {
				return appleID
			}
			return safeStringValue(setting.GetId())
		},
	}, nil
}

// listDepEnrollmentProfiles pages through the enrollmentProfiles child
// collection of a DEP onboarding setting.
func listDepEnrollmentProfiles(ctx context.Context, client *msgraphbeta.GraphServiceClient, settingID string) ([]betamodels.EnrollmentProfileable, error) {
	var profiles []betamodels.EnrollmentProfileable

	builder := client.DeviceManagement().DepOnboardingSettings().ByDepOnboardingSettingId(settingID).EnrollmentProfiles()
	for {
		resp, err := builder.Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list DEP enrollment profiles: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
		}
		if resp == nil {
			break
		}
		profiles = append(profiles, resp.GetValue()...)

		next := resp.GetOdataNextLink()
		if next == nil || *next == "" {
			break
		}
		builder = builder.WithUrl(*next)
	}

	return profiles, nil
}
