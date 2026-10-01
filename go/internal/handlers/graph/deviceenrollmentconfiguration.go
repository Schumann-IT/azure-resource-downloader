package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewDeviceEnrollmentConfigurationHandler creates a handler for Intune device
// enrollment configurations
// (deviceManagement/deviceEnrollmentConfigurations, Microsoft Graph beta).
// The collection is polymorphic: enrollment limits, platform restrictions,
// Enrollment Status Page (ESP), Windows Hello for Business and enrollment
// notification configurations, including the tenant defaults.
func NewDeviceEnrollmentConfigurationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/deviceEnrollmentConfigurations",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune device enrollment configuration, such as the Enrollment Status Page or enrollment restrictions.",
			KeySettings: []string{
	"priority",
	"deviceEnrollmentConfigurationType",
	"limit",
	"windowsRestriction",
	"iosRestriction",
	"androidForWorkRestriction",
	"platformRestriction",
	"showInstallationProgress",
	"selectedMobileAppIds",
	"installProgressTimeoutInMinutes",
	"allowDeviceUseOnInstallFailure",
},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
	"Edits apply to new enrollments only; devices already enrolled are not affected.",
	"Restrictions: priority 1 is highest and only the highest-priority assigned policy applies; the default applies to everyone not covered and always to enrollments that aren't user-driven.",
	"Enrollment Status Page: a device-targeted profile wins over a user-targeted one, then the default.",
},
			RelatedTypes: []string{
	"Microsoft.Graph/groups (assignment target groups)",
	"Microsoft.Graph/windowsAutopilotDeploymentProfiles (ESP applies during Autopilot)",
	"Microsoft.Graph/mobileApps (ESP blocking apps in selectedMobileAppIds)",
	"Microsoft.Graph/notificationMessageTemplates (enrollment notifications)",
	"Microsoft.Graph/assignmentFilters (platform restrictions support filters)",
	"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
},
			SubtypeNote: "Polymorphic (@odata.type): deviceEnrollmentLimitConfiguration, deviceEnrollmentPlatformRestrictionsConfiguration (default, all platforms), deviceEnrollmentPlatformRestrictionConfiguration (one platform), deviceEnrollmentWindowsHelloForBusinessConfiguration, windows10EnrollmentCompletionPageConfiguration (Enrollment Status Page), deviceEnrollmentNotificationConfiguration, deviceComanagementAuthorityConfiguration and windowsRestoreDeviceEnrollmentConfiguration (tenant-wide, not targetable); deviceEnrollmentConfigurationType values starting with 'default' mark the built-in defaults.",
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-deviceenrollmentconfiguration?view=graph-rest-beta",
				Permissions: "https://learn.microsoft.com/en-us/graph/api/intune-onboarding-deviceenrollmentconfiguration-list?view=graph-rest-beta",
			SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-onboarding-deviceenrollmentplatformrestriction?view=graph-rest-beta",
BestPractices: []string{
		"https://learn.microsoft.com/en-us/intune/device-enrollment/restrictions",
		"https://learn.microsoft.com/en-us/intune/device-enrollment/windows/setup-status-page",
		"https://learn.microsoft.com/en-us/intune/device-security/identity-protection/configure-tenant-wide-policy",
	},
},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().DeviceEnrollmentConfigurations()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list device enrollment configurations: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().DeviceEnrollmentConfigurations().ByDeviceEnrollmentConfigurationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get device enrollment configuration: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().DeviceEnrollmentConfigurations().ByDeviceEnrollmentConfigurationId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceEnrollmentConfigurations", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if c, ok := item.(betamodels.DeviceEnrollmentConfigurationable); ok {
				return safeStringValue(c.GetDisplayName())
			}
			return ""
		},
	}, nil
}
