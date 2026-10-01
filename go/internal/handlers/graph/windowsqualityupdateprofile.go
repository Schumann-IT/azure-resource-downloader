package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewWindowsQualityUpdateProfileHandler creates a handler for Windows quality
// update profiles (deviceManagement/windowsQualityUpdateProfiles, Microsoft
// Graph beta).
func NewWindowsQualityUpdateProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/windowsQualityUpdateProfiles",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Windows quality (expedited) update profile that accelerates a specific quality update.",
			KeySettings: []string{
				"expeditedUpdateSettings (qualityUpdateRelease, daysUntilForcedReboot)",
				"releaseDateDisplayName",
				"deployableContentDisplayName",
			},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"Expedites one quality update per policy: it bypasses ring deferrals without pausing or changing the monthly update policies, and skips devices that already have that update or a newer one.",
				"Deleting the policy doesn't uninstall a completed update; in-progress installs are cancelled on a best-effort basis.",
				"Requires Intune Plan 1 and a Windows license with the Autopatch entitlement (Pro, Enterprise or Education, not LTSC) and Entra joined or hybrid joined devices; the separate Windows quality update policies (monthly updates, hotpatch) are a different resource that is not exported.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target groups)",
				"Microsoft.Graph/deviceConfigurations (update rings whose deferral the expedite bypasses)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-softwareupdate-windowsqualityupdateprofile?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-softwareupdate-windowsqualityupdateprofile-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-softwareupdate-expeditedwindowsqualityupdatesettings?view=graph-rest-beta",
				BestPractices:   []string{"https://learn.microsoft.com/en-us/intune/device-updates/windows/configure-expedite-policy"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().WindowsQualityUpdateProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Windows quality update profiles: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().WindowsQualityUpdateProfiles().ByWindowsQualityUpdateProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Windows quality update profile: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().WindowsQualityUpdateProfiles().ByWindowsQualityUpdateProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/windowsQualityUpdateProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.WindowsQualityUpdateProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
