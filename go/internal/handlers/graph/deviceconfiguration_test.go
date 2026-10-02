package graph

import (
	"slices"
	"testing"

	betamodels "github.com/microsoftgraph/msgraph-beta-sdk-go/models"
)

func TestDeviceConfigurationHandler_GetType(t *testing.T) {
	handler, err := NewDeviceConfigurationHandler(fakeTokenCredential{}, false)
	if err != nil {
		t.Fatalf("NewDeviceConfigurationHandler() unexpected error: %v", err)
	}

	expected := "Microsoft.Graph/deviceConfigurations"
	result := handler.GetType()

	if result != expected {
		t.Errorf("GetType() = %q, want %q", result, expected)
	}
}

func TestDeviceConfigurationHandler_RequiredPermissions(t *testing.T) {
	tests := []struct {
		name           string
		resolveSecrets bool
		want           string
	}{
		{name: "read-only", resolveSecrets: false, want: "DeviceManagementConfiguration.Read.All"},
		{name: "resolve secrets needs read-write", resolveSecrets: true, want: "DeviceManagementConfiguration.ReadWrite.All"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewDeviceConfigurationHandler(fakeTokenCredential{}, tt.resolveSecrets)
			if err != nil {
				t.Fatalf("NewDeviceConfigurationHandler() unexpected error: %v", err)
			}
			perms := handler.RequiredPermissions()
			if len(perms) != 1 || perms[0] != tt.want {
				t.Errorf("RequiredPermissions() = %v, want [%s]", perms, tt.want)
			}
			if !handler.RequiresDedicatedApp() {
				t.Error("RequiresDedicatedApp() = false, want true for deviceConfigurations")
			}
		})
	}
}

// TestDeviceConfigurationHandler_DocumentedPermissionsStatic verifies the
// documented permission line and the prompt do not depend on resolve-secrets:
// both scopes are named statically, Read.All first, while the run's own scope
// (RequiredPermissions) still follows the setting.
func TestDeviceConfigurationHandler_DocumentedPermissionsStatic(t *testing.T) {
	off, err := NewDeviceConfigurationHandler(fakeTokenCredential{}, false)
	if err != nil {
		t.Fatalf("NewDeviceConfigurationHandler(false) unexpected error: %v", err)
	}
	on, err := NewDeviceConfigurationHandler(fakeTokenCredential{}, true)
	if err != nil {
		t.Fatalf("NewDeviceConfigurationHandler(true) unexpected error: %v", err)
	}

	offPerms, onPerms := off.Documentation().RequiredPermissions, on.Documentation().RequiredPermissions
	if !slices.Equal(offPerms, onPerms) {
		t.Errorf("Documentation().RequiredPermissions differs: resolveSecrets false %v, true %v", offPerms, onPerms)
	}
	if len(offPerms) != 2 || offPerms[0] != "DeviceManagementConfiguration.Read.All" {
		t.Errorf("Documentation().RequiredPermissions = %v, want Read.All first and the ReadWrite.All line second", offPerms)
	}
	if off.GetDocumentationPrompt() != on.GetDocumentationPrompt() {
		t.Error("GetDocumentationPrompt() differs between resolveSecrets false and true")
	}
}

func TestExtractDeviceConfigurationID(t *testing.T) {
	tests := []struct {
		name       string
		resourceID string
		expected   string
	}{
		{
			name:       "full path format",
			resourceID: "/deviceManagement/deviceConfigurations/abc-123-def",
			expected:   "abc-123-def",
		},
		{
			name:       "direct profile ID",
			resourceID: "abc-123-def",
			expected:   "abc-123-def",
		},
		{
			name:       "UUID format",
			resourceID: "12345678-1234-1234-1234-123456789abc",
			expected:   "12345678-1234-1234-1234-123456789abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGraphItemID(tt.resourceID)
			if result != tt.expected {
				t.Errorf("extractGraphItemID(%q) = %q, want %q", tt.resourceID, result, tt.expected)
			}
		})
	}
}

func TestApplyPlaintextToOmaSetting(t *testing.T) {
	t.Run("string setting", func(t *testing.T) {
		setting := betamodels.NewOmaSettingString()
		applyPlaintextToOmaSetting(setting, "secret-value")
		if got := safeStringValue(setting.GetValue()); got != "secret-value" {
			t.Errorf("OmaSettingString value = %q, want %q", got, "secret-value")
		}
	})

	t.Run("xml setting", func(t *testing.T) {
		setting := betamodels.NewOmaSettingStringXml()
		applyPlaintextToOmaSetting(setting, "<a/>")
		if got := string(setting.GetValue()); got != "<a/>" {
			t.Errorf("OmaSettingStringXml value = %q, want %q", got, "<a/>")
		}
	})
}
