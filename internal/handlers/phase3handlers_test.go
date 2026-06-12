package handlers

import (
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

// TestPhase3HandlerTypes verifies type metadata of the $expand / child-fetch
// based handlers (compliance, administrative templates, intents).
func TestPhase3HandlerTypes(t *testing.T) {
	tests := []struct {
		name          string
		azureType     string
		terraformType string
	}{
		{
			name:          "NewDeviceCompliancePolicyHandler",
			azureType:     "Microsoft.Graph/deviceCompliancePolicies",
			terraformType: "microsoft365_graph_beta_device_management_windows_device_compliance_policy",
		},
		{
			name:          "NewCompliancePolicyHandler",
			azureType:     "Microsoft.Graph/compliancePolicies",
			terraformType: "microsoft365_graph_beta_device_management_linux_device_compliance_policy",
		},
		{
			name:          "NewGroupPolicyConfigurationHandler",
			azureType:     "Microsoft.Graph/groupPolicyConfigurations",
			terraformType: "microsoft365_graph_beta_device_management_group_policy_configuration",
		},
		{
			name:          "NewDeviceManagementIntentHandler",
			azureType:     "Microsoft.Graph/deviceManagementIntents",
			terraformType: "",
		},
	}

	constructors := map[string]func() (*GraphCollectionHandler, error){
		"NewDeviceCompliancePolicyHandler": func() (*GraphCollectionHandler, error) {
			return NewDeviceCompliancePolicyHandler(fakeTokenCredential{})
		},
		"NewCompliancePolicyHandler": func() (*GraphCollectionHandler, error) {
			return NewCompliancePolicyHandler(fakeTokenCredential{})
		},
		"NewGroupPolicyConfigurationHandler": func() (*GraphCollectionHandler, error) {
			return NewGroupPolicyConfigurationHandler(fakeTokenCredential{})
		},
		"NewDeviceManagementIntentHandler": func() (*GraphCollectionHandler, error) {
			return NewDeviceManagementIntentHandler(fakeTokenCredential{})
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := constructors[tt.name]()
			if err != nil {
				t.Fatalf("%s() unexpected error: %v", tt.name, err)
			}
			if got := handler.GetType(); got != tt.azureType {
				t.Errorf("GetType() = %q, want %q", got, tt.azureType)
			}
			if got := handler.GetTerraformResourceType(); got != tt.terraformType {
				t.Errorf("GetTerraformResourceType() = %q, want %q", got, tt.terraformType)
			}
		})
	}
}

// TestCompliancePolicyHandler_Transform verifies that Settings Catalog based
// compliance policies are named via their `name` field.
func TestCompliancePolicyHandler_Transform(t *testing.T) {
	handler, err := NewCompliancePolicyHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewCompliancePolicyHandler() unexpected error: %v", err)
	}

	id := "policy-1"
	name := "Linux Compliance Baseline"
	policy := betamodels.NewDeviceManagementCompliancePolicy()
	policy.SetId(&id)
	policy.SetName(&name)

	result, err := handler.Transform(policy)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if result.DisplayName != name {
		t.Errorf("Transform() DisplayName = %q, want %q", result.DisplayName, name)
	}
	if result.ID != id {
		t.Errorf("Transform() ID = %q, want %q", result.ID, id)
	}
}

// TestDeviceManagementIntentHandler_Transform verifies that attached child
// settings survive generic serialization.
func TestDeviceManagementIntentHandler_Transform(t *testing.T) {
	handler, err := NewDeviceManagementIntentHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("NewDeviceManagementIntentHandler() unexpected error: %v", err)
	}

	id := "intent-1"
	name := "Disk Encryption"
	definitionID := "deviceConfiguration--windows10EndpointProtectionConfiguration_bitLockerEnabled"
	intent := betamodels.NewDeviceManagementIntent()
	intent.SetId(&id)
	intent.SetDisplayName(&name)
	setting := betamodels.NewDeviceManagementBooleanSettingInstance()
	setting.SetDefinitionId(&definitionID)
	intent.SetSettings([]betamodels.DeviceManagementSettingInstanceable{setting})

	result, err := handler.Transform(intent)
	if err != nil {
		t.Fatalf("Transform() unexpected error: %v", err)
	}
	if result.DisplayName != name {
		t.Errorf("Transform() DisplayName = %q, want %q", result.DisplayName, name)
	}
	settings, ok := result.Properties["settings"].([]interface{})
	if !ok || len(settings) != 1 {
		t.Fatalf("Properties[settings] = %v, want 1 serialized setting", result.Properties["settings"])
	}
	settingMap, ok := settings[0].(map[string]interface{})
	if !ok || settingMap["definitionId"] != definitionID {
		t.Errorf("settings[0] = %v, want definitionId %q", settings[0], definitionID)
	}
}
