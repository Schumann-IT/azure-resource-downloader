package handlers

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestNdesConnectorHandler_GetType(t *testing.T) {
	handler, err := NewNdesConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewNdesConnectorHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/ndesConnectors"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestNdesConnectorHandler_GetTerraformResourceType(t *testing.T) {
	handler, err := NewNdesConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewNdesConnectorHandler() unexpected error: %v", err)
	}

	if result := handler.GetTerraformResourceType(); result != "" {
		t.Errorf("GetTerraformResourceType() = %q, want empty (no provider resource)", result)
	}
}

func TestNdesConnectorHandler_Transform(t *testing.T) {
	handler, err := NewNdesConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewNdesConnectorHandler() unexpected error: %v", err)
	}

	connector := betamodels.NewNdesConnector()
	id := "55555555-5555-5555-5555-555555555555"
	name := "Corp SCEP Connector"
	connector.SetId(&id)
	connector.SetDisplayName(&name)

	transformed, err := handler.Transform(connector)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != name {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, name)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/ndesConnectors" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/ndesConnectors")
	}
}

// NDES connectors without a friendly name fall back to the item ID for naming.
func TestNdesConnectorHandler_Transform_IDFallback(t *testing.T) {
	handler, err := NewNdesConnectorHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewNdesConnectorHandler() unexpected error: %v", err)
	}

	connector := betamodels.NewNdesConnector()
	id := "66666666-6666-6666-6666-666666666666"
	connector.SetId(&id)

	transformed, err := handler.Transform(connector)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != id {
		t.Errorf("Transform() Name = %q, want %q (ID fallback)", transformed.Name, id)
	}
}
