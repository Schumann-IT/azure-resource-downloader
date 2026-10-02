package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewRoleDefinitionHandler creates a handler for custom Intune RBAC role
// definitions (deviceManagement/roleDefinitions, Microsoft Graph beta).
// Built-in role definitions are skipped during listing (they are not tenant
// configuration), matching the reference exporter's isBuiltIn=false filter.
func NewRoleDefinitionHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/roleDefinitions",
		documentation: models.ResourceDocumentation{
			Template: referencedPromptTemplateText,
			Purpose:  "An Intune RBAC role definition listing the permissions granted by the role.",
			KeySettings: []string{
				"rolePermissions[].resourceActions[].allowedResourceActions",
				"rolePermissions[].resourceActions[].notAllowedResourceActions",
				"roleScopeTagIds (tags on the role itself: they control which scoped admins can see the role, not which objects its holders manage; that is set by the role assignment's scope tags, not exported)",
			},
			RequiredPermissions: []string{"DeviceManagementRBAC.Read.All"},
			Lifecycle: []string{
				"Only custom Intune roles are exported (built-in roles can't be edited); their assignments (admin groups, scope groups and scope tags) live in roleAssignments, which is not exported.",
				"No exported type references a role definition: a role is enforced only through its role assignments, so the export holds no referencing policies for it.",
				"Permissions from several role assignments add up and one can't deny what another grants; by default Intune also merges a permission category across assignments with different scope tags, which can over-grant access, unless the Scoped permissions setting (one-way, opt-in preview, not exported) is on.",
				"Intune RBAC doesn't restrict Microsoft Entra roles: Global Administrator and Intune Administrator have full read/write access to Intune whatever the custom roles and scope tags allow.",
				"Custom roles are created, edited and assigned by the Intune Administrator Entra role or, with less privilege, the built-in Intune Role Administrator role or a custom role with the Roles permissions plus Organization Read.",
				"Effective access shows under Tenant administration > Roles > Monitor: My permissions, Roles by permission (which role, assignment and group grant a permission) and Admin permissions (per user).",
				"Review custom roles against least privilege regularly.",
			},
			RelatedTypes: []string{
				"Microsoft.Graph/roleScopeTags",
				"Microsoft.Graph/groups (through role assignments, not exported)",
			},
			SubtypeNote: "Items may carry @odata.type deviceAndAppManagementRoleDefinition, which inherits roleDefinition and adds no properties.",
			Links: models.ResourceLinks{
				EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-rbac-roledefinition?view=graph-rest-beta",
				Permissions:     "https://learn.microsoft.com/en-us/graph/api/intune-rbac-roledefinition-list?view=graph-rest-beta",
				SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/intune-rbac-rolepermission?view=graph-rest-beta",
				BestPractices: []string{
					"https://learn.microsoft.com/en-us/intune/fundamentals/role-based-access-control/create-custom-role",
				},
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().RoleDefinitions()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list role definitions: %w (hint: requires 'DeviceManagementRBAC.Read.All' permission in Microsoft Graph)", err)
				}
				if resp == nil {
					break
				}
				for _, item := range resp.GetValue() {
					if builtIn := item.GetIsBuiltIn(); builtIn != nil && *builtIn {
						continue
					}
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
			item, err := client.DeviceManagement().RoleDefinitions().ByRoleDefinitionId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get role definition: %w (hint: requires 'DeviceManagementRBAC.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if d, ok := item.(betamodels.RoleDefinitionable); ok {
				return safeStringValue(d.GetDisplayName())
			}
			return ""
		},
	}, nil
}
