package graph

import (
	"azure-resource-downloader/internal/azure"
	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betadevicemanagement "github.com/microsoftgraph/msgraph-beta-sdk-go/devicemanagement"
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
		documentation: models.ResourceDocumentation{
			Template:            referencedPromptTemplateText,
			Purpose:             "An Intune notification message template used for compliance and other notifications.",
			EmbeddedPayloads:    []string{"localizedNotificationMessages (per-locale subject and message body)"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           []string{"Referenced by compliance policies noncompliance actions; deleting a template breaks those actions.", "Localized messages fall back to the default locale."},
			RelatedTypes:        []string{"Microsoft.Graph/deviceCompliancePolicies (noncompliance actions)"},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-notification-notificationmessagetemplate?view=graph-rest-beta",
			},
		},
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
			// localizedNotificationMessages (the per-locale subject and message
			// body) is a navigation property Graph omits from a plain item GET,
			// so fetch it via $expand and copy it onto the item. Best-effort: a
			// failure here still exports the template, just without its content.
			requestConfig := &betadevicemanagement.NotificationMessageTemplatesNotificationMessageTemplateItemRequestBuilderGetRequestConfiguration{
				QueryParameters: &betadevicemanagement.NotificationMessageTemplatesNotificationMessageTemplateItemRequestBuilderGetQueryParameters{
					Expand: []string{"localizedNotificationMessages"},
				},
			}
			if expanded, err := client.DeviceManagement().NotificationMessageTemplates().ByNotificationMessageTemplateId(itemID).Get(ctx, requestConfig); err != nil {
				logger.Default.Warn("Failed to fetch localized notification messages; exporting template without content",
					"type", "Microsoft.Graph/notificationMessageTemplates",
					"id", itemID,
					"reason", azure.ErrorSummary(err))
				logger.Default.Debug("Localized notification messages fetch failed",
					"type", "Microsoft.Graph/notificationMessageTemplates",
					"id", itemID,
					"error", err)
			} else if expanded != nil {
				item.SetLocalizedNotificationMessages(expanded.GetLocalizedNotificationMessages())
			}
			return item, nil
		},
		displayName: func(item serialization.Parsable) string {
			if t, ok := item.(betamodels.NotificationMessageTemplateable); ok {
				return safeStringValue(t.GetDisplayName())
			}
			return ""
		},
		normalize: func(properties map[string]interface{}) {
			// localizedNotificationMessages[].lastModifiedDateTime is stamped
			// by the Graph API with the response time rather than the tenant's
			// actual modification time, so it changes on every read and makes
			// sourceSha256 churn. Strip it to keep the exported YAML stable.
			msgs, ok := properties["localizedNotificationMessages"].([]interface{})
			if !ok {
				return
			}
			for _, m := range msgs {
				if msg, ok := m.(map[string]interface{}); ok {
					delete(msg, "lastModifiedDateTime")
				}
			}
		},
	}, nil
}
