package models

import (
	"strings"
	"testing"
)

// fullDoc returns a ResourceDocumentation with every field populated.
func fullDoc() ResourceDocumentation {
	return ResourceDocumentation{
		AzureType:           "Microsoft.Graph/deviceConfigurations",
		Purpose:             "A legacy Intune device configuration profile.",
		KeySettings:         []string{"omaSettings", "encrypted values"},
		EmbeddedPayloads:    []string{"omaSettings (custom OMA-URI values)"},
		RequiredPermissions: []string{"DeviceManagementConfiguration.Read.All"},
		Lifecycle:           []string{"Superseded by the Settings Catalog; plan migration."},
		RelatedTypes:        []string{"Microsoft.Graph/groups (assignment target groups)"},
		SubtypeNote:         "Identify the concrete profile type from @odata.type first.",
		Links: ResourceLinks{
			EndpointDocs:    "https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-deviceconfiguration?view=graph-rest-beta",
			BestPractices:   []string{"https://learn.microsoft.com/en-us/mem/intune/protect/security-baselines"},
			SchemaReference: "https://learn.microsoft.com/en-us/graph/api/resources/schema",
			Permissions:     "https://learn.microsoft.com/en-us/graph/permissions-reference",
		},
	}
}

func TestBuildDocumentationPromptAlwaysPresent(t *testing.T) {
	prompt := BuildDocumentationPrompt(ResourceDocumentation{AzureType: "Microsoft.Test/things"})

	for _, want := range []string{
		"You are a senior Microsoft cloud and endpoint-management consultant.",
		"Azure resource type: Microsoft.Test/things",
		"- An H1 title set to the resource's display name.",
		"a short summary paragraph",
		"a metadata table stating the resource type",
		"a table of any assignments/targeting present",
		"Then the following H2 sections, unnumbered, in this order:",
		"References:\n",
		"Lifecycle & operations:\n",
		"Security:\n",
		"Settings:\n",
		"- document EVERY setting/property present in the YAML.",
		"collapsible HTML `<details>` block, collapsed by default",
		"externally decoded sidecar file",
		"group names are NOT resolved",
		"never invent values",
		"masked or redacted",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

func TestBuildDocumentationPromptOptionalFields(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceDocumentation)
		present []string
		absent  []string
	}{
		{
			name:   "all fields rendered",
			mutate: func(*ResourceDocumentation) {},
			present: []string{
				"About this resource type: A legacy Intune device configuration profile.",
				"Subtype guidance: Identify the concrete profile type from @odata.type first.",
				"Permissions required to read this resource type:\n- DeviceManagementConfiguration.Read.All\n",
				"Lifecycle notes for this resource type:\n- Superseded by the Settings Catalog; plan migration.\n",
				"Reference material for this resource type",
				"- API reference: https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-deviceconfiguration?view=graph-rest-beta",
				"- Schema reference: https://learn.microsoft.com/en-us/graph/api/resources/schema",
				"- Required permissions: https://learn.microsoft.com/en-us/graph/permissions-reference",
				"- Best-practice baseline: https://learn.microsoft.com/en-us/mem/intune/protect/security-baselines",
				"Related resource types exported alongside this one",
				"- Microsoft.Graph/groups (assignment target groups)",
				"This resource carries embedded or encoded payloads: omaSettings (custom OMA-URI values)",
				"- give particular attention to: omaSettings, encrypted values.\n",
			},
		},
		{
			name:   "purpose omitted",
			mutate: func(d *ResourceDocumentation) { d.Purpose = "" },
			absent: []string{"About this resource type:"},
		},
		{
			name:   "subtype note omitted",
			mutate: func(d *ResourceDocumentation) { d.SubtypeNote = "" },
			absent: []string{"Subtype guidance:"},
		},
		{
			name:   "permissions omitted",
			mutate: func(d *ResourceDocumentation) { d.RequiredPermissions = nil },
			absent: []string{"Permissions required to read this resource type:"},
		},
		{
			name:   "lifecycle omitted",
			mutate: func(d *ResourceDocumentation) { d.Lifecycle = nil },
			absent: []string{"Lifecycle notes for this resource type:"},
		},
		{
			name:   "links section omitted when empty",
			mutate: func(d *ResourceDocumentation) { d.Links = ResourceLinks{} },
			absent: []string{"Reference material for this resource type", "- API reference:"},
		},
		{
			name:   "related types omitted",
			mutate: func(d *ResourceDocumentation) { d.RelatedTypes = nil },
			absent: []string{"Related resource types exported alongside this one"},
		},
		{
			name:   "no embedded payloads omits decode instruction",
			mutate: func(d *ResourceDocumentation) { d.EmbeddedPayloads = nil },
			absent: []string{"This resource carries embedded or encoded payloads:"},
		},
		{
			name:   "key settings omitted",
			mutate: func(d *ResourceDocumentation) { d.KeySettings = nil },
			absent: []string{"give particular attention to:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := fullDoc()
			tt.mutate(&doc)
			prompt := BuildDocumentationPrompt(doc)

			for _, want := range tt.present {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt missing %q", want)
				}
			}
			for _, unwanted := range tt.absent {
				if strings.Contains(prompt, unwanted) {
					t.Errorf("prompt unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}
