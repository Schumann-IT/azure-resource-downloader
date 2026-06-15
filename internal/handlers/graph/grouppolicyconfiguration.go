package graph

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	msgraphbeta "github.com/microsoftgraph/msgraph-beta-sdk-go"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewGroupPolicyConfigurationHandler creates a handler for Intune
// Administrative Templates (deviceManagement/groupPolicyConfigurations,
// Microsoft Graph beta).
//
// The configured settings are not part of the policy object: they live in the
// child collection definitionValues (with their definition expanded), so Fetch
// retrieves them separately and attaches them to the model before
// serialization.
func NewGroupPolicyConfigurationHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/groupPolicyConfigurations",
		terraformType: "microsoft365_graph_beta_device_management_group_policy_configuration",
		documentation: docMeta(
			"An Intune Administrative Templates (ADMX-backed) group policy configuration.",
			nil,
			[]string{"definitionValues (the configured ADMX settings)", "presentationValues (the values supplied to each setting's presentations)"},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().GroupPolicyConfigurations()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list group policy configurations: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().GroupPolicyConfigurations().ByGroupPolicyConfigurationId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get group policy configuration: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
			}

			values, err := listGroupPolicyDefinitionValues(ctx, client, itemID)
			if err != nil {
				return nil, err
			}
			item.SetDefinitionValues(values)

			if assignments, err := client.DeviceManagement().GroupPolicyConfigurations().ByGroupPolicyConfigurationId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/groupPolicyConfigurations", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}

			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.GroupPolicyConfigurationable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}

// listGroupPolicyDefinitionValues pages through the definitionValues child
// collection of a group policy configuration with $expand=definition, so each
// configured setting carries its ADMX definition metadata.
func listGroupPolicyDefinitionValues(ctx context.Context, client *msgraphbeta.GraphServiceClient, configurationID string) ([]betamodels.GroupPolicyDefinitionValueable, error) {
	var values []betamodels.GroupPolicyDefinitionValueable

	builder := client.DeviceManagement().GroupPolicyConfigurations().ByGroupPolicyConfigurationId(configurationID).DefinitionValues()
	requestConfig := &betadevicemanagement.GroupPolicyConfigurationsItemDefinitionValuesRequestBuilderGetRequestConfiguration{
		QueryParameters: &betadevicemanagement.GroupPolicyConfigurationsItemDefinitionValuesRequestBuilderGetQueryParameters{
			Expand: []string{"definition"},
		},
	}
	for {
		resp, err := builder.Get(ctx, requestConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to list group policy definition values: %w (hint: requires 'DeviceManagementConfiguration.Read.All' permission in Microsoft Graph)", err)
		}
		if resp == nil {
			break
		}
		values = append(values, resp.GetValue()...)

		next := resp.GetOdataNextLink()
		if next == nil || *next == "" {
			break
		}
		builder = builder.WithUrl(*next)
		requestConfig = nil
	}

	return values, nil
}
