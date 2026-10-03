package consistency

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"azure-resource-downloader/internal/docs"
)

// Synthetic payload identifiers and markers; none comes from a real tenant.
const (
	rootIDA      = "com.example.profile.alpha"
	rootIDB      = "com.example.profile.beta"
	innerID      = "com.example.profile.inner"
	payloadMark  = "synthetic-payload-marker-7c1e"
	bundleIDTest = "com.example.app"
)

// plist builds a synthetic configuration profile whose root dict carries
// PayloadContent (with an inner PayloadIdentifier) before rootID; an empty
// rootID omits the root PayloadIdentifier.
func plist(rootID, marker string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	b.WriteString("  <key>PayloadContent</key>\n  <array>\n    <dict>\n")
	b.WriteString("      <key>PayloadIdentifier</key>\n      <string>" + innerID + "</string>\n")
	b.WriteString("      <key>Marker</key>\n      <string>" + marker + "</string>\n")
	b.WriteString("    </dict>\n  </array>\n")
	b.WriteString("  <key>PayloadDisplayName</key>\n  <string>Synthetic</string>\n")
	if rootID != "" {
		b.WriteString("  <key>PayloadIdentifier</key>\n  <string> " + rootID + " </string>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func customProfile(odataType, payload string) map[string]interface{} {
	return map[string]interface{}{
		"@odata.type":       odataType,
		"displayName":       "custom",
		"payloadName":       "Synthetic name",
		"payloadFileName":   "synthetic.mobileconfig",
		"deploymentChannel": "deviceChannel",
		"supportsScopeTags": true,
		"payload":           payload,
	}
}

func appConfig(bundleID, xmlText string) map[string]interface{} {
	return map[string]interface{}{
		"@odata.type":       odataMacOSCustomApp,
		"displayName":       "app",
		"bundleId":          bundleID,
		"fileName":          "app.xml",
		"supportsScopeTags": true,
		"configurationXml":  xmlText,
	}
}

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestIndexAppleCustomProfileKeys(t *testing.T) {
	text := plist(rootIDA, payloadMark)
	normalised := strings.TrimRight(text, "\n")
	for _, odataType := range []string{odataMacOSCustom, odataIOSCustom} {
		got := indexOf(t, dcType+"m.yaml", typeDeviceConfigurations, customProfile(odataType, text))
		key := odataType + "#payload:" + rootIDA
		hasLen(t, got, 1, "only the payload identifier is a key", odataType, got)
		equal(t, hashOf(normalised), got[key].Value, odataType)
		equal(t, ClassConfiguration, got[key].Class)
		absent(t, got, odataType+"#payload:"+innerID, "an inner PayloadContent identifier is not the key")
		for _, prop := range []string{"payload", "payloadName", "payloadFileName", "deploymentChannel", "supportsScopeTags", "displayName"} {
			absent(t, got, odataType+"#"+prop)
		}
	}

	app := indexOf(t, dcType+"a.yaml", typeDeviceConfigurations, appConfig(" "+bundleIDTest+" ", "<dict><key>k</key><string>v</string></dict>"))
	hasLen(t, app, 1, app)
	equal(t, hashOf("<dict><key>k</key><string>v</string></dict>"), app[odataMacOSCustomApp+"#configurationXml:"+bundleIDTest].Value)
	for _, prop := range []string{"bundleId", "fileName", "configurationXml", "supportsScopeTags"} {
		absent(t, app, odataMacOSCustomApp+"#"+prop)
	}
}

func TestIndexAppleCustomProfileBase64AndSidecar(t *testing.T) {
	text := plist(rootIDA, payloadMark)
	inline := indexOf(t, dcType+"m.yaml", typeDeviceConfigurations, customProfile(odataMacOSCustom, text))
	encoded := indexOf(t, dcType+"m.yaml", typeDeviceConfigurations,
		customProfile(odataMacOSCustom, base64.StdEncoding.EncodeToString([]byte(text))))
	key := odataMacOSCustom + "#payload:" + rootIDA
	equal(t, inline[key].Value, encoded[key].Value, "a still-base64 inline payload indexes like its decoded form")

	// A byte-exact sidecar with a BOM, CRLF and trailing blanks hashes like the inline copy.
	typeDir := t.TempDir()
	sidecar := "\uFEFF" + strings.ReplaceAll(strings.ReplaceAll(text, "\n", "  \r\n"), "<dict>", "<dict>\t")
	if err := os.WriteFile(filepath.Join(typeDir, "synthetic.mobileconfig"), []byte(sidecar), 0644); err != nil {
		t.Fatal(err)
	}
	doc := customProfile(odataMacOSCustom, base64.StdEncoding.EncodeToString([]byte("not used")))
	settings := indexResource(dcType+"m.yaml", typeDeviceConfigurations, doc, []string{"synthetic.mobileconfig"}, typeDir)
	if len(settings) != 1 || settings[0].Value != inline[key].Value {
		t.Errorf("the sidecar copy must hash like the inline copy: %+v", settings)
	}

	// An unreadable sidecar indexes nothing.
	settings = indexResource(dcType+"m.yaml", typeDeviceConfigurations, doc, []string{"missing.mobileconfig"}, typeDir)
	if len(settings) != 0 {
		t.Errorf("an unreadable sidecar indexes nothing: %+v", settings)
	}
}

func TestIndexAppleCustomProfileIndexesNothing(t *testing.T) {
	signed := append([]byte{0x30, 0x82, 0x1f, 0x00, 0x06, 0x09}, []byte(plist(rootIDA, payloadMark))...)
	binary := append([]byte("bplist00"), 0xd1, 0x01, 0x02)
	tests := map[string]map[string]interface{}{
		"signed CMS profile":      customProfile(odataMacOSCustom, base64.StdEncoding.EncodeToString(signed)),
		"binary plist":            customProfile(odataMacOSCustom, base64.StdEncoding.EncodeToString(binary)),
		"no root identifier":      customProfile(odataIOSCustom, plist("", payloadMark)),
		"not base64, not markup":  customProfile(odataMacOSCustom, "%%% not base64 %%%"),
		"no payload":              customProfile(odataMacOSCustom, ""),
		"not a plist root":        customProfile(odataMacOSCustom, "<dict><key>PayloadIdentifier</key><string>x</string></dict>"),
		"app without bundleId":    appConfig("  ", "<dict/>"),
		"app without payload xml": appConfig(bundleIDTest, ""),
	}
	for name, doc := range tests {
		t.Run(name, func(t *testing.T) {
			hasLen(t, indexOf(t, dcType+"x.yaml", typeDeviceConfigurations, doc), 0)
		})
	}
}

// TestAnalyzeAppleCustomProfiles runs the whole analysis over Apple custom
// profiles on one scope.
func TestAnalyzeAppleCustomProfiles(t *testing.T) {
	f := newFixture(t)
	mac := func(odataType string, artifacts ...string) docs.ResourceMeta {
		return docs.ResourceMeta{Platforms: "macOS", ODataType: odataType, Artifacts: artifacts, AssignmentTargets: targets(allDevices())}
	}
	ios := func() docs.ResourceMeta {
		return docs.ResourceMeta{Platforms: "iOS", ODataType: odataIOSCustom, AssignmentTargets: targets(allDevices())}
	}
	text := plist(rootIDA, payloadMark)

	// Same identifier, different bytes: a conflict.
	f.add(dcType+"mac_a1.yaml", mac(odataMacOSCustom), customProfile(odataMacOSCustom, text))
	f.add(dcType+"mac_a2.yaml", mac(odataMacOSCustom), customProfile(odataMacOSCustom, plist(rootIDA, payloadMark+"-other")))
	// A different identifier: no finding against either.
	f.add(dcType+"mac_b.yaml", mac(odataMacOSCustom), customProfile(odataMacOSCustom, plist(rootIDB, payloadMark+"-b")))
	// The same payload as a CRLF sidecar (file mode, no remove-source): a duplicate of mac_a1.
	f.add(dcType+"mac_a3.yaml", mac(odataMacOSCustom, "mac_a3.mobileconfig"),
		customProfile(odataMacOSCustom, base64.StdEncoding.EncodeToString([]byte(text))))
	f.writeRaw(dcType+"mac_a3.mobileconfig", []byte(strings.ReplaceAll(text, "\n", "\r\n")))
	// iOS behaves like macOS.
	f.add(dcType+"ios_1.yaml", ios(), customProfile(odataIOSCustom, plist(rootIDA, payloadMark)))
	f.add(dcType+"ios_2.yaml", ios(), customProfile(odataIOSCustom, plist(rootIDA, payloadMark+"-ios")))
	// Two app configurations on one bundleId with different XML.
	f.add(dcType+"app_1.yaml", mac(odataMacOSCustomApp), appConfig(bundleIDTest, "<dict><key>"+payloadMark+"</key><true/></dict>"))
	f.add(dcType+"app_2.yaml", mac(odataMacOSCustomApp), appConfig(bundleIDTest, "<dict><key>"+payloadMark+"</key><false/></dict>"))
	dir := f.save()

	res, err := Analyze(Options{TenantDir: dir, ToolVersion: "v"})
	if err != nil {
		t.Fatal(err)
	}
	got := findingsByKey(res.Mechanical.Findings)
	macKey := odataMacOSCustom + "#payload:" + rootIDA
	iosKey := odataIOSCustom + "#payload:" + rootIDA
	appKey := odataMacOSCustomApp + "#configurationXml:" + bundleIDTest
	want := []string{
		"conflict|" + macKey + "|" + dcType + "mac_a1.yaml|" + dcType + "mac_a2.yaml",
		"conflict|" + macKey + "|" + dcType + "mac_a2.yaml|" + dcType + "mac_a3.yaml",
		"duplicate|" + macKey + "|" + dcType + "mac_a1.yaml|" + dcType + "mac_a3.yaml",
		"conflict|" + iosKey + "|" + dcType + "ios_1.yaml|" + dcType + "ios_2.yaml",
		"conflict|" + appKey + "|" + dcType + "app_1.yaml|" + dcType + "app_2.yaml",
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing finding %s", k)
		}
	}
	if len(got) != len(want) {
		t.Errorf("want exactly %d findings, got %+v", len(want), res.Mechanical.Findings)
	}

	for _, name := range []string{MechanicalFileName, MetadataFileName} {
		data, err := os.ReadFile(filepath.Join(dir, DirName, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, leak := range []string{payloadMark, "PayloadDisplayName", innerID, "Synthetic name"} {
			if bytes.Contains(data, []byte(leak)) {
				t.Errorf("%s leaks payload content %q", name, leak)
			}
		}
	}
}
