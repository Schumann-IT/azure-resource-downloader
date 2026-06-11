package transform

import (
	"encoding/base64"
	"testing"

	"azure-resource-downloader/internal/models"
)

func fileModeConfig() *models.Base64DecodeConfig {
	return models.ParseBase64DecodeConfig(map[string]interface{}{"mode": "file"})
}

func TestApplyBase64DecodeInline(t *testing.T) {
	plist := "<?xml version=\"1.0\"?>\n<plist></plist>"
	encoded := base64.StdEncoding.EncodeToString([]byte(plist))

	t.Run("default mode replaces payload in place", func(t *testing.T) {
		properties := map[string]interface{}{
			"payload":         encoded,
			"payloadFileName": "WindowsDefenderATPOnboarding.xml",
		}
		cfg := models.ParseBase64DecodeConfig(map[string]interface{}{})
		if cfg.Mode != models.Base64ModeInline {
			t.Fatalf("default mode = %q, want %q", cfg.Mode, models.Base64ModeInline)
		}

		artifacts, err := ApplyBase64Decode(properties, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 0 {
			t.Fatalf("expected no artifact in inline mode, got %+v", artifacts)
		}
		if properties["payload"] != plist {
			t.Errorf("payload = %q, want decoded %q", properties["payload"], plist)
		}
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		properties := map[string]interface{}{"payload": "not-base64!!!"}
		if _, err := ApplyBase64Decode(properties, models.ParseBase64DecodeConfig(map[string]interface{}{})); err == nil {
			t.Fatalf("expected error, got nil")
		}
	})

	t.Run("missing payload is a no-op", func(t *testing.T) {
		properties := map[string]interface{}{"other": "x"}
		artifacts, err := ApplyBase64Decode(properties, models.ParseBase64DecodeConfig(map[string]interface{}{}))
		if err != nil || len(artifacts) != 0 {
			t.Fatalf("expected no-op, got artifacts=%v err=%v", artifacts, err)
		}
	})
}

func TestApplyBase64DecodeFileMode(t *testing.T) {
	plist := "<?xml version=\"1.0\"?><plist></plist>"
	encoded := base64.StdEncoding.EncodeToString([]byte(plist))

	tests := []struct {
		name         string
		properties   map[string]interface{}
		wantNil      bool
		wantFilename string
		wantContent  string
		wantErr      bool
	}{
		{
			name: "decodes payload and replaces .xml extension",
			properties: map[string]interface{}{
				"payload":         encoded,
				"payloadFileName": "WindowsDefenderATPOnboarding.xml",
			},
			wantFilename: "WindowsDefenderATPOnboarding.mobileconfig",
			wantContent:  plist,
		},
		{
			name: "keeps name when already .mobileconfig",
			properties: map[string]interface{}{
				"payload":         encoded,
				"payloadFileName": "FortiClient_Configuration_Profile.Intune.mobileconfig",
			},
			wantFilename: "FortiClient_Configuration_Profile.Intune.mobileconfig",
			wantContent:  plist,
		},
		{
			name: "missing payload returns nil",
			properties: map[string]interface{}{
				"payloadFileName": "x.xml",
			},
			wantNil: true,
		},
		{
			name: "missing filename returns nil",
			properties: map[string]interface{}{
				"payload": encoded,
			},
			wantNil: true,
		},
		{
			name: "invalid base64 returns error",
			properties: map[string]interface{}{
				"payload":         "not-base64!!!",
				"payloadFileName": "x.xml",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifacts, err := ApplyBase64Decode(tt.properties, fileModeConfig())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantNil {
				if len(artifacts) != 0 {
					t.Fatalf("expected no artifact, got %+v", artifacts)
				}
				return
			}

			if len(artifacts) != 1 {
				t.Fatalf("expected 1 artifact, got %d", len(artifacts))
			}
			if artifacts[0].Filename != tt.wantFilename {
				t.Errorf("Filename = %q, want %q", artifacts[0].Filename, tt.wantFilename)
			}
			if string(artifacts[0].Content) != tt.wantContent {
				t.Errorf("Content = %q, want %q", string(artifacts[0].Content), tt.wantContent)
			}
		})
	}
}

func TestApplyBase64DecodeFileModeRemoveSource(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	properties := map[string]interface{}{
		"payload":         encoded,
		"payloadFileName": "x.xml",
	}
	cfg := models.ParseBase64DecodeConfig(map[string]interface{}{"mode": "file", "remove-source": true})

	if _, err := ApplyBase64Decode(properties, cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := properties["payload"]; exists {
		t.Errorf("expected payload to be removed from properties")
	}
}

func TestApplyBase64DecodeOmaSettings(t *testing.T) {
	xml := "<a/>"
	encoded := base64.StdEncoding.EncodeToString([]byte(xml))

	newProps := func() map[string]interface{} {
		return map[string]interface{}{
			"omaSettings": []interface{}{
				map[string]interface{}{
					"@odata.type": "#microsoft.graph.omaSettingStringXml",
					"fileName":    "CB_VPN_Profile.xml",
					"value":       encoded,
				},
				map[string]interface{}{
					"@odata.type": "#microsoft.graph.omaSettingString",
					"value":       "****",
				},
			},
		}
	}

	t.Run("inline replaces only omaSettingStringXml value", func(t *testing.T) {
		props := newProps()
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 0 {
			t.Fatalf("expected no artifacts in inline mode, got %d", len(artifacts))
		}

		settings := props["omaSettings"].([]interface{})
		xmlSetting := settings[0].(map[string]interface{})
		if xmlSetting["value"] != xml {
			t.Errorf("xml value = %q, want decoded %q", xmlSetting["value"], xml)
		}
		strSetting := settings[1].(map[string]interface{})
		if strSetting["value"] != "****" {
			t.Errorf("omaSettingString value = %q, want untouched ****", strSetting["value"])
		}
	})

	t.Run("file mode emits artifact named after fileName", func(t *testing.T) {
		props := newProps()
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{"mode": "file"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if artifacts[0].Filename != "CB_VPN_Profile.xml" {
			t.Errorf("Filename = %q, want CB_VPN_Profile.xml", artifacts[0].Filename)
		}
		if string(artifacts[0].Content) != xml {
			t.Errorf("Content = %q, want %q", string(artifacts[0].Content), xml)
		}
	})
}

func TestBuildArtifactFileName(t *testing.T) {
	tests := []struct {
		fileName  string
		extension string
		expected  string
	}{
		{"WindowsDefenderATPOnboarding.xml", ".mobileconfig", "WindowsDefenderATPOnboarding.mobileconfig"},
		{"profile.Intune.mobileconfig", ".mobileconfig", "profile.Intune.mobileconfig"},
		{"noext", ".mobileconfig", "noext.mobileconfig"},
		{"file.xml", "mobileconfig", "file.mobileconfig"},
		{"file.xml", "", "file"},
	}

	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			result := buildArtifactFileName(tt.fileName, tt.extension)
			if result != tt.expected {
				t.Errorf("buildArtifactFileName(%q, %q) = %q, want %q", tt.fileName, tt.extension, result, tt.expected)
			}
		})
	}
}
