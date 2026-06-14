package graph

import (
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
		azureType:     "Microsoft.Graph/roleDefinitions",
		terraformType: "microsoft365_graph_beta_device_management_role_definition",
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
