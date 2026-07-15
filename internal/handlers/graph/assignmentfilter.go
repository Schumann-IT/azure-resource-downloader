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
			Purpose:             "An Intune assignment filter (device/app filter) used to refine policy and app assignments.",
			KeySettings:         []string{"platform", "rule"},
			EmbeddedPayloads:    []string{"rule (filter rule expression)"},
			RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
			Lifecycle:           "Filters are referenced by assignments across many policy and app types; deleting a filter breaks the assignments that reference it. Rule changes re-evaluate at the next device/app check-in.",
			RelatedTypes:        []string{"all assignable Intune types (policies, profiles and apps reference filters by ID in their assignments)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-policyset-deviceandappmanagementassignmentfilter?view=graph-rest-beta",
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
