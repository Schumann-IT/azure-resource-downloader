package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// NewNotificationMessageTemplateHandler creates a handler for Intune
// notification message templates
// (deviceManagement/notificationMessageTemplates, Microsoft Graph beta).
func NewNotificationMessageTemplateHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/notificationMessageTemplates",
		documentation: docMeta(
			"An Intune notification message template used for compliance and other notifications.",
			nil,
			[]string{"localizedNotificationMessages (per-locale subject and message body)"},
			models.ResourceLinks{},
		),
		listIDs: func(ctx context.Context) ([]string, error) {
			var ids []string
			builder := client.DeviceManagement().NotificationMessageTemplates()
			for {
				resp, err := builder.Get(ctx, nil)
				if err != nil {
					return nil, fmt.Errorf("failed to list notification message templates: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
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
			item, err := client.DeviceManagement().NotificationMessageTemplates().ByNotificationMessageTemplateId(itemID).Get(ctx, nil)
			if err != nil {
				return nil, fmt.Errorf("failed to get notification message template: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if t, ok := item.(betamodels.NotificationMessageTemplateable); ok {
				return safeStringValue(t.GetDisplayName())
			}
			return ""
		},
	}, nil
}
