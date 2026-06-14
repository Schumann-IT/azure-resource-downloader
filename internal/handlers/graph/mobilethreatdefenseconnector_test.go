package graph

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestMobileThreatDefenseConnectorHandler_GetType(t *testing.T) {
	handler, err := NewMobileThreatDefenseConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewMobileThreatDefenseConnectorHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/mobileThreatDefenseConnectors"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestMobileThreatDefenseConnectorHandler_GetTerraformResourceType(t *testing.T) {
	handler, err := NewMobileThreatDefenseConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewMobileThreatDefenseConnectorHandler() unexpected error: %v", err)
	}

	if result := handler.GetTerraformResourceType(); result != "" {
		t.Errorf("GetTerraformResourceType() = %q, want empty (no provider resource)", result)
	}
}

// MTD connectors carry no display name, so the item ID (the partner identifier)
// is used as the resource name.
func TestMobileThreatDefenseConnectorHandler_Transform_UsesID(t *testing.T) {
	handler, err := NewMobileThreatDefenseConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewMobileThreatDefenseConnectorHandler() unexpected error: %v", err)
	}

	connector := betamodels.NewMobileThreatDefenseConnector()
	id := "44444444-4444-4444-4444-444444444444"
	connector.SetId(&id)

	transformed, err := handler.Transform(connector)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != id {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, id)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/mobileThreatDefenseConnectors" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/mobileThreatDefenseConnectors")
	}
}
