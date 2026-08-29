package graph

import (
	"azure-resource-downloader/internal/models"
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/microsoft/kiota-abstractions-go/serialization"
	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// applePushCertificateFallbackName names the Apple MDM push certificate output
// when the certificate carries no Apple ID.
const applePushCertificateFallbackName = "Apple MDM Push Certificate"

// applePushCertificateStableID returns the certificate's stable identity, used
// both as its display name and as the resource id recorded in metadata. The
// server-assigned id is a GUID regenerated on every read, so the Apple ID is
// used instead (falling back to a constant); this keeps the export reproducible
// across runs.
func applePushCertificateStableID(cert betamodels.ApplePushNotificationCertificateable) string {
	if appleID := safeStringValue(cert.GetAppleIdentifier()); appleID != "" {
		return appleID
	}
	return applePushCertificateFallbackName
}

// NewApplePushNotificationCertificateHandler creates a handler for the Apple
// MDM push certificate (deviceManagement/applePushNotificationCertificate,
// Microsoft Graph beta). This is a tenant **singleton**: List probes the
// object and returns at most one pseudo-ID, and Fetch retrieves the singleton
// regardless of the requested ID. Tenants without an Apple certificate are
// listed as empty instead of failing.
func NewApplePushNotificationCertificateHandler(credential azcore.TokenCredential) (*GraphCollectionHandler, error) {
	client, err := newBetaGraphClient(credential)
	if err != nil {
		return nil, err
	}

	getSingleton := func(ctx context.Context) (betamodels.ApplePushNotificationCertificateable, error) {
		cert, err := client.DeviceManagement().ApplePushNotificationCertificate().Get(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to get Apple MDM push certificate: %w (hint: requires 'DeviceManagementServiceConfig.Read.All' permission in Microsoft Graph)", err)
		}
		return cert, nil
	}

	return &GraphCollectionHandler{
		azureType: "Microsoft.Graph/applePushNotificationCertificate",
		documentation: models.ResourceDocumentation{
			Template:            credentialPromptTemplateText,
			Purpose:             "The Apple Push Notification service (APNs) certificate used by Intune to manage Apple devices.",
			KeySettings:         []string{"expirationDateTime", "appleIdentifier"},
			RequiredPermissions: []string{"DeviceManagementServiceConfig.Read.All"},
			Lifecycle:           []string{"The Apple MDM push certificate expires yearly and must be renewed with the SAME Apple ID; letting it expire or renewing with a different Apple ID forces re-enrollment of all Apple devices."},
			Links: models.ResourceLinks{
				EndpointDocs: "https://learn.microsoft.com/en-us/graph/api/resources/intune-devices-applepushnotificationcertificate?view=graph-rest-beta",
			},
		},
		listIDs: func(ctx context.Context) ([]string, error) {
			cert, err := getSingleton(ctx)
			if err != nil {
				return nil, err
			}
			// Presence is probed via the server id, but the returned id must be
			// STABLE across reads (the server id is a GUID regenerated on every
			// read), so the singleton is identified by its Apple ID.
			if cert == nil || cert.GetId() == nil || *cert.GetId() == "" {
				return nil, nil
			}
			return []string{applePushCertificateStableID(cert)}, nil
		},
		fetchItem: func(ctx context.Context, _ string) (serialization.Parsable, error) {
			return getSingleton(ctx)
		},
		displayName: func(item serialization.Parsable) string {
			if cert, ok := item.(betamodels.ApplePushNotificationCertificateable); ok {
				return applePushCertificateStableID(cert)
			}
			return applePushCertificateFallbackName
		},
		normalize: func(properties map[string]interface{}) {
			// This is a tenant singleton whose id is a server-generated GUID
			// regenerated on every read; dropping it keeps the exported YAML
			// identical across runs (the stable identity is appleIdentifier,
			// which the file is named after).
			delete(properties, "id")
		},
	}, nil
}
