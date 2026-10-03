package consistency

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexSettingsCatalog(t *testing.T) {
	doc := catalogPolicy("windows10",
		catalogChoice("device_vendor_msft_policy_config_defender_allowrealtimemonitoring",
			"device_vendor_msft_policy_config_defender_allowrealtimemonitoring_1",
			simpleInstance("device_vendor_msft_policy_config_defender_child", 5)),
		map[string]interface{}{"settingInstance": map[string]interface{}{
			"settingDefinitionId": "group_root",
			"groupSettingCollectionValue": []interface{}{
				map[string]interface{}{"children": []interface{}{simpleInstance("group_item_value", "b")}},
				map[string]interface{}{"children": []interface{}{simpleInstance("group_item_value", "a")}},
			},
		}},
		map[string]interface{}{"settingInstance": map[string]interface{}{
			"settingDefinitionId": "simple_collection",
			"simpleSettingCollectionValue": []interface{}{
				map[string]interface{}{"value": "y"},
				map[string]interface{}{"value": "x"},
			},
		}},
		map[string]interface{}{"settingInstance": map[string]interface{}{
			"settingDefinitionId": "choice_collection",
			"choiceSettingCollectionValue": []interface{}{
				map[string]interface{}{"value": "choice_collection_2", "children": []interface{}{}},
			},
		}},
		catalogSimple("empty_string", ""),
	)

	got := indexOf(t, "Microsoft.Graph/deviceManagementConfigurationPolicies/p.yaml", typeSettingsCatalog, doc)

	choice := got["device_vendor_msft_policy_config_defender_allowrealtimemonitoring"]
	equal(t, "device_vendor_msft_policy_config_defender_allowrealtimemonitoring_1", choice.Value)
	equal(t, "1", choice.Scalar, "a choice's bridge form is its option suffix")
	equal(t, ClassConfiguration, choice.Class)

	equal(t, "5", got["device_vendor_msft_policy_config_defender_child"].Value, "choice children are walked")
	equal(t, `["a","b"]`, got["group_item_value"].Value, "group collection items fold into one sorted value")
	empty(t, got["group_item_value"].Scalar, "a folded collection is not a scalar")
	equal(t, `["x","y"]`, got["simple_collection"].Value)
	equal(t, "2", got["choice_collection"].Scalar)
	absent(t, got, "group_root", "a group is a container, not a setting")
	absent(t, got, "empty_string", "an empty string is not configured")
}

func TestIndexCompliancePoliciesAreRequirements(t *testing.T) {
	doc := catalogPolicy("macOS", catalogSimple("com.apple.passcode_minlength", 12))
	got := indexOf(t, "Microsoft.Graph/compliancePolicies/c.yaml", typeCompliancePolicies, doc)
	equal(t, ClassRequirement, got["com.apple.passcode_minlength"].Class)

	typed := map[string]interface{}{"@odata.type": "#microsoft.graph.macOSCompliancePolicy", "passwordMinimumLength": 12}
	got = indexOf(t, "Microsoft.Graph/deviceCompliancePolicies/c.yaml", typeDeviceCompliancePolicies, typed)
	equal(t, ClassRequirement, got["#microsoft.graph.macOSCompliancePolicy#passwordMinimumLength"].Class)
}

func TestNormaliseOMAURI(t *testing.T) {
	tests := map[string]string{
		"./Device/Vendor/MSFT/Policy/Config/Defender/AllowRealtimeMonitoring":          "device_vendor_msft_policy_config_defender_allowrealtimemonitoring",
		"./Vendor/MSFT/Policy/Config/Browser/AllowCookies":                             "device_vendor_msft_policy_config_browser_allowcookies",
		"./User/Vendor/MSFT/Policy/Config/Start/HideRecentlyAddedApps":                 "user_vendor_msft_policy_config_start_hiderecentlyaddedapps",
		"./Device/Vendor/MSFT/Policy/Config/ADMX_Power/PW_PromptPasswordOnResume_DC_1": "device_vendor_msft_policy_config_admx_power_pw_promptpasswordonresume_dc_1",
		"  ./device/vendor/msft/Policy/Config/Update/ActiveHoursStart  ":               "device_vendor_msft_policy_config_update_activehoursstart",
	}
	for in, want := range tests {
		equal(t, want, normaliseOMAURI(in), in)
	}
}

func TestIndexOMASettings(t *testing.T) {
	typeDir := t.TempDir()
	payload := []byte("<xml/>")
	if err := os.WriteFile(filepath.Join(typeDir, "policy.xml"), payload, 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)

	doc := map[string]interface{}{
		"@odata.type": "#microsoft.graph.windows10CustomConfiguration",
		"displayName": "custom",
		"omaSettings": []interface{}{
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/Int", "value": 1},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/Bool", "value": false},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/Xml", "value": "<enabled/><data id=\"x\" value=\"1\"/>"},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/File", "fileName": "policy.xml"},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/Secret", "value": "s3cr3t-Plaintext", "isEncrypted": true},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/Masked", "value": "********"},
			map[string]interface{}{"omaUri": "./Device/Vendor/MSFT/Policy/Config/A/Ref", "value": "x", "secretReferenceValueId": "ref"},
		},
		"someTypedProperty": "ignored for custom profiles",
	}
	settings := indexResource("Microsoft.Graph/deviceConfigurations/c.yaml", typeDeviceConfigurations, doc, []string{"policy.xml"}, typeDir, nil)
	got := map[string]Setting{}
	for _, s := range settings {
		got[s.Key] = s
	}

	equal(t, "1", got["device_vendor_msft_policy_config_a_int"].Scalar)
	equal(t, "false", got["device_vendor_msft_policy_config_a_bool"].Value, "an explicit OMA-URI false is a value")
	empty(t, got["device_vendor_msft_policy_config_a_xml"].Scalar, "an XML payload is not a scalar")
	equal(t, "sha256:"+hex.EncodeToString(sum[:]), got["device_vendor_msft_policy_config_a_file"].Value)
	for _, k := range []string{"device_vendor_msft_policy_config_a_secret", "device_vendor_msft_policy_config_a_masked", "device_vendor_msft_policy_config_a_ref"} {
		isTrue(t, got[k].Unknown, k)
		empty(t, got[k].Value, k)
	}
	hasLen(t, got, 7, "a custom profile is indexed by its OMA-URIs only")
}

func TestIndexTypedProperties(t *testing.T) {
	doc := map[string]interface{}{
		"@odata.type":                        "#microsoft.graph.windowsUpdateForBusinessConfiguration",
		"@odata.context":                     "ctx",
		"id":                                 "id",
		"displayName":                        "name",
		"description":                        "d",
		"version":                            3,
		"createdDateTime":                    "t",
		"lastModifiedDateTime":               "t",
		"roleScopeTagIds":                    []interface{}{"0"},
		"assignments":                        []interface{}{"x"},
		"scheduledActionsForRule":            []interface{}{"x"},
		"qualityUpdatesDeferralPeriodInDays": 7,
		"driversExcluded":                    false,
		"automaticUpdateMode":                "notConfigured",
		"emptyList":                          []interface{}{},
		"nothing":                            nil,
		"installationSchedule": map[string]interface{}{
			"@odata.type":          "#microsoft.graph.windowsUpdateScheduledInstall",
			"scheduledInstallDay":  "everyday",
			"scheduledInstallTime": "03:00:00",
		},
		"allowed": []interface{}{"b", "a"},
	}
	got := indexOf(t, "Microsoft.Graph/deviceConfigurations/u.yaml", typeDeviceConfigurations, doc)

	prefix := "#microsoft.graph.windowsUpdateForBusinessConfiguration#"
	equal(t, "7", got[prefix+"qualityUpdatesDeferralPeriodInDays"].Value)
	equal(t, "everyday", got[prefix+"installationSchedule.scheduledInstallDay"].Value, "nested objects flatten to dotted paths")
	equal(t, `["a","b"]`, got[prefix+"allowed"].Value)
	hasLen(t, got, 4, "non-settings, OData annotations and not-configured values are never indexed: %v", got)
}

func TestIndexTypedEnumDefaultsAreNotConfigured(t *testing.T) {
	const prefix = "#microsoft.graph.windows10CompliancePolicy#"
	doc := map[string]interface{}{
		"@odata.type": "#microsoft.graph.windows10CompliancePolicy",
		"deviceThreatProtectionRequiredSecurityLevel":   "unavailable",
		"advancedThreatProtectionRequiredSecurityLevel": "Unavailable",
		"passwordRequiredType":                          "deviceDefault",
		"passcodeRequiredType":                          "DeviceDefault",
	}
	hasLen(t, indexOf(t, "Microsoft.Graph/deviceCompliancePolicies/d.yaml", typeDeviceCompliancePolicies, doc), 0, "the four Graph defaults are not settings")

	doc = map[string]interface{}{
		"@odata.type":          "#microsoft.graph.windows10CompliancePolicy",
		"otherThreatLevel":     "unavailable",
		"passwordRequiredType": "alphanumeric",
		"nested": map[string]interface{}{
			"passwordRequiredType": "deviceDefault",
		},
	}
	got := indexOf(t, "Microsoft.Graph/deviceCompliancePolicies/e.yaml", typeDeviceCompliancePolicies, doc)
	equal(t, "unavailable", got[prefix+"otherThreatLevel"].Value, "unavailable on another property is a value")
	equal(t, "alphanumeric", got[prefix+"passwordRequiredType"].Value)
	equal(t, "deviceDefault", got[prefix+"nested.passwordRequiredType"].Value, "a nested path is never matched")
	hasLen(t, got, 3)
}

func TestIndexTypedCredentials(t *testing.T) {
	wifi := map[string]interface{}{
		"@odata.type":  "#microsoft.graph.windowsWifiConfiguration",
		"preSharedKey": "hunter2",
		"ssid":         "corp",
	}
	got := indexOf(t, "Microsoft.Graph/deviceConfigurations/w.yaml", typeDeviceConfigurations, wifi)
	psk := got["#microsoft.graph.windowsWifiConfiguration#preSharedKey"]
	if !psk.Unknown {
		t.Errorf("a credential-named property must be unknown: %+v", psk)
	}
	empty(t, psk.Value, "a credential value is never written")
}

func TestIndexGroupPolicyAndIntents(t *testing.T) {
	admx := map[string]interface{}{"definitionValues": []interface{}{
		map[string]interface{}{"enabled": true, "definition": map[string]interface{}{"id": "AAAA-Def"}},
		map[string]interface{}{"enabled": false, "definition": map[string]interface{}{"id": "bbbb"}},
		map[string]interface{}{"definition": map[string]interface{}{"id": "no-state"}},
	}}
	got := indexOf(t, "Microsoft.Graph/groupPolicyConfigurations/g.yaml", typeGroupPolicyConfigs, admx)
	equal(t, "enabled", got["aaaa-def"].Value)
	equal(t, "disabled", got["bbbb"].Value, "an ADMX Disabled is a configured state")
	absent(t, got, "no-state")

	intent := map[string]interface{}{"settings": []interface{}{
		map[string]interface{}{"definitionId": "deviceConfiguration--a", "valueJson": `{"b":1,"a":[2,1]}`},
		map[string]interface{}{"definitionId": "deviceConfiguration--b", "valueJson": `"on"`},
		map[string]interface{}{"definitionId": "deviceConfiguration--c", "valueJson": `null`},
		map[string]interface{}{"definitionId": "deviceConfiguration--d", "valueJson": `{not json`},
	}}
	got = indexOf(t, "Microsoft.Graph/deviceManagementIntents/i.yaml", typeIntents, intent)
	equal(t, `{"a":[1,2],"b":1}`, got["deviceConfiguration--a"].Value, "valueJson compares in canonical JSON")
	equal(t, "on", got["deviceConfiguration--b"].Scalar)
	absent(t, got, "deviceConfiguration--c")
	isTrue(t, got["deviceConfiguration--d"].Unknown)
}

func TestCatalogSecretIsUnknown(t *testing.T) {
	doc := catalogPolicy("windows10", map[string]interface{}{"settingInstance": map[string]interface{}{
		"settingDefinitionId": "secret_setting",
		"simpleSettingValue": map[string]interface{}{
			"@odata.type": "#microsoft.graph.deviceManagementConfigurationSecretSettingValue",
			"value":       "plaintext-from-resolve",
			"valueState":  "notEncrypted",
		},
	}})
	got := indexOf(t, "Microsoft.Graph/deviceManagementConfigurationPolicies/s.yaml", typeSettingsCatalog, doc)
	isTrue(t, got["secret_setting"].Unknown)
	empty(t, got["secret_setting"].Value)
}

const (
	loginPayload = "com.apple.servicemanagement"
	loginRules   = "com.apple.servicemanagement_rules"
	ruleType     = "com.apple.servicemanagement_rules_item_ruletype"
	ruleValue    = "com.apple.servicemanagement_rules_item_rulevalue"
	tccPayload   = "com.apple.tcc.configuration-profile-policy"
	tccServices  = "com.apple.tcc.configuration-profile-policy_services"
	tccAccess    = "com.apple.tcc.configuration-profile-policy_services_accessibility"
	tccFiles     = "com.apple.tcc.configuration-profile-policy_services_systempolicyallfiles"
	tccItemID    = "com.apple.tcc.configuration-profile-policy_services_accessibility_item_identifier"
	tccFilesID   = "com.apple.tcc.configuration-profile-policy_services_systempolicyallfiles_item_identifier"
)

// loginRule is one element of the managed login item rules collection.
func loginRule(team string) []interface{} {
	return children(simpleInstance(ruleType, "TeamIdentifier"), simpleInstance(ruleValue, team))
}

// loginItemsPolicy builds a policy whose login item rules collection carries
// one rule per team, beside a payload-level key.
func loginItemsPolicy(platform string, teams ...string) map[string]interface{} {
	rules := make([][]interface{}, 0, len(teams))
	for _, team := range teams {
		rules = append(rules, loginRule(team))
	}
	return catalogPolicy(platform, applePayload(loginPayload,
		simpleInstance("com.apple.servicemanagement_flag", 1),
		groupCollection(loginRules, rules...),
	))
}

func TestIndexAppleNestedCollectionsAreMembers(t *testing.T) {
	got := indexOf(t, scType+"m.yaml", typeSettingsCatalog, loginItemsPolicy("macOS", "TEAMB", "TEAMA"))

	rules := got[loginRules]
	isTrue(t, rules.ListMember, "a nested Apple collection is a member list")
	equal(t, 2, len(rules.Members))
	equal(t, `{"`+ruleType+`":"TeamIdentifier","`+ruleValue+`":"TEAMA"}`, rules.Members[0], "members are sorted canonical JSON")
	empty(t, rules.Value, "a member list has no folded value")
	empty(t, rules.Scalar)
	absent(t, got, ruleType, "the children of an additive collection are no longer keys")
	absent(t, got, ruleValue)
	absent(t, got, loginPayload, "the payload instance is never a member list")

	flag := got["com.apple.servicemanagement_flag"]
	equal(t, "1", flag.Value, "a payload-level key stays a key")
	equal(t, false, flag.ListMember)
	hasLen(t, got, 2)

	ios := indexOf(t, scType+"i.yaml", typeSettingsCatalog, loginItemsPolicy("iOS", "TEAMA"))
	isTrue(t, ios[loginRules].ListMember && len(ios[loginRules].Members) == 1, "iOS is additive too")

	cp := indexOf(t, "Microsoft.Graph/compliancePolicies/m.yaml", typeCompliancePolicies, loginItemsPolicy("macOS", "TEAMA"))
	isTrue(t, cp[loginRules].ListMember && len(cp[loginRules].Members) == 1, "compliance policies are additive too")
}

func TestFoldMembersPlainValueOnMemberKeyIsUnknown(t *testing.T) {
	c := newCollector("apple")
	c.addMember("k", "k", `{"a":"1"}`, false)
	c.add("k", rawValue{sourceKey: "k", value: "plain"})
	got := c.settings("r", typeSettingsCatalog)

	equal(t, 1, len(got))
	isTrue(t, got[0].ListMember)
	isTrue(t, got[0].UnknownMembers, "a plain value on a member key is an unknown member")
	equal(t, 1, len(got[0].Members))
	for _, m := range got[0].Members {
		isTrue(t, !strings.Contains(m, "plain"))
	}
}

func TestIndexNonAppleCollectionsKeepFolding(t *testing.T) {
	for _, platform := range []string{"windows10", "macOS, windows10", ""} {
		got := indexOf(t, scType+"p.yaml", typeSettingsCatalog, loginItemsPolicy(platform, "TEAMB", "TEAMA"))
		absent(t, got, loginRules, platform)
		equal(t, `["TEAMA","TEAMB"]`, got[ruleValue].Value, platform)
		equal(t, false, got[ruleValue].ListMember, platform)
	}
}

func TestIndexAppleTCCMembersAtInnerLevel(t *testing.T) {
	item := func(idKey, identifier string) []interface{} {
		return children(simpleInstance(idKey, identifier))
	}
	doc := catalogPolicy("macOS", applePayload(tccPayload,
		groupCollection(tccServices, children(
			groupCollection(tccAccess, item(tccItemID, "com.example.a"), item(tccItemID, "com.example.b")),
			groupCollection(tccFiles, item(tccFilesID, "com.example.c")),
		)),
	))
	got := indexOf(t, scType+"tcc.yaml", typeSettingsCatalog, doc)

	isTrue(t, got[tccAccess].ListMember, "the per-service collection is the member list")
	equal(t, 2, len(got[tccAccess].Members))
	equal(t, `{"`+tccFilesID+`":"com.example.c"}`, got[tccFiles].Members[0])
	absent(t, got, tccServices, "the outer element holds only inner collections, so it adds no member")
	absent(t, got, tccItemID)
	hasLen(t, got, 2)
}

func TestIndexAppleSimpleAndChoiceMembers(t *testing.T) {
	const (
		simpleList = "com.apple.example_domains"
		choiceList = "com.apple.example_modes"
		childKey   = "com.apple.example_modes_child"
	)
	doc := catalogPolicy("macOS", applePayload("com.apple.example",
		map[string]interface{}{
			"settingDefinitionId": simpleList,
			"simpleSettingCollectionValue": []interface{}{
				map[string]interface{}{"value": "b.example"},
				map[string]interface{}{"value": "a.example"},
				map[string]interface{}{"value": ""},
			},
		},
		map[string]interface{}{
			"settingDefinitionId": choiceList,
			"choiceSettingCollectionValue": []interface{}{
				map[string]interface{}{"value": choiceList + "_1", "children": []interface{}{simpleInstance(childKey, 3)}},
			},
		},
	))
	got := indexOf(t, scType+"m.yaml", typeSettingsCatalog, doc)

	equal(t, `"a.example"`, got[simpleList].Members[0])
	equal(t, 2, len(got[simpleList].Members), "an empty value adds no member")
	equal(t, `"`+choiceList+`_1"`, got[choiceList].Members[0])
	equal(t, "3", got[childKey].Value, "a choice option's children are still keys")
}

func TestIndexAppleSecretMemberIsUnknown(t *testing.T) {
	secret := map[string]interface{}{
		"@odata.type":         "#microsoft.graph.deviceManagementConfigurationSimpleSettingInstance",
		"settingDefinitionId": ruleValue,
		"simpleSettingValue": map[string]interface{}{
			"@odata.type": "#microsoft.graph.deviceManagementConfigurationSecretSettingValue",
			"value":       "resolved-member-secret",
			"valueState":  "notEncrypted",
		},
	}
	doc := catalogPolicy("macOS", applePayload(loginPayload,
		groupCollection(loginRules, loginRule("TEAMA"), children(secret)),
	))
	got := indexOf(t, scType+"m.yaml", typeSettingsCatalog, doc)

	rules := got[loginRules]
	isTrue(t, rules.UnknownMembers, "an element holding a secret is an unknown member")
	equal(t, 1, len(rules.Members), "an unknown member keeps no value")
	for _, m := range rules.Members {
		isTrue(t, !strings.Contains(m, "resolved-member-secret"))
	}
}
