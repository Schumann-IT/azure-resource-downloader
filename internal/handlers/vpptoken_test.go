package handlers

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestVppTokenHandler_GetType(t *testing.T) {
	handler, err := NewVppTokenHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewVppTokenHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/vppTokens"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestVppTokenHandler_GetTerraformResourceType(t *testing.T) {
	handler, err := NewVppTokenHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewVppTokenHandler() unexpected error: %v", err)
	}

	if result := handler.GetTerraformResourceType(); result != "" {
		t.Errorf("GetTerraformResourceType() = %q, want empty (no provider resource)", result)
	}
}

// VPP tokens have no displayName in many tenants; the handler falls back to the
// organization name (then the Apple ID) so the export still gets a stable name.
func TestVppTokenHandler_Transform_OrganizationNameFallback(t *testing.T) {
	handler, err := NewVppTokenHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewVppTokenHandler() unexpected error: %v", err)
	}

	token := betamodels.NewVppToken()
	id := "33333333-3333-3333-3333-333333333333"
	org := "Contoso Ltd"
	token.SetId(&id)
	token.SetOrganizationName(&org)

	transformed, err := handler.Transform(token)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != org {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, org)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/vppTokens" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/vppTokens")
	}
}
