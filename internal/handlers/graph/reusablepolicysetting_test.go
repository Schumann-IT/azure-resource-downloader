package graph

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestReusablePolicySettingHandler_GetType(t *testing.T) {
	handler, err := NewReusablePolicySettingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewReusablePolicySettingHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/reusablePolicySettings"
	if result := handler.GetType(); result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestReusablePolicySettingHandler_GetTerraformResourceType(t *testing.T) {
	handler, err := NewReusablePolicySettingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewReusablePolicySettingHandler() unexpected error: %v", err)
	}

	expected := "microsoft365_graph_beta_device_management_reuseable_policy_setting"
	if result := handler.GetTerraformResourceType(); result != expected {
		t.Errorf("GetTerraformResourceType() = %q, want %q", result, expected)
	}
}

func TestReusablePolicySettingHandler_Transform(t *testing.T) {
	handler, err := NewReusablePolicySettingHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewReusablePolicySettingHandler() unexpected error: %v", err)
	}

	setting := betamodels.NewDeviceManagementReusablePolicySetting()
	id := "22222222-2222-2222-2222-222222222222"
	name := "Reusable Firewall Rules"
	setting.SetId(&id)
	setting.SetDisplayName(&name)

	transformed, err := handler.Transform(setting)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if transformed.Name != name {
		t.Errorf("Transform() Name = %q, want %q", transformed.Name, name)
	}
	if transformed.ID != id {
		t.Errorf("Transform() ID = %q, want %q", transformed.ID, id)
	}
	if transformed.Type != "Microsoft.Graph/reusablePolicySettings" {
		t.Errorf("Transform() Type = %q, want %q", transformed.Type, "Microsoft.Graph/reusablePolicySettings")
	}
}
