package consistency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Source types the setting index reads. Every other type is not indexed in v1.
const (
	typeSettingsCatalog          = "Microsoft.Graph/deviceManagementConfigurationPolicies"
	typeCompliancePolicies       = "Microsoft.Graph/compliancePolicies"
	typeDeviceConfigurations     = "Microsoft.Graph/deviceConfigurations"
	typeDeviceCompliancePolicies = "Microsoft.Graph/deviceCompliancePolicies"
	typeGroupPolicyConfigs       = "Microsoft.Graph/groupPolicyConfigurations"
	typeIntents                  = "Microsoft.Graph/deviceManagementIntents"
)

// indexedTypes lists the source types in a fixed order, so counts and
// iteration never depend on map order.
var indexedTypes = []string{
	typeCompliancePolicies,
	typeDeviceCompliancePolicies,
	typeDeviceConfigurations,
	typeIntents,
	typeSettingsCatalog,
	typeGroupPolicyConfigs,
}

// Class says whether a setting is checked (a compliance requirement) or applied
// (a configuration).
type Class string

// The two setting classes.
const (
	ClassRequirement   Class = "requirement"
	ClassConfiguration Class = "configuration"
)

// classOf returns the class of every setting a source type contributes.
func classOf(sourceType string) Class {
	if sourceType == typeDeviceCompliancePolicies || sourceType == typeCompliancePolicies {
		return ClassRequirement
	}
	return ClassConfiguration
}

// Value families: two settings on the same key compare by their canonical
// value only when they come from the same family; across families (the
// OMA-URI ↔ Settings Catalog bridge, or an equivalence) they compare by their
// scalar form.
const (
	familyCatalog = "catalog"
	familyOMA     = "oma"
	familyTyped   = "typed"
	familyADMX    = "admx"
	familyIntent  = "intent"
)

// Setting is one entry of the setting index: a resource configures a canonical
// key to a value. Several raw occurrences of one key in one resource (a
// collection, a group collection) are folded into one Setting whose value is
// the sorted list.
type Setting struct {
	// Resource is the resource's metadata key ("<type>/<name>.yaml").
	Resource string
	// SourceType is the resource type the setting was read from.
	SourceType string
	// Key is the canonical key settings are joined on.
	Key string
	// SourceKey is the key as written in the resource (settingDefinitionId,
	// omaUri, @odata.type#property, definition id).
	SourceKey string
	// Value is the canonical, comparable value; empty when Unknown.
	Value string
	// Scalar is the value's scalar form used across families (a choice's
	// option suffix, an OMA-URI value as a string); empty when the value is
	// not a scalar.
	Scalar string
	// Unknown marks a setting that is set but whose value cannot be read or
	// must not be: a secret, an unparseable value. It is never compared and
	// its value never leaves the export.
	Unknown bool
	// Class is requirement (compliance) or configuration.
	Class Class
	// ListMember marks an additive collection key: the resource adds the
	// entries in Members to a list the device installs side by side with
	// every other resource's (an Apple Settings Catalog nested collection).
	// Such a setting has no Value or Scalar, never crosses the OMA-URI bridge
	// or an equivalence, and only ever yields a duplicate on shared members.
	ListMember bool
	// Members are the sorted, distinct known member values of a ListMember
	// setting, each in canonical JSON.
	Members []string
	// UnknownMembers marks a ListMember setting with at least one member that
	// cannot or must not be read (a secret); such a member is never compared
	// and never written.
	UnknownMembers bool

	family string
}

// rawValue is one occurrence of a key before folding.
type rawValue struct {
	sourceKey string
	value     string
	scalar    string
	unknown   bool
	// member marks one entry of an additive collection; value is then the
	// member's canonical JSON.
	member bool
}

// collector folds raw occurrences into Settings per key for one resource.
type collector struct {
	family string
	byKey  map[string][]rawValue
	// additive makes nested Settings Catalog collections member lists (an
	// Apple-only resource); otherwise every occurrence folds into one value.
	additive bool
}

func newCollector(family string) *collector {
	return &collector{family: family, byKey: map[string][]rawValue{}}
}

// addMember records one entry of an additive collection under key; an
// unknown member keeps no value.
func (c *collector) addMember(key, sourceKey, value string, unknown bool) {
	if unknown {
		value = ""
	}
	c.add(key, rawValue{sourceKey: sourceKey, value: value, unknown: unknown, member: true})
}

// hasMembers reports whether a key's occurrences are collection members.
func hasMembers(raws []rawValue) bool {
	for _, r := range raws {
		if r.member {
			return true
		}
	}
	return false
}

func (c *collector) add(key string, v rawValue) {
	if key == "" {
		return
	}
	c.byKey[key] = append(c.byKey[key], v)
}

// settings returns the folded settings, sorted by key.
func (c *collector) settings(resource, sourceType string) []Setting {
	keys := make([]string, 0, len(c.byKey))
	for k := range c.byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]Setting, 0, len(keys))
	for _, k := range keys {
		raws := c.byKey[k]
		s := Setting{
			Resource:   resource,
			SourceType: sourceType,
			Key:        k,
			Class:      classOf(sourceType),
			SourceKey:  firstSourceKey(raws),
			family:     c.family,
		}
		if hasMembers(raws) {
			foldMembers(&s, raws)
		} else {
			foldValues(&s, raws)
		}
		out = append(out, s)
	}
	return out
}

// firstSourceKey returns the lowest source key of a key's occurrences.
func firstSourceKey(raws []rawValue) string {
	sourceKeys := make([]string, 0, len(raws))
	for _, r := range raws {
		sourceKeys = append(sourceKeys, r.sourceKey)
	}
	sort.Strings(sourceKeys)
	return sourceKeys[0]
}

// foldValues folds a key's occurrences into one value: the single value, or
// the sorted list of all of them.
func foldValues(s *Setting, raws []rawValue) {
	values := make([]string, 0, len(raws))
	for _, r := range raws {
		values = append(values, r.value)
		if r.unknown {
			s.Unknown = true
		}
	}
	switch {
	case s.Unknown:
		// Never carry a value of a setting that holds an unknown part.
	case len(raws) == 1:
		s.Value = raws[0].value
		s.Scalar = raws[0].scalar
	default:
		sort.Strings(values)
		s.Value = canonicalJSON(values)
	}
}

// foldMembers turns a key's occurrences into the members of an additive
// collection: sorted, distinct known members, and a flag for unknown ones.
func foldMembers(s *Setting, raws []rawValue) {
	s.ListMember = true
	seen := map[string]bool{}
	for _, r := range raws {
		switch {
		case r.unknown:
			s.UnknownMembers = true
		case r.member:
			seen[r.value] = true
		default:
			// A plain value on a member key can never match a member.
			s.UnknownMembers = true
		}
	}
	s.Members = sortedSet(seen)
}

// typedNonSettings are top-level properties of the typed (legacy) resources
// that describe the object rather than configure a setting.
var typedNonSettings = map[string]bool{
	"id":                      true,
	"displayName":             true,
	"description":             true,
	"version":                 true,
	"createdDateTime":         true,
	"lastModifiedDateTime":    true,
	"roleScopeTagIds":         true,
	"assignments":             true,
	"scheduledActionsForRule": true,
	"omaSettings":             true,
	// A read-only Graph capability flag, true on every typed profile.
	"supportsScopeTags": true,
}

// typedEnumDefaults lists, per top-level typed property (any @odata.type), the
// enum values Graph returns by default and that therefore mean "not
// configured". An explicit per-property list: the same strings may be a real
// choice on another property.
var typedEnumDefaults = map[string][]string{
	"deviceThreatProtectionRequiredSecurityLevel":   {"unavailable"},
	"advancedThreatProtectionRequiredSecurityLevel": {"unavailable"},
	"passwordRequiredType":                          {"deviceDefault"},
	// The iOS/iPadOS compliance spelling of the same Graph default.
	"passcodeRequiredType": {"deviceDefault"},
}

// isTypedEnumDefault reports whether v is a top-level property's Graph default.
func isTypedEnumDefault(path string, v interface{}) bool {
	s, ok := v.(string)
	if !ok || strings.Contains(path, ".") {
		return false
	}
	for _, d := range typedEnumDefaults[path] {
		if strings.EqualFold(s, d) {
			return true
		}
	}
	return false
}

// isODataKey reports whether a property key is OData annotation, never a
// setting.
func isODataKey(k string) bool {
	return strings.Contains(k, "@odata.")
}

// notConfigured reports whether a leaf value means "not configured": null, an
// empty string or list, the enum value notConfigured, or — when falseIsUnset —
// boolean false (the legacy profiles render false as "Not configured", so an
// explicit false is missed rather than every default reported as a duplicate).
func notConfigured(v interface{}, falseIsUnset bool) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == "" || t == "notConfigured"
	case []interface{}:
		return len(t) == 0
	case map[string]interface{}:
		return len(t) == 0
	case bool:
		return !t && falseIsUnset
	}
	return false
}

// scalarString renders a YAML scalar in one comparable form; ok is false for
// lists and maps.
func scalarString(v interface{}) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	}
	return "", false
}

// canonicalJSON renders any decoded value as JSON with sorted map keys and
// OData annotations removed, so equal content always renders equal.
func canonicalJSON(v interface{}) string {
	data, err := json.Marshal(stripOData(v))
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

// stripOData removes OData annotation keys at any depth and sorts lists by
// their canonical form, so the order a service returned a list in never makes
// two equal configurations differ.
func stripOData(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, val := range t {
			if isODataKey(k) {
				continue
			}
			out[k] = stripOData(val)
		}
		return out
	case []interface{}:
		items := make([]interface{}, 0, len(t))
		for _, val := range t {
			items = append(items, stripOData(val))
		}
		sort.SliceStable(items, func(i, j int) bool {
			a, _ := json.Marshal(items[i])
			b, _ := json.Marshal(items[j])
			return string(a) < string(b)
		})
		return items
	}
	return v
}

// leafValue builds the raw value of a typed or simple leaf.
func leafValue(sourceKey string, v interface{}) rawValue {
	if s, ok := scalarString(v); ok {
		return rawValue{sourceKey: sourceKey, value: s, scalar: s}
	}
	return rawValue{sourceKey: sourceKey, value: canonicalJSON(v)}
}

// indexSettingsCatalog indexes a Settings Catalog policy or a Settings Catalog
// compliance policy by settingDefinitionId, walking choice children and group
// collections.
func indexSettingsCatalog(doc map[string]interface{}, c *collector) {
	items, _ := doc["settings"].([]interface{})
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if inst, ok := item["settingInstance"].(map[string]interface{}); ok {
			walkCatalogInstance(inst, c, true)
		}
	}
}

// walkCatalogInstance indexes one setting instance and recurses into the
// children it carries. top marks the settingInstance of a settings[] item:
// on Apple it is the payload instance, whose collection elements are payloads
// and never members, so only collections below it can be additive.
func walkCatalogInstance(inst map[string]interface{}, c *collector, top bool) {
	id, _ := inst["settingDefinitionId"].(string)
	if id == "" {
		return
	}
	if cv, ok := inst["choiceSettingValue"].(map[string]interface{}); ok {
		walkCatalogChoice(id, cv, c)
	}
	if sv, ok := inst["simpleSettingValue"].(map[string]interface{}); ok {
		addCatalogSimple(id, sv, c)
	}
	if gv, ok := inst["groupSettingValue"].(map[string]interface{}); ok {
		walkCatalogChildren(gv, c)
	}
	if c.additive && !top {
		addCatalogMembers(id, inst, c)
		return
	}
	for _, raw := range listOf(inst["choiceSettingCollectionValue"]) {
		if cv, ok := raw.(map[string]interface{}); ok {
			walkCatalogChoice(id, cv, c)
		}
	}
	for _, raw := range listOf(inst["simpleSettingCollectionValue"]) {
		if sv, ok := raw.(map[string]interface{}); ok {
			addCatalogSimple(id, sv, c)
		}
	}
	for _, raw := range listOf(inst["groupSettingCollectionValue"]) {
		if gv, ok := raw.(map[string]interface{}); ok {
			walkCatalogChildren(gv, c)
		}
	}
}

func walkCatalogChildren(value map[string]interface{}, c *collector) {
	for _, raw := range listOf(value["children"]) {
		if child, ok := raw.(map[string]interface{}); ok {
			walkCatalogInstance(child, c, false)
		}
	}
}

// addCatalogMembers indexes the collections of a nested instance as additive
// lists under the collection's lowercased id: one member per element.
func addCatalogMembers(id string, inst map[string]interface{}, c *collector) {
	key := strings.ToLower(id)
	for _, raw := range listOf(inst["choiceSettingCollectionValue"]) {
		cv, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if option, _ := cv["value"].(string); !notConfigured(option, false) {
			c.addMember(key, id, canonicalJSON(option), false)
		}
		// An option's children are still settings of their own.
		walkCatalogChildren(cv, c)
	}
	for _, raw := range listOf(inst["simpleSettingCollectionValue"]) {
		if sv, ok := raw.(map[string]interface{}); ok {
			addSimpleMember(key, id, sv, c)
		}
	}
	for _, raw := range listOf(inst["groupSettingCollectionValue"]) {
		if gv, ok := raw.(map[string]interface{}); ok {
			addGroupMember(key, id, gv, c)
		}
	}
}

// addSimpleMember adds one simple collection value as a member; a secret is
// an unknown member.
func addSimpleMember(key, id string, sv map[string]interface{}, c *collector) {
	if isCatalogSecret(sv) {
		c.addMember(key, id, "", true)
		return
	}
	if v := sv["value"]; !notConfigured(v, true) {
		c.addMember(key, id, canonicalJSON(v), false)
	}
}

// addGroupMember adds one group collection element as a member valued by the
// canonical JSON of its children, indexed into a fresh collector so every
// value rule applies unchanged. A collection nested inside the element is a
// member list of its own and stays out of the element's value; an element
// with no configured child adds no member; one holding an unknown setting is
// an unknown member.
func addGroupMember(key, id string, gv map[string]interface{}, c *collector) {
	element := newCollector(c.family)
	element.additive = true
	walkCatalogChildren(gv, element)

	children := map[string]interface{}{}
	unknown := false
	for _, s := range element.settings("", "") {
		if s.ListMember {
			c.byKey[s.Key] = append(c.byKey[s.Key], element.byKey[s.Key]...)
			continue
		}
		if s.Unknown {
			unknown = true
			continue
		}
		children[s.Key] = s.Value
	}
	switch {
	case unknown:
		c.addMember(key, id, "", true)
	case len(children) > 0:
		c.addMember(key, id, canonicalJSON(children), false)
	}
}

// walkCatalogChoice indexes a choice by its option id; across the OMA-URI
// bridge it compares by the option suffix (the part after "<id>_").
func walkCatalogChoice(id string, cv map[string]interface{}, c *collector) {
	if option, _ := cv["value"].(string); !notConfigured(option, false) {
		scalar := option
		if rest, ok := strings.CutPrefix(strings.ToLower(option), strings.ToLower(id)+"_"); ok {
			scalar = rest
		}
		c.add(strings.ToLower(id), rawValue{sourceKey: id, value: option, scalar: scalar})
	}
	walkCatalogChildren(cv, c)
}

// isCatalogSecret reports a secret simple value: valueState present, or a
// secret value type.
func isCatalogSecret(sv map[string]interface{}) bool {
	odataType, _ := sv["@odata.type"].(string)
	_, secret := sv["valueState"]
	return secret || strings.Contains(odataType, "SecretSettingValue")
}

// addCatalogSimple indexes a simple value. A secret (valueState present, or a
// secret value type) is set with an unknown value, whatever the export holds.
func addCatalogSimple(id string, sv map[string]interface{}, c *collector) {
	key := strings.ToLower(id)
	if isCatalogSecret(sv) {
		c.add(key, rawValue{sourceKey: id, unknown: true})
		return
	}
	v := sv["value"]
	if notConfigured(v, true) {
		return
	}
	c.add(key, leafValue(id, v))
}

// appleOnly reports whether every platform family of a resource is macOS or
// iOS/iPadOS, where the device installs every profile's nested collection
// entries side by side. Unknown, mixed or other platforms keep folding.
func appleOnly(platforms []string) bool {
	for _, p := range platforms {
		if p != PlatformMacOS && p != PlatformIOS {
			return false
		}
	}
	return len(platforms) > 0
}

func listOf(v interface{}) []interface{} {
	l, _ := v.([]interface{})
	return l
}

// normaliseOMAURI maps an OMA-URI path onto the Settings Catalog id form:
// lowercase, leading "./" dropped, "vendor/msft/…" read as
// "device/vendor/msft/…", "/" → "_". Area and setting are deliberately not
// split: ADMX-backed ids contain underscores of their own.
func normaliseOMAURI(uri string) string {
	k := strings.ToLower(strings.TrimSpace(uri))
	k = strings.TrimPrefix(k, "./")
	k = strings.TrimPrefix(k, "/")
	if strings.HasPrefix(k, "vendor/msft/") {
		k = "device/" + k
	}
	return strings.ReplaceAll(k, "/", "_")
}

// isMaskedValue reports a value the service masked (a run of asterisks).
func isMaskedValue(v interface{}) bool {
	s, ok := v.(string)
	return ok && len(s) >= 3 && strings.Trim(s, "*") == ""
}

// indexOMASettings indexes a custom profile's omaSettings by normalised
// OMA-URI. An encrypted, secret-referenced or masked value is set with an
// unknown value even when the export resolved it to plaintext. A value the
// export moved to a sidecar artifact compares by the artifact bytes' SHA-256.
func indexOMASettings(doc map[string]interface{}, artifacts map[string]bool, typeDir string, c *collector) {
	for _, raw := range listOf(doc["omaSettings"]) {
		if s, ok := raw.(map[string]interface{}); ok {
			indexOMASetting(s, artifacts, typeDir, c)
		}
	}
}

// indexOMASetting indexes one omaSettings item.
func indexOMASetting(s map[string]interface{}, artifacts map[string]bool, typeDir string, c *collector) {
	uri, _ := s["omaUri"].(string)
	if strings.TrimSpace(uri) == "" {
		return
	}
	key := normaliseOMAURI(uri)
	encrypted, _ := s["isEncrypted"].(bool)
	ref, _ := s["secretReferenceValueId"].(string)
	value, hasValue := s["value"]
	if encrypted || ref != "" || isMaskedValue(value) {
		c.add(key, rawValue{sourceKey: uri, unknown: true})
		return
	}
	if !hasValue {
		if fileName, _ := s["fileName"].(string); fileName != "" && artifacts[fileName] {
			c.add(key, artifactValue(uri, filepath.Join(typeDir, filepath.Base(fileName))))
		}
		return
	}
	if notConfigured(value, false) {
		return
	}
	rv := leafValue(uri, value)
	// An XML payload (an ADMX <enabled/> with its data elements) is not a
	// scalar: across the bridge it can only be listed as unknown.
	if strings.HasPrefix(strings.TrimSpace(rv.scalar), "<") {
		rv.scalar = ""
	}
	c.add(key, rv)
}

// artifactValue hashes a sidecar artifact; a missing artifact is unknown.
func artifactValue(sourceKey, path string) rawValue {
	data, err := os.ReadFile(path)
	if err != nil {
		return rawValue{sourceKey: sourceKey, unknown: true}
	}
	sum := sha256.Sum256(data)
	v := "sha256:" + hex.EncodeToString(sum[:])
	return rawValue{sourceKey: sourceKey, value: v, scalar: v}
}

// indexTypedProperties indexes a typed (legacy) profile or compliance policy
// by "@odata.type#property", nested objects flattened to dotted paths, so keys
// only ever join within one @odata.type.
func indexTypedProperties(doc map[string]interface{}, c *collector) {
	odataType, _ := doc["@odata.type"].(string)
	if odataType == "" {
		return
	}
	for k, v := range doc {
		if typedNonSettings[k] || isODataKey(k) {
			continue
		}
		walkTyped(odataType, k, v, c)
	}
}

func walkTyped(odataType, path string, v interface{}, c *collector) {
	if m, ok := v.(map[string]interface{}); ok {
		for k, child := range m {
			if isODataKey(k) {
				continue
			}
			walkTyped(odataType, path+"."+k, child, c)
		}
		return
	}
	if notConfigured(v, true) || isTypedEnumDefault(path, v) {
		return
	}
	key := odataType + "#" + path
	c.add(key, typedLeaf(key, path, v))
}

// credentialSuffixes end the property names whose values are credentials.
var credentialSuffixes = []string{"password", "passphrase", "secret", "sharedkey", "token"}

// typedLeaf builds the raw value of one typed property. A credential-named
// property is unknown (never written in clear).
func typedLeaf(key, path string, v interface{}) rawValue {
	last := strings.ToLower(path[strings.LastIndex(path, ".")+1:])
	for _, suffix := range credentialSuffixes {
		if strings.HasSuffix(last, suffix) {
			return rawValue{sourceKey: key, unknown: true}
		}
	}
	return leafValue(key, v)
}

// indexGroupPolicy indexes an Administrative Templates profile by definition
// id. Presentation values are not exported, so the value is enabled/disabled.
func indexGroupPolicy(doc map[string]interface{}, c *collector) {
	for _, raw := range listOf(doc["definitionValues"]) {
		dv, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		def, _ := dv["definition"].(map[string]interface{})
		id, _ := def["id"].(string)
		enabled, ok := dv["enabled"].(bool)
		if id == "" || !ok {
			continue
		}
		v := "disabled"
		if enabled {
			v = "enabled"
		}
		c.add(strings.ToLower(id), rawValue{sourceKey: id, value: v, scalar: v})
	}
}

// indexIntent indexes a security-baseline / endpoint-security intent by
// definitionId with the parsed valueJson in canonical JSON.
func indexIntent(doc map[string]interface{}, c *collector) {
	for _, raw := range listOf(doc["settings"]) {
		s, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := s["definitionId"].(string)
		if id == "" {
			continue
		}
		valueJSON, ok := s["valueJson"].(string)
		if !ok || valueJSON == "" {
			continue
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(valueJSON), &parsed); err != nil {
			c.add(id, rawValue{sourceKey: id, unknown: true})
			continue
		}
		if notConfigured(parsed, true) {
			continue
		}
		rv := rawValue{sourceKey: id, value: canonicalJSON(parsed)}
		if s, ok := scalarString(parsed); ok {
			rv.scalar = s
		}
		c.add(id, rv)
	}
}

// indexResource indexes one parsed resource document of a source type.
// platforms are the resource's platform families (resourcePlatforms).
func indexResource(resource, sourceType string, doc map[string]interface{}, artifacts []string, typeDir string, platforms []string) []Setting {
	switch sourceType {
	case typeSettingsCatalog, typeCompliancePolicies:
		c := newCollector(familyCatalog)
		c.additive = appleOnly(platforms)
		indexSettingsCatalog(doc, c)
		return c.settings(resource, sourceType)
	case typeDeviceConfigurations:
		if _, custom := doc["omaSettings"].([]interface{}); custom {
			names := map[string]bool{}
			for _, a := range artifacts {
				names[a] = true
			}
			c := newCollector(familyOMA)
			indexOMASettings(doc, names, typeDir, c)
			return c.settings(resource, sourceType)
		}
		c := newCollector(familyTyped)
		if odataType, _ := doc["@odata.type"].(string); appleCustomPayloadProperty[odataType] != "" {
			indexAppleCustomProfile(resource, doc, artifacts, typeDir, c)
		} else {
			indexTypedProperties(doc, c)
		}
		return c.settings(resource, sourceType)
	case typeDeviceCompliancePolicies:
		c := newCollector(familyTyped)
		indexTypedProperties(doc, c)
		return c.settings(resource, sourceType)
	case typeGroupPolicyConfigs:
		c := newCollector(familyADMX)
		indexGroupPolicy(doc, c)
		return c.settings(resource, sourceType)
	case typeIntents:
		c := newCollector(familyIntent)
		indexIntent(doc, c)
		return c.settings(resource, sourceType)
	}
	return nil
}
