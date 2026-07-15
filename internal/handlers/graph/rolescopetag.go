package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewRoleScopeTagHandler creates a handler for Intune RBAC scope tags
// (deviceManagement/roleScopeTags, Microsoft Graph beta).
func NewRoleScopeTagHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/roleScopeTags",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune RBAC scope tag used to scope which admins can see and manage which objects.",
			RequiredPermissions: []string{"DeviceManagementRBAC.Read.All"},
			Lifecycle:           []string{"Scope tags partition RBAC visibility; deleting a tag removes it from all tagged objects and role assignments.", "The default tag cannot be deleted."},
			RelatedTypes:        []string{"Microsoft.Graph/roleDefinitions"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-rbac-rolescopetag?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().RoleScopeTags()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list role scope tags: %w (hint: requires 'DeviceManagementRBAC.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().RoleScopeTags().ByRoleScopeTagId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get role scope tag: %w (hint: requires 'DeviceManagementRBAC.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().RoleScopeTags().ByRoleScopeTagId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/roleScopeTags", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if t, ok := item.(betamodels.RoleScopeTagable); ok {
				return safeStringValue(t.GetDisplayName())
			}
			return ""
		},
	}, nil
}
