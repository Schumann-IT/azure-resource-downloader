package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
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
		azureType:      "Microsoft.Graph/roleScopeTags",
		hasAssignments: true,
		documentation: models.ResourceDocumentation{
			Template:            referencedPromptTemplateText,
			Purpose:             "An Intune RBAC scope tag used to scope which admins can see and manage which objects.",
			RequiredPermissions: []string{"DeviceManagementRBAC.Read.All"},
			Lifecycle: []string{
				"The built-in Default tag is added automatically to every untagged object that supports scope tags; an object or role can carry at most 100 tags.",
				"New objects automatically get the scope tags of the admin who creates them; Corp device identifiers, Windows Autopilot devices, device compliance locations and Jamf devices don't support scope tags.",
				"A tag's assignments apply it automatically to devices in the targeted groups (overwriting manually assigned tags); admins whose role assignment includes a tag can't update or delete that tag.",
				"Creating, updating or deleting scope tags requires the Intune Administrator Microsoft Entra role.",
				"Tags limit visibility only for admins whose role assignment carries scope tags: an assignment without tags sees all objects its permissions cover, and Microsoft Entra roles such as Intune Administrator aren't limited by tags at all.",
				"By default Intune merges a permission category across an admin's role assignments with different scope tags, so tags may not separate what the admin can do; the one-way Scoped permissions tenant setting (opt-in preview, not exported) confines each assignment's permissions to its own tags.",
				"Endpoint analytics custom device scopes filter reports by scope tag; a device scope whose tag is deleted stops working until it is edited.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/roleDefinitions",
				"Microsoft.Graph/groups (tag assignments)",
				"Microsoft.Graph/vppTokens (VPP apps and books inherit the token's tags)",
				"Microsoft.Graph/deviceConfigurations, Microsoft.Graph/deviceCompliancePolicies, Microsoft.Graph/mobileApps and every other Intune type that carries roleScopeTagIds (tagged objects)",
			},
			KeySettings: []string{"assignments[].target (groups whose devices get the tag)", "isBuiltIn"},
			Links: models.ResourceLinks{
				EndpointDocs:  "https://learn.microsoft.com/en-us/graph/api/resources/intune-rbac-rolescopetag?view=graph-rest-beta",
				Permissions:   "https://learn.microsoft.com/en-us/graph/api/intune-rbac-rolescopetag-list?view=graph-rest-beta",
				BestPractices: []string{"https://learn.microsoft.com/en-us/intune/fundamentals/role-based-access-control/scope-tags"},
			},
		},
		probe: func(ctx context.Context) error {
			_, err := client.DeviceManagement().RoleScopeTags().Get(ctx, &betadevicemanagement.RoleScopeTagsRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.RoleScopeTagsRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list role scope tags: %w (hint: requires 'DeviceManagementRBAC.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
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
