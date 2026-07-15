package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadeviceappmanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/deviceappmanagement"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewIosManagedAppProtectionHandler creates a handler for Intune iOS app
// protection (MAM) policies (deviceAppManagement/iosManagedAppProtections,
// Microsoft Graph beta).
//
// Fetch uses $expand=apps so the targeted app list is included (it is not
// returned by a plain GET).
func NewIosManagedAppProtectionHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/iosManagedAppProtections",
		documentation: models.ResourceDocumentation{
			Purpose:             "An Intune iOS App Protection (MAM) policy controlling data protection for managed apps.",
			KeySettings:         []string{"dataBackupBlocked", "managedBrowserToOpenLinksRequired", "pinRequired", "allowedOutboundDataTransferDestinations"},
			RequiredPermissions: []string{"DeviceManagementApps.Read.All"},
			Lifecycle:           []string{"Policy changes apply at the protected app's next check-in; deleting a policy removes app protection from targeted apps (protected data remains until a selective wipe is issued)."},
			RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)", "Microsoft.Graph/assignmentFilters (assignment filters)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-mam-iosmanagedappprotection?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().IosManagedAppProtections()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list iOS managed app protections: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadeviceappmanagement.IosManagedAppProtectionsIosManagedAppProtectionItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.IosManagedAppProtectionsIosManagedAppProtectionItemRequestBuilderGetQueryParameters{
					Expand: []string{"apps"},
				},
			}
			item, err := client.DeviceAppManagement().IosManagedAppProtections().ByIosManagedAppProtectionId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get iOS managed app protection: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().IosManagedAppProtections().ByIosManagedAppProtectionId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/iosManagedAppProtections", itemID, err)
			} else if assignments != nil {
				item.SetAssignments(assignments.GetValue())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if p, ok := item.(betamodels.ManagedAppPolicyable); ok {
				return safeStringValue(p.GetDisplayName())
			}
			return ""
		},
	}, nil
}
