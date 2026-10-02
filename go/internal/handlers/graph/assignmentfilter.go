package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewAssignmentFilterHandler creates a handler for Intune assignment filters
// (deviceManagement/assignmentFilters, Microsoft Graph beta).
func NewAssignmentFilterHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/assignmentFilters",
		documentation: models.ResourceDocumentation{
			Template:    referencedPromptTemplateText,
			Purpose:     "An Intune assignment filter (device/app filter) used to refine policy and app assignments.",
			KeySettings: []string{"platform", "rule", "assignmentFilterManagementType", "payloads (where the filter is used)"},

			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle: []string{
				"A filter that is still used in any assignment can't be deleted; remove it from all assignments first.",
				"Rule changes re-evaluate at the next device or app check-in; a property not yet set when the device enrolls (such as deviceCategory) evaluates as empty, and an app installed on that result isn't removed automatically once the property is set.",
				"At most 200 filters per tenant and 3,072 characters per filter; app filters (assignmentFilterManagementType apps) apply only to app protection and app configuration policies.",
				"Each referencing assignment stores one filter id with its own include or exclude mode, and an assignment can only use a filter whose platform matches the policy's platform.",
				"For managed devices, when a device gets the same policy through several assignments, Exclude mode wins over no filter, which wins over Include, and filters in the same mode combine with OR; for apps the winning intent is chosen first.",
				"Results appear up to 30 minutes after evaluation and are kept 30 days, per device under Devices > All devices > (device) > Filter evaluation and in app and Settings Catalog device status reports (app filters: App protection and App configuration status reports); Associated Assignments lists the policies and modes using it.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/groups (filters refine group assignments)",
				"Microsoft.Graph/deviceConfigurations",
				"Microsoft.Graph/deviceManagementConfigurationPolicies",
				"Microsoft.Graph/deviceCompliancePolicies",
				"Microsoft.Graph/groupPolicyConfigurations",
				"Microsoft.Graph/mobileApps",
				"Microsoft.Graph/mobileAppConfigurations",
				"Microsoft.Graph/targetedManagedAppConfigurations",
				"Microsoft.Graph/iosManagedAppProtections",
				"Microsoft.Graph/androidManagedAppProtections",
				"Microsoft.Graph/windowsManagedAppProtections",
				"Microsoft.Graph/deviceHealthScripts",
				"Microsoft.Graph/deviceEnrollmentConfigurations (platform restrictions)",
				"Microsoft.Graph/roleScopeTags (roleScopeTags)",
			},
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-policyset-deviceandappmanagementassignmentfilter?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-policyset-deviceandappmanagementassignmentfilter-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/intune/fundamentals/filters/ref-device-properties",
				BestPractices:   []string{"https://learn.microsoft.com/en-us/intune/fundamentals/filters/performance-recommendations"},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().AssignmentFilters()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list assignment filters: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().AssignmentFilters().ByDeviceAndAppManagementAssignmentFilterId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get assignment filter: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if f, ok := item.(betamodels.DeviceAndAppManagementAssignmentFilterable); ok {
				return safeStringValue(f.GetDisplayName())
			}
			return ""
		},
	}, nil
}
