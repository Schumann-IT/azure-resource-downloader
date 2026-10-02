package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewAppleUserInitiatedEnrollmentProfileHandler creates a handler for Apple
// user-initiated enrollment profiles
// (deviceManagement/appleUserInitiatedEnrollmentProfiles, Microsoft Graph
// beta).
func NewAppleUserInitiatedEnrollmentProfileHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:      "Microsoft.Graph/appleUserInitiatedEnrollmentProfiles",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune enrollment type profile that decides how assigned users enroll iOS/iPadOS devices: device enrollment, account-driven user enrollment, web-based device enrollment or user choice.",
			KeySettings:         []string{"defaultEnrollmentType", "availableEnrollmentTypeOptions", "platform", "priority"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle: []string{
				"Profiles target user groups only (device groups aren't supported); when several apply, the higher-priority profile wins (0 is highest).",
				"User enrollment with Company Portal is no longer supported for newly enrolled devices; account-driven user enrollment and web-based device enrollment need iOS/iPadOS 15 or later, and a web-based profile enrolls users on older versions through Company Portal device enrollment instead.",
				"Account-driven user enrollment needs Managed Apple IDs (or federated authentication with Apple Business) and a service-discovery file under /.well-known/com.apple.remotemanagement on the sign-in domain, and uses JIT registration with Microsoft Authenticator; web-based device enrollment can use JIT registration too, and both need Microsoft Authenticator for work apps.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (assignment target user groups)",
				"Microsoft.Graph/applePushNotificationCertificate (prerequisite)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-appleuserinitiatedenrollmentprofile?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-enrollment-appleuserinitiatedenrollmentprofile-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-enrollment-appleownertypeenrollmenttype?view=graph-rest-beta",
				AdminCenter:     "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/DevicesIosMenu/~/iosEnrollment",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/device-enrollment/apple/setup-account-driven-user",
					"https://learn.microsoft.com/en-us/intune/device-enrollment/apple/setup-web-based-ios",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().AppleUserInitiatedEnrollmentProfiles()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Apple user-initiated enrollment profiles: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().AppleUserInitiatedEnrollmentProfiles().ByAppleUserInitiatedEnrollmentProfileId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get Apple user-initiated enrollment profile: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().AppleUserInitiatedEnrollmentProfiles().ByAppleUserInitiatedEnrollmentProfileId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/appleUserInitiatedEnrollmentProfiles", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.AppleUserInitiatedEnrollmentProfileable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
