package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
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
		azureType: "Microsoft.Graph/mobileApps",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune managed application (e.g. Win32, store, line-of-business app) and its deployment configuration.",
			KeySettings:         []string{"installCommandLine", "uninstallCommandLine", "minimumSupportedOperatingSystem"},
			EmbeddedPayloads:    []string{"detectionRules", "requirementRules", "installExperience", "returnCodes", "largeIcon (base64 image)"},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle:           []string{"Deleting an app from Intune does not uninstall it from devices (assign an uninstall intent first); Win32 apps follow supersedence rules when updated."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)", "Microsoft.Graph/assignmentFilters (assignment filters)", "Microsoft.Graph/mobileAppConfigurations (app configuration policies)"},
			SubtypeNote:         "Highly polymorphic (win32LobApp, winGetApp, macOSPkgApp, iosStoreApp, officeSuiteApp, ...) - identify the concrete app type from @odata.type first; detection/requirement rules and install experience are subtype-specific.",
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-shared-mobileapp?view=graph-rest-beta",
			},
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
