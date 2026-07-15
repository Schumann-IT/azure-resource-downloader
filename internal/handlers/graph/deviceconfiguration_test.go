package graph

import (
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
