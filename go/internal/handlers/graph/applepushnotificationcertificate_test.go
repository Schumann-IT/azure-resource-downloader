package graph

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestApplePushNotificationCertificateHandler_GetType(t *testing.T) {
	handler, err := NewApplePushNotificationCertificateHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewApplePushNotificationCertificateHandler() unexpected error: %v", err)
	}
	if got := handler.GetType(); got != "Microsoft.Graph/applePushNotificationCertificate" {
		t.Errorf("GetType() = %q, want %q", got, "Microsoft.Graph/applePushNotificationCertificate")
	}
}

// TestApplePushNotificationCertificateHandler_StripsVolatileID verifies that the
// singleton's server-generated id (regenerated on every read) is dropped from
// the exported properties, so the YAML is identical across runs. The stable
// identity is appleIdentifier, which the file is named after.
func TestApplePushNotificationCertificateHandler_StripsVolatileID(t *testing.T) {
	handler, err := NewApplePushNotificationCertificateHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewApplePushNotificationCertificateHandler() unexpected error: %v", err)
	}

	id := "059578fe-2401-407b-b21f-036e719039c2"
	appleID := "admin@cb-gmbh.com"
	cert := betamodels.NewApplePushNotificationCertificate()
	cert.SetId(&id)
	cert.SetAppleIdentifier(&appleID)

	result, err := handler.Transform(cert)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}

	if _, present := result.Properties["id"]; present {
		t.Errorf("Transform() must strip the volatile id from properties, got %v", result.Properties["id"])
	}
	if result.DisplayName != appleID {
		t.Errorf("Transform() DisplayName = %q, want %q", result.DisplayName, appleID)
	}
}

// TestApplePushCertificateStableID verifies the singleton's stable identity
// (recorded as its metadata resourceId) is the Apple ID rather than the
// per-read server GUID, falling back to a constant when the Apple ID is absent.
func TestApplePushCertificateStableID(t *testing.T) {
	appleID := "admin@cb-gmbh.com"
	withID := betamodels.NewApplePushNotificationCertificate()
	withID.SetAppleIdentifier(&appleID)
	if got := applePushCertificateStableID(withID); got != appleID {
		t.Errorf("applePushCertificateStableID() = %q, want %q", got, appleID)
	}

	empty := betamodels.NewApplePushNotificationCertificate()
	if got := applePushCertificateStableID(empty); got != applePushCertificateFallbackName {
		t.Errorf("applePushCertificateStableID() fallback = %q, want %q", got, applePushCertificateFallbackName)
	}
}
