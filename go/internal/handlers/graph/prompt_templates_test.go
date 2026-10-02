package graph

import (
	"strings"
	"testing"
)

// TestSharedPromptTemplateOverrides verifies that one representative handler
// per shared template category renders its category-specific prompt instead of
// the default settings/assignments layout.
func TestSharedPromptTemplateOverrides(t *testing.T) {
	tests := []struct {
		name        string
		newHandler  func() (*GraphCollectionHandler, error)
		marker      string
		description string
	}{
		{
			name:        "singleton",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewOrganizationHandler(fakeTokenCredential{}) },
			marker:      "tenant-wide singleton",
			description: "organization uses the singleton template",
		},
		{
			name: "singleton organizationalBranding",
			newHandler: func() (*GraphCollectionHandler, error) {
				return NewOrganizationalBrandingHandler(fakeTokenCredential{})
			},
			marker:      "tenant-wide singleton",
			description: "organizationalBranding uses the singleton template",
		},
		{
			name:        "credential",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewVppTokenHandler(fakeTokenCredential{}) },
			marker:      "Expiry and renewal:",
			description: "vppTokens uses the credential template",
		},
		{
			name:        "record",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewDeviceCategoryHandler(fakeTokenCredential{}) },
			marker:      "inventory or registry record",
			description: "deviceCategories uses the record template",
		},
		{
			name:        "referenced",
			newHandler:  func() (*GraphCollectionHandler, error) { return NewAssignmentFilterHandler(fakeTokenCredential{}) },
			marker:      "Usage and references:",
			description: "assignmentFilters uses the referenced template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := tt.newHandler()
			if err != nil {
				t.Fatalf("constructor unexpected error: %v", err)
			}

			prompt := handler.GetDocumentationPrompt()

			if !strings.Contains(prompt, tt.marker) {
				t.Errorf("%s: prompt missing %q", tt.description, tt.marker)
			}
			if !strings.Contains(prompt, "Azure resource type: "+handler.GetType()) {
				t.Errorf("%s: prompt missing resource type line", tt.description)
			}
			if strings.Contains(prompt, "a table of any assignments/targeting present") {
				t.Errorf("%s: prompt unexpectedly contains default-template assignments text", tt.description)
			}
		})
	}
}

// TestCompliancePolicyPromptWrapsNotifications verifies both compliance-policy
// handlers instruct wrapping the noncompliance-notification reference in the
// splice markers, so a template rename can re-splice that block. Types that do
// not reference templates must not carry the instruction.
func TestCompliancePolicyPromptWrapsNotifications(t *testing.T) {
	referencing := []func() (*GraphCollectionHandler, error){
		func() (*GraphCollectionHandler, error) {
			return NewDeviceCompliancePolicyHandler(fakeTokenCredential{})
		},
		func() (*GraphCollectionHandler, error) { return NewCompliancePolicyHandler(fakeTokenCredential{}) },
	}
	for _, newHandler := range referencing {
		handler, err := newHandler()
		if err != nil {
			t.Fatalf("constructor unexpected error: %v", err)
		}
		prompt := handler.GetDocumentationPrompt()
		for _, want := range []string{
			"`<!-- notifications:start -->` / `<!-- notifications:end -->` markers",
			"notificationTemplateId",
		} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s: prompt missing %q", handler.GetType(), want)
			}
		}
	}

	// A non-referencing type reuses the default template without the marker.
	other, err := NewDeviceCategoryHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("constructor unexpected error: %v", err)
	}
	if strings.Contains(other.GetDocumentationPrompt(), "<!-- notifications:start -->") {
		t.Error("deviceCategories must not carry the notification-marker instruction")
	}
}

// TestConditionalAccessPromptTemplate verifies Conditional Access renders its
// own template: a Conditions section for the conditions.* targeting, the
// Conditional Access heading set, and no assignments instruction or markers.
func TestConditionalAccessPromptTemplate(t *testing.T) {
	handler, err := NewConditionalAccessPolicyHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("constructor unexpected error: %v", err)
	}
	prompt := handler.GetDocumentationPrompt()

	for _, want := range []string{
		"\nConditions:\n",
		"`Condition | Include | Exclude`",
		"\n<!-- doc-headings: References | Conditions | Lifecycle and operations | Security | Settings -->\n",
		"its targeting is documented in the `Conditions` section",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"a table of any assignments/targeting present",
		"assignment information belongs in the assignments block above",
		"<!-- assignments:",
		"Targeting — the users",
	} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("prompt unexpectedly contains %q", unwanted)
		}
	}
}

// TestReferencedPromptTemplateAssignments verifies the referenced template is
// used by reusablePolicySettings and roleScopeTags, and that only roleScopeTags
// — the one with an assignments concept — carries the assignments instruction.
func TestReferencedPromptTemplateAssignments(t *testing.T) {
	tests := []struct {
		name           string
		newHandler     func() (*GraphCollectionHandler, error)
		hasAssignments bool
	}{
		{
			name: "reusablePolicySettings",
			newHandler: func() (*GraphCollectionHandler, error) {
				return NewReusablePolicySettingHandler(fakeTokenCredential{})
			},
		},
		{
			name:           "roleScopeTags",
			newHandler:     func() (*GraphCollectionHandler, error) { return NewRoleScopeTagHandler(fakeTokenCredential{}) },
			hasAssignments: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := tt.newHandler()
			if err != nil {
				t.Fatalf("constructor unexpected error: %v", err)
			}
			prompt := handler.GetDocumentationPrompt()

			if !strings.Contains(prompt, "Usage and references:") {
				t.Error("prompt missing \"Usage and references:\"")
			}
			for _, marker := range []string{
				"a table of any assignments/targeting present",
				"assignment information belongs in the assignments block above",
			} {
				if got := strings.Contains(prompt, marker); got != tt.hasAssignments {
					t.Errorf("prompt contains %q = %v, want %v", marker, got, tt.hasAssignments)
				}
			}
			noOwnAssignments := strings.Contains(prompt, "it has no assignments of its own")
			if noOwnAssignments == tt.hasAssignments {
				t.Errorf("intro claims \"no assignments of its own\" = %v, want %v", noOwnAssignments, !tt.hasAssignments)
			}
		})
	}
}

// TestGroupPromptTemplateHeader verifies the group template renders the shared
// header — every curated link, including the Admin center, and related types —
// and the group heading set, References and Lifecycle and operations included.
func TestGroupPromptTemplateHeader(t *testing.T) {
	handler, err := NewGroupHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("constructor unexpected error: %v", err)
	}
	prompt := handler.GetDocumentationPrompt()

	for _, want := range []string{
		"- Admin center: https://entra.microsoft.com/",
		"- Required permissions: ",
		"Related resource types exported alongside this one",
		"`Targeted by` block",
		"\nReferences:\n",
		"\nLifecycle and operations:\n",
		"\n<!-- doc-headings: References | Membership | Usage as assignment target | Lifecycle and operations | Security | Properties -->\n",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	if strings.Contains(prompt, "likely purpose") {
		t.Error("prompt still asks for the group's likely purpose")
	}
}

// TestRecordPromptTemplateSecurity verifies the record template has a Security
// section like every other family: the change role and the exposed-credential
// call-out live there, the redaction rule points at it, the standard data-note
// wording applies, and the old Lifecycle-section variant is gone.
func TestRecordPromptTemplateSecurity(t *testing.T) {
	handler, err := NewDeviceCategoryHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("constructor unexpected error: %v", err)
	}
	prompt := handler.GetDocumentationPrompt()

	for _, want := range []string{
		"`«redacted — secret present in source»`",
		"call it out in the **Security** section as an exposed credential to rotate",
		"Add `data-note=\"security\"` when the property is one called out in the Security section",
		"\nSecurity:\n- name the permission needed to read this resource",
		"- call out any exposed credential found under the redaction rule below.",
		"\n<!-- doc-headings: References | Lifecycle and operations | Security | Properties -->",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"call it out in the **Lifecycle and operations** section",
		"(a record has no Security section)",
		"only on a block whose value you redacted",
	} {
		if strings.Contains(prompt, unwanted) {
			t.Errorf("prompt unexpectedly contains %q", unwanted)
		}
	}
}

// TestCredentialPromptTemplateHeadings verifies the credential heading set puts
// Expiry and renewal right after References, before Lifecycle and operations,
// and that the sections appear in that order in the instructions too.
func TestCredentialPromptTemplateHeadings(t *testing.T) {
	handler, err := NewVppTokenHandler(fakeTokenCredential{})
	if err != nil {
		t.Fatalf("constructor unexpected error: %v", err)
	}
	prompt := handler.GetDocumentationPrompt()

	const headings = "\n<!-- doc-headings: References | Expiry and renewal | Lifecycle and operations | Security | Properties -->\n"
	if !strings.Contains(prompt, headings) {
		t.Errorf("prompt missing %q", headings)
	}
	expiry, lifecycle := strings.Index(prompt, "\nExpiry and renewal:\n"), strings.Index(prompt, "\nLifecycle and operations:\n")
	if expiry < 0 || lifecycle < 0 || expiry > lifecycle {
		t.Errorf("Expiry and renewal (at %d) must precede Lifecycle and operations (at %d)", expiry, lifecycle)
	}
}
