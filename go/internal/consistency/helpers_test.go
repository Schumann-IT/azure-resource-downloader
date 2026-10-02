package consistency

import (
	"os"
	"path/filepath"
	"testing"

	"azure-resource-downloader/internal/docs"

	"gopkg.in/yaml.v3"
)

// Synthetic ids used across the fixtures; none comes from a real tenant.
const (
	grpDevices = "11111111-1111-1111-1111-111111111111"
	grpUsers   = "22222222-2222-2222-2222-222222222222"
	grpAdmins  = "33333333-3333-3333-3333-333333333333"
	grpOther   = "44444444-4444-4444-4444-444444444444"
	fltMac     = "55555555-5555-5555-5555-555555555555"
	zeroFilter = "00000000-0000-0000-0000-000000000000"

	targetGroup   = "#microsoft.graph.groupAssignmentTarget"
	targetExclude = "#microsoft.graph.exclusionGroupAssignmentTarget"
	targetDevices = "#microsoft.graph.allDevicesAssignmentTarget"
	targetUsers   = "#microsoft.graph.allLicensedUsersAssignmentTarget"
)

// fixture builds a synthetic export in a temp directory.
type fixture struct {
	t    *testing.T
	dir  string
	meta docs.Metadata
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return &fixture{
		t:   t,
		dir: t.TempDir(),
		meta: docs.Metadata{
			GeneratedAt: "2026-01-01T00:00:00Z",
			Tenant:      "contoso.example.com",
			ToolVersion: "test",
			Run:         docs.RunMeta{Complete: true},
			Types:       map[string]docs.TypeMeta{},
			Resources:   map[string]docs.ResourceMeta{},
		},
	}
}

// add writes a resource YAML and its metadata entry.
func (f *fixture) add(key string, entry docs.ResourceMeta, doc map[string]interface{}) {
	f.t.Helper()
	entry.PresentInTenant = true
	f.addEntry(key, entry)
	if doc != nil {
		f.writeYAML(key, doc)
	}
}

func (f *fixture) addEntry(key string, entry docs.ResourceMeta) {
	f.meta.Resources[key] = entry
}

func (f *fixture) writeYAML(key string, doc interface{}) {
	f.t.Helper()
	data, err := yaml.Marshal(doc)
	if err != nil {
		f.t.Fatal(err)
	}
	f.writeRaw(key, data)
}

func (f *fixture) writeRaw(key string, data []byte) {
	f.t.Helper()
	p := filepath.Join(f.dir, "resources", filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		f.t.Fatal(err)
	}
}

// save writes resources/metadata.yaml.
func (f *fixture) save() string {
	f.t.Helper()
	data, err := yaml.Marshal(&f.meta)
	if err != nil {
		f.t.Fatal(err)
	}
	p := filepath.Join(f.dir, "resources", "metadata.yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0644); err != nil {
		f.t.Fatal(err)
	}
	return f.dir
}

// include, exclude and the built-in targets build raw assignment targets as
// metadata records them.
func include(group string) interface{} {
	return map[string]interface{}{"target": map[string]interface{}{"@odata.type": targetGroup, "groupId": group}}
}

func includeFiltered(group, filter, mode string) interface{} {
	return map[string]interface{}{"target": map[string]interface{}{
		"@odata.type": targetGroup, "groupId": group,
		"deviceAndAppManagementAssignmentFilterId":   filter,
		"deviceAndAppManagementAssignmentFilterType": mode,
	}}
}

func exclude(group string) interface{} {
	return map[string]interface{}{"target": map[string]interface{}{"@odata.type": targetExclude, "groupId": group}}
}

func allDevices() interface{} {
	return map[string]interface{}{"target": map[string]interface{}{
		"@odata.type": targetDevices,
		"deviceAndAppManagementAssignmentFilterId": zeroFilter,
	}}
}

func allUsers() interface{} {
	return map[string]interface{}{"target": map[string]interface{}{"@odata.type": targetUsers}}
}

func targets(ts ...interface{}) []interface{} { return ts }

// addGroup adds a group entry; a non-empty rule makes it dynamic.
func (f *fixture) addGroup(name, id, rule string) {
	entry := docs.ResourceMeta{ResourceId: id, DisplayName: name}
	doc := map[string]interface{}{"id": id, "displayName": name}
	if rule != "" {
		entry.GroupTypes = []string{"DynamicMembership"}
		doc["membershipRule"] = rule
	}
	f.add("Microsoft.Graph/groups/"+name+".yaml", entry, doc)
}

// catalogChoice builds a Settings Catalog choice setting.
func catalogChoice(id, option string, children ...interface{}) interface{} {
	if children == nil {
		children = []interface{}{}
	}
	return map[string]interface{}{"settingInstance": map[string]interface{}{
		"@odata.type":         "#microsoft.graph.deviceManagementConfigurationChoiceSettingInstance",
		"settingDefinitionId": id,
		"choiceSettingValue":  map[string]interface{}{"value": option, "children": children},
	}}
}

// catalogSimple builds a Settings Catalog simple setting.
func catalogSimple(id string, value interface{}) interface{} {
	return map[string]interface{}{"settingInstance": simpleInstance(id, value)}
}

func simpleInstance(id string, value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"@odata.type":         "#microsoft.graph.deviceManagementConfigurationSimpleSettingInstance",
		"settingDefinitionId": id,
		"simpleSettingValue": map[string]interface{}{
			"@odata.type": "#microsoft.graph.deviceManagementConfigurationIntegerSettingValue",
			"value":       value,
		},
	}
}

func catalogPolicy(platform string, settings ...interface{}) map[string]interface{} {
	return map[string]interface{}{"id": "p", "name": "p", "platforms": platform, "settings": settings}
}

// indexOf returns a resource's settings by key.
func indexOf(t *testing.T, resource, sourceType string, doc map[string]interface{}) map[string]Setting {
	t.Helper()
	out := map[string]Setting{}
	for _, s := range indexResource(resource, sourceType, doc, nil, t.TempDir()) {
		out[s.Key] = s
	}
	return out
}

func equal[T comparable](t *testing.T, want, got T, msg ...interface{}) {
	t.Helper()
	if want != got {
		t.Errorf("want %v, got %v %v", want, got, msg)
	}
}

func empty(t *testing.T, got string, msg ...interface{}) {
	t.Helper()
	if got != "" {
		t.Errorf("want empty, got %q %v", got, msg)
	}
}

func isTrue(t *testing.T, got bool, msg ...interface{}) {
	t.Helper()
	if !got {
		t.Errorf("want true %v", msg)
	}
}

func absent(t *testing.T, m map[string]Setting, key string, msg ...interface{}) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Errorf("%q must not be indexed %v", key, msg)
	}
}

func hasLen(t *testing.T, m map[string]Setting, n int, msg ...interface{}) {
	t.Helper()
	if len(m) != n {
		t.Errorf("want %d settings, got %d %v", n, len(m), msg)
	}
}
