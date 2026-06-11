package handlers

import (
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
		azureType:     "Microsoft.Graph/roleScopeTags",
		terraformType: "microsoft365_graph_beta_device_management_role_scope_tag",
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
