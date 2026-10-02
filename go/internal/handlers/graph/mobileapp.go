package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadeviceappmanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/deviceappmanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewMobileAppHandler creates a handler for Intune applications
// (deviceAppManagement/mobileApps, Microsoft Graph beta). The collection is
// highly polymorphic (win32LobApp, winGetApp, macOSPkgApp, iosStoreApp,
// officeSuiteApp, ...) and includes Microsoft built-in apps.
func NewMobileAppHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/mobileApps",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune managed application (e.g. Win32, store, line-of-business app) and its deployment configuration.",
			KeySettings: []string{
				"installCommandLine",
				"uninstallCommandLine",
				"minimumSupportedOperatingSystem",
				"installExperience (runAsAccount, deviceRestartBehavior; with basedOnReturnCode a hardReboot return code restarts the device immediately and a softReboot code only tells the user a restart is needed)",
				"returnCodes",
				"rules",
				"allowedArchitectures",
				"minimumSupportedWindowsRelease",
				"packageIdentifier",
			},
			EmbeddedPayloads: []string{
				"largeIcon.value (base64 image)",
				"detectionRules[].scriptContent, requirementRules[].scriptContent, rules[].scriptContent (base64 PowerShell detection/requirement scripts)",
				"officeConfigurationXml (officeSuiteApp Office configuration XML)",
				"preInstallScript.scriptContent, postInstallScript.scriptContent (macOSPkgApp base64 shell scripts)",
			},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle: []string{
				"To remove an app from devices assign it as Uninstall (and remove the install assignment for those groups); delete the app from Intune only after removing its assignments and revoking any VPP licenses.",
				"Win32 apps aren't uninstalled when a device unenrolls, and Win32 apps in a dependency relationship can't be deleted; supersedence is configured per app and the superseding app needs its own assignment.",
				"Win32 apps need Windows Enterprise, Pro or Education devices that are enrolled in Intune and Microsoft Entra registered, joined or hybrid joined; the Intune Management Extension that installs them is added automatically and checks for new Win32 assignments every hour.",
				"For most app types an Available assignment works only for user groups; Win32 apps and apps for Android Enterprise fully managed (COBO) or corporate-owned personally enabled (COPE) devices can also be Available to device groups.",
				"When groups give one user or device different intents, Intune resolves the conflict (Required beats Uninstall; some pairs keep both Required and Available) before assignment filters, so a filter may not act as expected; an exclusion overrides only an inclusion of the same group type (user or device).",
				"Install results show in Apps > Monitor > App install status and, per app, in Device install status and User install status (Installed, Failed, Install pending, Not installed, Not applicable); Microsoft Store and Android store apps assigned as Available report no install status.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/assignmentFilters (assignment filters)",
				"Microsoft.Graph/mobileAppConfigurations (app configuration policies)",
				"Microsoft.Graph/vppTokens (iosVppApp.vppTokenId)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			SubtypeNote: "Highly polymorphic (win32LobApp, winGetApp, macOSPkgApp, iosStoreApp, officeSuiteApp, ...) - identify the concrete app type from @odata.type first; detection/requirement rules and install experience are subtype-specific.",
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-shared-mobileapp?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-shared-mobileapp-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-apps-win32lobapp?view=graph-rest-beta",
				AdminCenter:     "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/AppsMenu/~/allApps",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/app-management/deployment/add-win32",
					"https://learn.microsoft.com/en-us/intune/app-management/deployment/configure-win32-supersedence",
				},
			},
		},
		probe: func(ctx context.Context) error {
			_, err := client.DeviceAppManagement().MobileApps().Get(ctx, &betadeviceappmanagement.MobileAppsRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.MobileAppsRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list mobile apps: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().MobileApps()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list mobile apps: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceAppManagement().MobileApps().ByMobileAppId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get mobile app: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().MobileApps().ByMobileAppId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/mobileApps", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if a, ok := item.(betamodels.MobileAppable); ok {
				return safeStringValue(a.GetDisplayName())
			}
			return ""
		},
	}, nil
}
