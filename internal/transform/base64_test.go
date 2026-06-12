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

func TestApplyBase64DecodeScriptContent(t *testing.T) {
	script := "#!/bin/sh\necho hello"
	encoded := base64.StdEncoding.EncodeToString([]byte(script))

	t.Run("inline decodes scriptContent in place", func(t *testing.T) {
		props := map[string]interface{}{
			"fileName":      "hello.sh",
			"scriptContent": encoded,
		}
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 0 {
			t.Fatalf("expected no artifacts in inline mode, got %d", len(artifacts))
		}
		if props["scriptContent"] != script {
			t.Errorf("scriptContent = %q, want decoded %q", props["scriptContent"], script)
		}
	})

	t.Run("inline strips BOM, normalizes CRLF and trims trailing whitespace", func(t *testing.T) {
		windowsScript := "\uFEFF$app = Get-WmiObject -Class Win32_Product  \r\n$app.Uninstall()\t\r"
		props := map[string]interface{}{
			"fileName":      "uninstall.ps1",
			"scriptContent": base64.StdEncoding.EncodeToString([]byte(windowsScript)),
		}
		_, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "$app = Get-WmiObject -Class Win32_Product\n$app.Uninstall()\n"
		if props["scriptContent"] != want {
			t.Errorf("scriptContent = %q, want normalized %q", props["scriptContent"], want)
		}
	})

	t.Run("file mode keeps BOM and CRLF byte-exact", func(t *testing.T) {
		windowsScript := "\uFEFFWrite-Host 'hi'\r\n"
		props := map[string]interface{}{
			"fileName":      "hi.ps1",
			"scriptContent": base64.StdEncoding.EncodeToString([]byte(windowsScript)),
		}
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{"mode": "file"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if string(artifacts[0].Content) != windowsScript {
			t.Errorf("Content = %q, want byte-exact %q", string(artifacts[0].Content), windowsScript)
		}
	})

	t.Run("file mode uses fileName for scriptContent", func(t *testing.T) {
		props := map[string]interface{}{
			"fileName":      "hello.sh",
			"scriptContent": encoded,
		}
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{"mode": "file", "remove-source": true}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 1 {
			t.Fatalf("expected 1 artifact, got %d", len(artifacts))
		}
		if artifacts[0].Filename != "hello.sh" {
			t.Errorf("Filename = %q, want hello.sh", artifacts[0].Filename)
		}
		if string(artifacts[0].Content) != script {
			t.Errorf("Content = %q, want %q", string(artifacts[0].Content), script)
		}
		if _, present := props["scriptContent"]; present {
			t.Error("scriptContent should be removed with remove-source")
		}
	})

	t.Run("health script detection and remediation pair", func(t *testing.T) {
		props := map[string]interface{}{
			"displayName":              "Fix Time Zone",
			"detectionScriptContent":   encoded,
			"remediationScriptContent": encoded,
		}
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{"mode": "file"}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 2 {
			t.Fatalf("expected 2 artifacts, got %d", len(artifacts))
		}
		want := map[string]bool{
			"fix_time_zone_detection.ps1":   false,
			"fix_time_zone_remediation.ps1": false,
		}
		for _, a := range artifacts {
			if _, ok := want[a.Filename]; !ok {
				t.Errorf("unexpected artifact filename %q", a.Filename)
			}
			want[a.Filename] = true
		}
		for name, seen := range want {
			if !seen {
				t.Errorf("missing artifact %q", name)
			}
		}
	})

	t.Run("inline decodes detection and remediation in place", func(t *testing.T) {
		props := map[string]interface{}{
			"displayName":              "Fix Time Zone",
			"detectionScriptContent":   encoded,
			"remediationScriptContent": encoded,
		}
		artifacts, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(artifacts) != 0 {
			t.Fatalf("expected no artifacts in inline mode, got %d", len(artifacts))
		}
		if props["detectionScriptContent"] != script || props["remediationScriptContent"] != script {
			t.Error("detection/remediation script content should be decoded in place")
		}
	})

	t.Run("invalid base64 returns error", func(t *testing.T) {
		props := map[string]interface{}{"scriptContent": "%%%not-base64%%%"}
		if _, err := ApplyBase64Decode(props, models.ParseBase64DecodeConfig(map[string]interface{}{})); err == nil {
			t.Error("expected error for invalid base64 scriptContent")
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
