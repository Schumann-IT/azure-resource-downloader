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
		azureType: "Microsoft.Graph/deviceEnrollmentConfigurations",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune device enrollment configuration, such as the Enrollment Status Page or enrollment restrictions.",
			KeySettings:         []string{"priority", "platformRestrictions", "blockUntilComplete"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           []string{"Changes affect only future enrollments; existing devices keep their applied configuration.", "Priority order matters when multiple configurations target a user."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)", "Microsoft.Graph/windowsAutopilotDeploymentProfiles (ESP applies during Autopilot)"},
			SubtypeNote:         "Polymorphic (@odata.type): enrollment limits, platform restrictions, Windows Hello for Business, ESP (windows10EnrollmentCompletionPageConfiguration), enrollment notifications - identify the concrete type first.",
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-shared-deviceenrollmentconfiguration?view=graph-rest-beta",
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
