package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	_ "embed"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	graphgroups "github.com/microsoftgraph/msgraph-sdk-go/groups"
	msgraphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
)

// groupPromptTemplateText overrides the default documentation prompt template
// for groups: groups carry no settings payload and no assignments of their own
// (they are the assignment targets), so the default settings/assignments-heavy
// layout does not fit. See models.ResourceDocumentation.Template.
//
//go:embed group_prompt.tmpl
var groupPromptTemplateText string

// NewGroupHandler creates a handler for Entra groups (groups, Microsoft Graph
// v1.0), including dynamic groups with their membership rules.
//
// NOTE: this exports the full directory group list, which can be very large in
// big tenants.
func NewGroupHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/groups",
		documentation: models.ResourceDocumentation{
			Purpose: "An Entra ID group (Microsoft 365, security, mail-enabled security or distribution), often used as an assignment target for policies and apps.",
			KeySettings: []string{
	"groupTypes",
	"membershipRule (for dynamic groups)",
	"membershipRuleProcessingState",
	"securityEnabled",
	"mailEnabled",
	"isAssignableToRole",
},
			RequiredPermissions: []string{"Group.Read.All"},
			Lifecycle: []string{
	"Deleted Microsoft 365 groups are soft-deleted and can be restored within 30 days with the same object ID; soft delete for cloud security groups is in preview, and distribution groups can't be restored.",
	"Dynamic membership rules re-evaluate automatically as attributes change (processing can be paused via membershipRuleProcessingState); dynamic groups need Entra ID P1.",
	"isAssignableToRole can only be set at creation; groups referenced by policy assignments should not be deleted while in use.",
},
			RelatedTypes: []string{
	"Microsoft.Graph/conditionalAccessPolicies (include and exclude groups)",
	"Microsoft.Graph/authenticationMethodsPolicy (method targets)",
	"Microsoft.Graph/roleScopeTags (scope tag assignments)",
	"Microsoft.Graph/deviceConfigurations, Microsoft.Graph/deviceManagementConfigurationPolicies, Microsoft.Graph/deviceCompliancePolicies, Microsoft.Graph/mobileApps and every other Intune type with assignments (assignments[].target.groupId)",
},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/group?view=graph-rest-1.0",
				Permissions:  "https://learn.microsoft.com/en-us/graph/api/group-list?view=graph-rest-1.0",
			AdminCenter: "https://entra.microsoft.com/#view/Microsoft_AAD_IAM/GroupsManagementMenuBlade/~/AllGroups/menuId/AllGroups",
},
			Template: groupPromptTemplateText,
		},
		probe: func(ctx context.Context) error {
			_, err := client.Groups().Get(ctx, &graphgroups.GroupsRequestBuilderGetRequestConfiguration{
				QueryParameters: &graphgroups.GroupsRequestBuilderGetQueryParameters{Top: probeTop(), Select: probeSelect()},
			})
			if err != nil {
				return fmt.Errorf("failed to list groups: %w (hint: requires 'Group.Read.All' permission in Microsoft Graph)", err)
			}
			return nil
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.Groups()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list groups: %w (hint: requires 'Group.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.Groups().ByGroupId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get group: %w (hint: requires 'Group.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if g, ok := item.(msgraphmodels.Groupable); ok {
				return safeStringValue(g.GetDisplayName())
			}
			return ""
		},
	}, nil
}
