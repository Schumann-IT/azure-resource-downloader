package consistency

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"azure-resource-downloader/internal/logger"
	"azure-resource-downloader/internal/transform"
)

// The Apple custom profile types a device replaces by identifier.
const (
	odataMacOSCustom    = "#microsoft.graph.macOSCustomConfiguration"
	odataIOSCustom      = "#microsoft.graph.iosCustomConfiguration"
	odataMacOSCustomApp = "#microsoft.graph.macOSCustomAppConfiguration"
)

// appleCustomPayloadProperty names, per Apple custom profile type, the
// property that carries its payload. These types are keyed by their payload
// identifier, never by their typed properties.
var appleCustomPayloadProperty = map[string]string{
	odataMacOSCustom:    "payload",
	odataIOSCustom:      "payload",
	odataMacOSCustomApp: "configurationXml",
}

// indexAppleCustomProfile indexes a macOS/iOS custom configuration profile by
// the root PayloadIdentifier of its property list, and a macOS custom app
// configuration by its bundleId (the app's preference domain). The device keeps
// one profile per identifier, so the same identifier with different payloads
// is a conflict. The value is the SHA-256 of the normalised payload text; the
// payload itself never leaves the export. Whatever indexes nothing logs at
// DEBUG with the resource key and a fixed reason only.
func indexAppleCustomProfile(resource string, doc map[string]interface{}, artifacts []string, typeDir string, c *collector) {
	odataType, _ := doc["@odata.type"].(string)
	property := appleCustomPayloadProperty[odataType]
	skip := func(reason string) {
		logger.Default.Debug("Custom profile not indexed by the consistency analysis", "resource", resource, "reason", reason)
	}

	var identity string
	if odataType == odataMacOSCustomApp {
		bundleID, _ := doc["bundleId"].(string)
		if identity = strings.TrimSpace(bundleID); identity == "" {
			skip("no bundleId")
			return
		}
	}

	data, reason := applePayloadBytes(doc, property, artifacts, typeDir)
	if data == nil {
		skip(reason)
		return
	}
	text := strings.TrimRight(transform.NormalizeInlineText(data), "\n")

	if odataType != odataMacOSCustomApp {
		if identity = plistRootIdentifier(text); identity == "" {
			skip("payload is not an XML property list with a root PayloadIdentifier")
			return
		}
	}

	sum := sha256.Sum256([]byte(text))
	value := "sha256:" + hex.EncodeToString(sum[:])
	key := typedKey(odataType, property+":"+identity)
	c.add(key, rawValue{sourceKey: key, value: value, scalar: value})
}

// applePayloadBytes returns a custom profile's payload: the resource's single
// sidecar artifact when the export wrote one, else the inline string, which is
// base64-decoded first when it is not markup (file mode without remove-source,
// or the base64-decode transformer off, leaves Graph's base64 in the YAML). On
// failure it returns nil and a fixed reason that never quotes the payload.
func applePayloadBytes(doc map[string]interface{}, property string, artifacts []string, typeDir string) ([]byte, string) {
	if len(artifacts) == 1 {
		data, err := os.ReadFile(filepath.Join(typeDir, filepath.Base(artifacts[0])))
		if err != nil {
			return nil, "sidecar artifact not readable"
		}
		return data, ""
	}
	inline, _ := doc[property].(string)
	trimmed := strings.TrimSpace(strings.TrimPrefix(inline, "\uFEFF"))
	if trimmed == "" {
		return nil, "no payload"
	}
	if strings.HasPrefix(trimmed, "<") {
		return []byte(inline), ""
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(trimmed), ""))
	if err != nil {
		return nil, "payload is neither markup nor base64"
	}
	return decoded, ""
}

// plistRootIdentifier reads PayloadIdentifier from the root <dict> of an XML
// property list — not from the inner PayloadContent payloads. It returns ""
// for anything that is not an XML plist (a signed CMS profile, a binary
// bplist) or carries no root PayloadIdentifier.
func plistRootIdentifier(text string) string {
	if !strings.HasPrefix(strings.TrimSpace(text), "<") {
		return ""
	}
	dec := xml.NewDecoder(bytes.NewReader([]byte(text)))
	seenPlist := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch {
		case !seenPlist && start.Name.Local == "plist":
			seenPlist = true
		case seenPlist && start.Name.Local == "dict":
			return rootDictIdentifier(dec)
		default:
			return ""
		}
	}
}

// rootDictIdentifier scans the direct children of the root dict the decoder
// has just entered, skipping nested values, for the PayloadIdentifier string.
func rootDictIdentifier(dec *xml.Decoder) string {
	wantValue := false
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return "" // the root dict ended without the key
		case xml.StartElement:
			id, done := rootDictChild(dec, t, &wantValue)
			if done {
				return id
			}
		}
	}
}

// rootDictChild handles one child element of the root dict. A <key> arms
// wantValue when it names PayloadIdentifier; the value that follows is read as
// the identifier. done reports that the scan is over, with id the result.
func rootDictChild(dec *xml.Decoder, t xml.StartElement, wantValue *bool) (id string, done bool) {
	switch {
	case t.Name.Local == "key":
		name, ok := keyName(dec, t)
		*wantValue = name == "PayloadIdentifier"
		return "", !ok
	case *wantValue:
		return stringValue(dec, t), true
	default:
		return "", dec.Skip() != nil
	}
}

// stringValue reads the trimmed text of a <string> element, or "" for any other
// element or a decode failure.
func stringValue(dec *xml.Decoder, start xml.StartElement) string {
	if start.Name.Local != "string" {
		return ""
	}
	var id string
	if err := dec.DecodeElement(&id, &start); err != nil {
		return ""
	}
	return strings.TrimSpace(id)
}

// keyName reads the trimmed text of a <key> element.
func keyName(dec *xml.Decoder, start xml.StartElement) (string, bool) {
	var name string
	if err := dec.DecodeElement(&name, &start); err != nil {
		return "", false
	}
	return strings.TrimSpace(name), true
}
