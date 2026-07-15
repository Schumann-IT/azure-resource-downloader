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

// NewDeviceManagementConfigurationPolicyHandler creates a handler for Intune
// Settings Catalog configuration policies
// (deviceManagement/configurationPolicies, Microsoft Graph beta).
//
// Fetch uses $expand=settings so the full polymorphic setting tree is returned
// (settings are not included in a plain GET). Settings Catalog policies use
// `name` instead of `displayName`.
func NewDeviceManagementConfigurationPolicyHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/deviceManagementConfigurationPolicies",
		documentation: models.ResourceDocumentation{
			Purpose:          "An Intune Settings Catalog configuration policy that applies settings via the unified settings catalog.",
			EmbeddedPayloads: []string{"settings (settingInstance / settingDefinition values, including secret settings)"},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().ConfigurationPolicies()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list configuration policies: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadevicemanagement.ConfigurationPoliciesDeviceManagementConfigurationPolicyItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.ConfigurationPoliciesDeviceManagementConfigurationPolicyItemRequestBuilderGetQueryParameters{
					Expand: []string{"settings"},
				},
			}
			item, err := client.DeviceManagement().ConfigurationPolicies().ByDeviceManagementConfigurationPolicyId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get configuration policy: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceManagement().ConfigurationPolicies().ByDeviceManagementConfigurationPolicyId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/deviceManagementConfigurationPolicies", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.DeviceManagementConfigurationPolicyable); ok {
				return safeStringValue(p.GetName())
			}
			return ""
		},
	}, nil
}
