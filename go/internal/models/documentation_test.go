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
			AdminCenter:     "https://intune.microsoft.com/#view/Microsoft_Intune_DeviceSettings/DevicesMenu/~/configuration",
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
		"Lifecycle and operations:\n",
		"Security:\n",
		"Settings:\n",
		"<!-- doc-headings: References | Lifecycle and operations | Security | Settings -->",
		DocumentationGroupsMarker(),
		"- document EVERY setting/property present in the YAML.",
		"collapsible HTML `<details>` block, collapsed by default",
		"externally decoded sidecar file",
		"group names are NOT resolved",
		"never invent values",
		"masked or redacted",
		"credential-shaped",
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
		{
			name:   "notification template instruction absent by default",
			mutate: func(*ResourceDocumentation) {},
			absent: []string{"<!-- notifications:start -->"},
		},
		{
			name:   "notification template instruction rendered when set",
			mutate: func(d *ResourceDocumentation) { d.ReferencesNotificationTemplates = true },
			present: []string{
				"scheduledActionsForRule[].scheduledActionConfigurations[].notificationTemplateId",
				"`<!-- notifications:start -->` / `<!-- notifications:end -->` markers",
			},
		},
		{
			name:    "doc-groups marker present by default",
			mutate:  func(*ResourceDocumentation) {},
			present: []string{DocumentationGroupsMarker()},
		},
		{
			name:   "doc-groups marker suppressed when OmitGroupAxes",
			mutate: func(d *ResourceDocumentation) { d.OmitGroupAxes = true },
			absent: []string{DocGroupsMarkerPrefix},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := fullDoc()
			tt.mutate(&doc)
			prompt := BuildDocumentationPrompt(doc)

			runPromptAssertions(t, prompt, tt.present, tt.absent)
		})
	}
}

func TestBuildDocumentationPromptTemplateOverride(t *testing.T) {
	doc := fullDoc()
	doc.Template = "Custom prompt for {{ .AzureType }} ({{ join .KeySettings \" / \" }})"

	got := BuildDocumentationPrompt(doc)
	// The custom template controls the body; the shared render path still appends
	// the doc-groups marker (the type did not opt out via OmitGroupAxes).
	want := "Custom prompt for Microsoft.Graph/deviceConfigurations (omaSettings / encrypted values)" +
		"\n\n" + DocumentationGroupsMarker()
	if got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

// promptPartialNames lists every shared partial of prompt_partials.tmpl.
var promptPartialNames = []string{
	"prompt-type", "prompt-subtype", "prompt-permissions", "prompt-lifecycle", "prompt-links",
	"prompt-related", "prompt-header", "prompt-key-settings", "prompt-embedded-payloads",
	"prompt-assignments", "prompt-purpose-rule", "prompt-references", "prompt-url-rule",
	"prompt-baseline-rule", "prompt-change-role", "prompt-referenced-ids", "prompt-settings-grouping",
	"prompt-masked-rule", "prompt-redaction-rule", "prompt-details-attrs", "prompt-complete-rule",
	"prompt-present-rule", "prompt-lifecycle-rule", "prompt-closed-set",
}

func TestBuildDocumentationPromptOverrideCallsEveryPartial(t *testing.T) {
	var calls strings.Builder
	for _, name := range promptPartialNames {
		calls.WriteString(`{{ template "` + name + `" . }}` + "\n")
	}

	tests := []struct {
		name    string
		doc     ResourceDocumentation
		present []string
	}{
		{
			name: "every field populated",
			doc:  fullDoc(),
			present: []string{
				"Azure resource type: Microsoft.Graph/deviceConfigurations",
				"About this resource type: A legacy Intune device configuration profile.",
				"Subtype guidance: Identify the concrete profile type from @odata.type first.",
				"Permissions required to read this resource type:\n- DeviceManagementConfiguration.Read.All",
				"Lifecycle notes for this resource type:\n- Superseded by the Settings Catalog; plan migration.",
				"Reference material for this resource type",
				"- API reference: https://learn.microsoft.com/en-us/graph/api/resources/intune-deviceconfig-deviceconfiguration?view=graph-rest-beta",
				"- Schema reference: https://learn.microsoft.com/en-us/graph/api/resources/schema",
				"- Required permissions: https://learn.microsoft.com/en-us/graph/permissions-reference",
				"- Best-practice baseline: https://learn.microsoft.com/en-us/mem/intune/protect/security-baselines",
				"Related resource types exported alongside this one",
				"- give particular attention to: omaSettings, encrypted values.",
				"- Use real, verifiable URLs;",
				"- Where a value is masked or redacted by the service",
				"**credential-shaped**",
				"call it out in the **Security** section as an exposed credential to rotate",
				"- Open each block as `<details data-setting=\"<exact YAML path>\">`",
				"- Do not omit a property this section covers;",
				"- Only describe what is actually present; never invent values.",
				"- Build on the lifecycle notes listed above",
				"These H2 headings are a closed set and a machine contract",
			},
		},
		{
			name:    "only the resource type",
			doc:     ResourceDocumentation{AzureType: "Microsoft.Test/things"},
			present: []string{"Azure resource type: Microsoft.Test/things", "These H2 headings are a closed set"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.doc.Template = calls.String()
			runPromptAssertions(t, BuildDocumentationPrompt(tt.doc), tt.present, nil)
		})
	}
}

func TestBuildDocumentationPromptOverrideWithoutPartials(t *testing.T) {
	doc := fullDoc()
	doc.Template = "Plain prompt for {{ .AzureType }}\n\n  indented line, no partials\n"

	got := BuildDocumentationPrompt(doc)
	// The partials add nothing to a template that does not call them.
	want := "Plain prompt for Microsoft.Graph/deviceConfigurations\n\n  indented line, no partials" +
		"\n\n" + DocumentationGroupsMarker()
	if got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestBuildDocumentationPromptDefaultTextAsOverride(t *testing.T) {
	doc := fullDoc()
	want := BuildDocumentationPrompt(doc)

	// The default text calls the partials; through the Template field it is
	// parsed with them and renders exactly like the built-in default.
	doc.Template = DefaultDocumentationPromptTemplate()
	if got := BuildDocumentationPrompt(doc); got != want {
		t.Errorf("default text as override renders differently:\n got %q\nwant %q", got, want)
	}
}

func TestBuildDocumentationPromptOverrideCannotChangePartialsOfNextType(t *testing.T) {
	const hijack = "HIJACKED closed-set paragraph"

	before := BuildDocumentationPrompt(fullDoc())

	first := fullDoc()
	first.Template = `{{ define "prompt-closed-set" }}` + hijack + `{{ end }}{{ template "prompt-closed-set" . }}`
	BuildDocumentationPrompt(first)

	// The next type rendered with the default template and the next override
	// both see the shared partials, not the first override's definition.
	if after := BuildDocumentationPrompt(fullDoc()); after != before {
		t.Errorf("default prompt changed after an override redefined a partial:\n got %q\nwant %q", after, before)
	}
	next := fullDoc()
	next.Template = `{{ template "prompt-closed-set" . }}`
	runPromptAssertions(t, BuildDocumentationPrompt(next),
		[]string{"These H2 headings are a closed set and a machine contract"}, []string{hijack})
}

func TestBuildDocumentationPromptOmitGroupAxes(t *testing.T) {
	doc := fullDoc()
	doc.Template = "Custom prompt for {{ .AzureType }}"
	doc.OmitGroupAxes = true

	got := BuildDocumentationPrompt(doc)
	if strings.Contains(got, DocGroupsMarkerPrefix) {
		t.Errorf("prompt unexpectedly contains doc-groups marker despite OmitGroupAxes: %q", got)
	}
}

func TestBuildDocumentationPromptInvalidTemplatePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for invalid template override")
		}
	}()

	doc := fullDoc()
	doc.Template = "{{ .AzureType " // unclosed action
	BuildDocumentationPrompt(doc)
}

// runPromptAssertions checks that the prompt contains every string in present
// and none of the strings in absent.
func runPromptAssertions(t *testing.T, prompt string, present, absent []string) {
	t.Helper()

	for _, want := range present {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	for _, unwanted := range absent {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("prompt unexpectedly contains %q", unwanted)
		}
	}
}

// TestPromptLinksPartialBytePinned pins every branch of the prompt-links partial
// byte for byte, including the Admin center line after the permissions line.
func TestPromptLinksPartialBytePinned(t *testing.T) {
	doc := fullDoc()
	doc.Template = `{{ template "prompt-links" . }}`

	got := BuildDocumentationPrompt(doc)
	want := "\n\nReference material for this resource type (treat these as authoritative; prefer them over recalled knowledge):" +
		"\n- API reference: " + doc.Links.EndpointDocs +
		"\n- Schema reference: " + doc.Links.SchemaReference +
		"\n- Required permissions: " + doc.Links.Permissions +
		"\n- Admin center: " + doc.Links.AdminCenter +
		"\n- Best-practice baseline: " + doc.Links.BestPractices[0]
	want += "\n\n" + DocumentationGroupsMarker()
	if got != want {
		t.Errorf("prompt-links = %q, want %q", got, want)
	}
}

// TestPromptLinksAdminCenterOnly renders a documentation with only AdminCenter
// set: the block must appear on that link alone.
func TestPromptLinksAdminCenterOnly(t *testing.T) {
	doc := ResourceDocumentation{
		AzureType: "Microsoft.Graph/example",
		Template:  `{{ template "prompt-links" . }}`,
		Links:     ResourceLinks{AdminCenter: "https://entra.microsoft.com/#view/example"},
	}

	got := BuildDocumentationPrompt(doc)
	want := "\n\nReference material for this resource type (treat these as authoritative; prefer them over recalled knowledge):" +
		"\n- Admin center: https://entra.microsoft.com/#view/example" +
		"\n\n" + DocumentationGroupsMarker()
	if got != want {
		t.Errorf("prompt-links = %q, want %q", got, want)
	}
}

// TestPromptClosedSetAssignmentsPointer verifies the closed-set paragraph
// points at the assignments block only for a type with an assignments concept.
func TestPromptClosedSetAssignmentsPointer(t *testing.T) {
	const pointer = "assignment information belongs in the assignments block above"
	for _, hasAssignments := range []bool{false, true} {
		doc := ResourceDocumentation{
			AzureType:      "Microsoft.Test/things",
			HasAssignments: hasAssignments,
			Template:       `{{ template "prompt-closed-set" . }}`,
		}
		prompt := BuildDocumentationPrompt(doc)
		if got := strings.Contains(prompt, pointer); got != hasAssignments {
			t.Errorf("HasAssignments=%v: closed-set contains the assignments pointer = %v", hasAssignments, got)
		}
		if !strings.Contains(prompt, "identifying fields belong in the metadata table above") {
			t.Errorf("HasAssignments=%v: closed-set lost the metadata-table pointer", hasAssignments)
		}
	}
}

// TestPromptLifecycleRule verifies the lifecycle rule builds on the curated
// lifecycle notes when the type has them, and otherwise confines the section to
// what the YAML shows and states that deprecation status is not documented.
func TestPromptLifecycleRule(t *testing.T) {
	const (
		withNotes    = "- Build on the lifecycle notes listed above and add only what the YAML itself shows."
		withoutNotes = "- State only what the YAML itself shows, and say that deprecation or migration status is not documented here."
	)
	tests := []struct {
		name      string
		lifecycle []string
		present   string
		absent    string
	}{
		{name: "with lifecycle notes", lifecycle: []string{"Superseded by the Settings Catalog."}, present: withNotes, absent: withoutNotes},
		{name: "without lifecycle notes", present: withoutNotes, absent: withNotes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := ResourceDocumentation{AzureType: "Microsoft.Test/things", Lifecycle: tt.lifecycle}
			runPromptAssertions(t, BuildDocumentationPrompt(doc), []string{"Lifecycle and operations:\n" + tt.present}, []string{tt.absent})

			doc.Template = `{{ template "prompt-lifecycle-rule" . }}`
			want := tt.present + "\n\n" + DocumentationGroupsMarker()
			if got := BuildDocumentationPrompt(doc); got != want {
				t.Errorf("prompt-lifecycle-rule = %q, want %q", got, want)
			}
		})
	}
}

// TestPromptEvidenceRules verifies the evidence-bound partials: the URL rule
// forbids guessed links, a recommended value is tied to a listed baseline (or
// ruled out when none is listed), and the embedded-payload line decodes only
// text and never reprints binary content.
func TestPromptEvidenceRules(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ResourceDocumentation)
		present []string
		absent  []string
	}{
		{
			name:    "URL rule gives no link rather than a guessed one",
			mutate:  func(*ResourceDocumentation) {},
			present: []string{"otherwise give it no link — never a guessed or merely nearby URL"},
			absent:  []string{"approximate"},
		},
		{
			name:    "baseline listed",
			mutate:  func(*ResourceDocumentation) {},
			present: []string{"only where a best-practice baseline listed above covers the setting"},
			absent:  []string{"No best-practice baseline is listed for this type"},
		},
		{
			name:    "no baseline listed",
			mutate:  func(d *ResourceDocumentation) { d.Links.BestPractices = nil },
			present: []string{"No best-practice baseline is listed for this type"},
			absent:  []string{"only where a best-practice baseline listed above covers the setting"},
		},
		{
			name:   "embedded payloads decoded only when text",
			mutate: func(*ResourceDocumentation) {},
			present: []string{
				"A payload the export already decoded (inline in the YAML, or as a sidecar file next to it) is documented as decoded.",
				"decoded and pretty-printed only when it is text",
				"binary content (certificates, images) is stated as present and never reprinted",
			},
		},
		{
			name:   "no review cadence and no inferred purpose",
			mutate: func(*ResourceDocumentation) {},
			present: []string{
				"never infer it from the display name",
				"otherwise state that the change role is not documented here",
			},
			absent: []string{"review cadence"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := fullDoc()
			tt.mutate(&doc)
			runPromptAssertions(t, BuildDocumentationPrompt(doc), tt.present, tt.absent)
		})
	}
}
