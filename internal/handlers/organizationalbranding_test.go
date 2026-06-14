package handlers

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestOrganizationalBrandingHandler_GetType(t *testing.T) {
	handler, err := NewOrganizationalBrandingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewOrganizationalBrandingHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/organizationalBranding"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestOrganizationalBrandingHandler_GetTerraformResourceType(t *testing.T) {
	handler, err := NewOrganizationalBrandingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewOrganizationalBrandingHandler() unexpected error: %v", err)
	}

	if result := handler.GetTerraformResourceType(); result != "" {
		t.Errorf("GetTerraformResourceType() = %q, want empty (no provider resource)", result)
	}
}

func TestOrganizationalBrandingHandler_Transform(t *testing.T) {
	handler, err := NewOrganizationalBrandingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewOrganizationalBrandingHandler() unexpected error: %v", err)
	}

	branding := betamodels.NewOrganizationalBranding()
	id := "0"
	branding.SetId(&id)

	transformed, err := handler.Transform(branding)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != organizationalBrandingFallbackName {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, organizationalBrandingFallbackName)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/organizationalBranding" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/organizationalBranding")
	}
}
