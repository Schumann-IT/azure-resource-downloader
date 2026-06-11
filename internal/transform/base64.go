package transform

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"azure-resource-downloader/internal/models"
)

// omaSettingStringXmlType is the @odata.type of Intune Windows OMA-URI settings
// whose value is a base64-encoded XML byte array.
const omaSettingStringXmlType = "#microsoft.graph.omaSettingStringXml"

// scriptContentKeys are the top-level properties of Intune script resources
// (deviceManagementScripts, deviceShellScripts, deviceCustomAttributeShellScripts,
// deviceHealthScripts) that hold base64-encoded script bodies. The Graph models
// declare them as byte arrays, so generic serialization renders them as base64
// strings. The suffix is used to build sidecar file names for resources that
// carry more than one script (detection + remediation).
var scriptContentKeys = []struct {
	key    string
	suffix string
}{
	{key: "scriptContent", suffix: ""},
	{key: "detectionScriptContent", suffix: "_detection"},
	{key: "remediationScriptContent", suffix: "_remediation"},
}

// ApplyBase64Decode decodes base64-encoded property values according to cfg.Mode
// and returns any sidecar file artifacts produced.
//
// It handles two locations:
//   - the top-level cfg.SourceKey (e.g. macOSCustomConfiguration "payload"),
//   - nested Windows OMA-URI settings in "omaSettings" whose @odata.type is
//     omaSettingStringXml (their "value" is base64-encoded XML), and
//   - Intune script bodies (scriptContent / detectionScriptContent /
//     remediationScriptContent) of script resources.
//
// In "inline" mode (default) each encoded value is replaced in place with the
// decoded text and no artifact is returned. In "file" mode the decoded value is
// returned as a sidecar file artifact instead: the top-level payload uses
// cfg.FilenameKey with its extension replaced by cfg.Extension, while OMA settings
// use their own "fileName" as-is. When cfg.RemoveSource is true the encoded value
// is removed from the YAML output in file mode.
func ApplyBase64Decode(properties map[string]interface{}, cfg *models.Base64DecodeConfig) ([]models.FileArtifact, error) {
	if properties == nil || cfg == nil {
		return nil, nil
	}

	var artifacts []models.FileArtifact

	payloadArtifact, err := decodePayload(properties, cfg)
	if err != nil {
		return nil, err
	}
	if payloadArtifact != nil {
		artifacts = append(artifacts, *payloadArtifact)
	}

	omaArtifacts, err := decodeOmaSettings(properties, cfg)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, omaArtifacts...)

	scriptArtifacts, err := decodeScriptContent(properties, cfg)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, scriptArtifacts...)

	return artifacts, nil
}

// decodePayload decodes the top-level cfg.SourceKey value. Returns (nil, nil) when
// the source key is missing/empty or, in file mode, the filename key is missing.
func decodePayload(properties map[string]interface{}, cfg *models.Base64DecodeConfig) (*models.FileArtifact, error) {
	encoded, ok := properties[cfg.SourceKey].(string)
	if !ok || encoded == "" {
		return nil, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("failed to base64-decode %q: %w", cfg.SourceKey, err)
	}

	if cfg.Mode == models.Base64ModeFile {
		fileName, ok := properties[cfg.FilenameKey].(string)
		if !ok || fileName == "" {
			return nil, nil
		}
		if cfg.RemoveSource {
			delete(properties, cfg.SourceKey)
		}
		return &models.FileArtifact{
			Filename: buildArtifactFileName(fileName, cfg.Extension),
			Content:  decoded,
		}, nil
	}

	// Inline mode: replace the encoded value with the decoded text.
	properties[cfg.SourceKey] = string(decoded)
	return nil, nil
}

// decodeOmaSettings decodes base64 omaSettingStringXml values within the
// "omaSettings" list. In file mode each decoded value is written to a sidecar file
// named after the setting's "fileName"; in inline mode the value is replaced in
// place. Non-XML settings (plain strings, secrets) are left untouched.
func decodeOmaSettings(properties map[string]interface{}, cfg *models.Base64DecodeConfig) ([]models.FileArtifact, error) {
	rawSettings, ok := properties["omaSettings"].([]interface{})
	if !ok {
		return nil, nil
	}

	var artifacts []models.FileArtifact
	for _, raw := range rawSettings {
		setting, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if odataType, _ := setting["@odata.type"].(string); odataType != omaSettingStringXmlType {
			continue
		}
		encoded, ok := setting["value"].(string)
		if !ok || encoded == "" {
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to base64-decode omaSettings value: %w", err)
		}

		if cfg.Mode == models.Base64ModeFile {
			fileName, _ := setting["fileName"].(string)
			if fileName == "" {
				continue
			}
			if cfg.RemoveSource {
				delete(setting, "value")
			}
			artifacts = append(artifacts, models.FileArtifact{
				Filename: fileName,
				Content:  decoded,
			})
			continue
		}

		setting["value"] = string(decoded)
	}

	return artifacts, nil
}

// decodeScriptContent decodes the base64 script bodies of Intune script
// resources. In inline mode each encoded value is replaced in place with the
// decoded text. In file mode the decoded script becomes a sidecar artifact:
// resources with a "fileName" property use it as-is (it already carries the
// proper .ps1/.sh extension); otherwise the file name is derived from the
// sanitized "displayName" plus the key's suffix and a .ps1 extension
// (deviceHealthScripts are Windows-only PowerShell).
func decodeScriptContent(properties map[string]interface{}, cfg *models.Base64DecodeConfig) ([]models.FileArtifact, error) {
	var artifacts []models.FileArtifact

	for _, sc := range scriptContentKeys {
		encoded, ok := properties[sc.key].(string)
		if !ok || encoded == "" {
			continue
		}

		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to base64-decode %q: %w", sc.key, err)
		}

		if cfg.Mode == models.Base64ModeFile {
			fileName := scriptArtifactFileName(properties, sc.suffix)
			if fileName == "" {
				continue
			}
			if cfg.RemoveSource {
				delete(properties, sc.key)
			}
			artifacts = append(artifacts, models.FileArtifact{
				Filename: fileName,
				Content:  decoded,
			})
			continue
		}

		properties[sc.key] = string(decoded)
	}

	return artifacts, nil
}

// scriptArtifactFileName resolves the sidecar file name for a decoded script:
// the resource's own "fileName" when present (single-script resources),
// otherwise the sanitized display name plus suffix with a .ps1 extension.
func scriptArtifactFileName(properties map[string]interface{}, suffix string) string {
	if fileName, ok := properties["fileName"].(string); ok && fileName != "" {
		if suffix == "" {
			return fileName
		}
		return buildArtifactFileName(fileName, "") + suffix + filepath.Ext(fileName)
	}
	displayName, _ := properties["displayName"].(string)
	if displayName == "" {
		return ""
	}
	return SanitizeFileName(displayName) + suffix + ".ps1"
}

// buildArtifactFileName replaces the existing extension of fileName with the
// configured extension. A leading dot on extension is optional.
func buildArtifactFileName(fileName, extension string) string {
	base := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if extension == "" {
		return base
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	return base + extension
}
