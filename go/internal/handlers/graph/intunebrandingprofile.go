package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewIntuneBrandingProfileHandler creates a handler for Intune branding
// profiles (deviceManagement/intuneBrandingProfiles, Microsoft Graph beta).
// Note: branding profiles use `profileName` instead of `displayName`.
func NewIntuneBrandingProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/intuneBrandingProfiles",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose: "An Intune Company Portal customization profile: branding, support contacts, privacy text, enrollment prompts, app sources and self-service device actions.",

			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
				"One default profile plus up to 25 profiles targeted at user groups (device groups aren't supported).",
				"Hiding Remove and Reset (isRemoveDeviceDisabled, isFactoryResetDisabled) exists only in the default profile and only hides Company Portal actions; it doesn't restrict device settings.",
				"enrollmentAvailability doesn't apply to iOS/iPadOS Automated Device Enrollment devices but does apply to Samsung Knox Mobile Enrollment (KME) devices, where unavailable stops enrollment in the out-of-box flow; Configuration Manager apps show only in the Windows Company Portal.",
				"Distinct from Entra sign-in branding (organizationalBranding).",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target user groups)",
				"Microsoft.Graph/deviceCategories (disableDeviceCategorySelection hides the category prompt)",
				"Microsoft.Graph/roleScopeTags (roleScopeTagIds)",
			},
			KeySettings: []string{
				"isDefaultProfile",
				"enrollmentAvailability",
				"companyPortalBlockedActions",
				"isRemoveDeviceDisabled",
				"isFactoryResetDisabled",
				"disableDeviceCategorySelection",
				"privacyUrl",
				"customPrivacyMessage",
				"contactITName",
				"contactITEmailAddress",
				"contactITPhoneNumber",
			},
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-wip-intunebrandingprofile?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-wip-intunebrandingprofile-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-shared-companyportalblockedaction?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/app-management/configuration/configure-company-portal",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().IntuneBrandingProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Intune branding profiles: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().IntuneBrandingProfiles().ByIntuneBrandingProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Intune branding profile: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().IntuneBrandingProfiles().ByIntuneBrandingProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/intuneBrandingProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.IntuneBrandingProfileable); ok {
				return safeStringValue(p.GetProfileName())
			}
			return ""
		},
	}, nil
}
