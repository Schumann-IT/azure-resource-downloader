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

// NewAndroidManagedAppProtectionHandler creates a handler for Intune Android
// app protection (MAM) policies
// (deviceAppManagement/androidManagedAppProtections, Microsoft Graph beta).
//
// Fetch uses $expand=apps so the targeted app list is included (it is not
// returned by a plain GET).
//
// terraform-provider-microsoft365 has no resource for managed app protection
// policies yet, so no Terraform import is emitted.
func NewAndroidManagedAppProtectionHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType:     "Microsoft.Graph/androidManagedAppProtections",
		terraformType: "",
		documentation: docMeta(
			"An Intune Android App Protection (MAM) policy controlling data protection for managed apps.",
			[]string{"dataBackupBlocked", "screenCaptureBlocked", "pinRequired", "allowedOutboundDataTransferDestinations"},
			nil,
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceAppManagement().AndroidManagedAppProtections()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list Android managed app protections: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
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
			requestConfig := &betadeviceappmanagement.AndroidManagedAppProtectionsAndroidManagedAppProtectionItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadeviceappmanagement.AndroidManagedAppProtectionsAndroidManagedAppProtectionItemRequestBuilderGetQueryParameters{
					Expand: []string{"apps"},
				},
			}
			item, err := client.DeviceAppManagement().AndroidManagedAppProtections().ByAndroidManagedAppProtectionId(itemID).Get(ctx, requestConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get Android managed app protection: %w (hint: requires 'DeviceManagementApps.Read.All' permission in Microsoft Graph)", err)
			}
			if assignments, err := client.DeviceAppManagement().AndroidManagedAppProtections().ByAndroidManagedAppProtectionId(itemID).Assignments().Get(ctx, nil); err != nil {
				warnAssignmentsFetchFailed("Microsoft.Graph/androidManagedAppProtections", itemID, err)
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
