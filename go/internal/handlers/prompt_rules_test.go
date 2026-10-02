package handlers

import (
	"regexp"
	"strings"
	"testing"

	"azure-resource-downloader/internal/models"
)

const (
	// assignmentsInstruction is the default/referenced templates' assignments
	// bullet (the prompt-assignments partial).
	assignmentsInstruction = "a table of any assignments/targeting present"
	// assignmentsPointer is the closed-set paragraph's pointer to that block.
	assignmentsPointer = "assignment information belongs in the assignments block above"
	// expirySection marks the credential template, the only one with an
	// Expiry and renewal section.
	expirySection = "Expiry and renewal:"
)

// TestDocumentationPromptEvidenceRules checks every registered type's rendered
// documentation prompt against the template rules: the assignments
// instruction and the closed-set pointer to it appear exactly when the type
// has an assignments concept, every declared embedded payload is rendered, no
// prompt invites approximate links or a search of sibling directories, and a
// review cadence is asked for only in the credential template's Expiry and
// renewal section.
func TestDocumentationPromptEvidenceRules(t *testing.T) {
	registry := NewRegistry(stubCredential{}, "sub-123", false)

	for _, resourceType := range registry.GetAllTypes() {
		t.Run(resourceType, func(t *testing.T) {
			handler, err := registry.Get(resourceType)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", resourceType, err)
			}
			prompt := handler.GetDocumentationPrompt()

			hasAssignments := false
			if capable, ok := handler.(models.AssignmentCapable); ok {
				hasAssignments = capable.HasAssignments()
			}
			documented, ok := handler.(models.Documented)
			if !ok {
				t.Fatalf("handler for %q does not implement models.Documented", resourceType)
			}
			doc := documented.Documentation()
			if doc.HasAssignments != hasAssignments {
				t.Errorf("Documentation().HasAssignments = %v, HasAssignments() = %v", doc.HasAssignments, hasAssignments)
			}

			for _, marker := range []string{assignmentsInstruction, assignmentsPointer} {
				if got := strings.Contains(prompt, marker); got != hasAssignments {
					t.Errorf("prompt contains %q = %v, want %v (HasAssignments)", marker, got, hasAssignments)
				}
			}

			if len(doc.EmbeddedPayloads) > 0 {
				want := "This resource carries embedded or encoded payloads: " + strings.Join(doc.EmbeddedPayloads, ", ")
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt does not render the declared embedded payloads %q", want)
				}
			}

			for _, unwanted := range []string{"approximate", "search sibling"} {
				if strings.Contains(prompt, unwanted) {
					t.Errorf("prompt unexpectedly contains %q", unwanted)
				}
			}

			isCredential := strings.Contains(prompt, expirySection)
			if got := strings.Contains(prompt, "review cadence"); got != isCredential {
				t.Errorf("prompt contains \"review cadence\" = %v, want %v (credential template)", got, isCredential)
			}
		})
	}
}

// browserHeadings is the closed vocabulary of H2 headings the documentation
// browser styles, copied (not imported) so the two projects stay decoupled:
// every heading a template declares must be one of them.
var browserHeadings = map[string]bool{
	"References":                 true,
	"Conditions":                 true,
	"Membership":                 true,
	"Usage as assignment target": true,
	"Usage and references":       true,
	"Expiry and renewal":         true,
	"Lifecycle and operations":   true,
	"Security":                   true,
	"Settings":                   true,
	"Properties":                 true,
	"Definition":                 true,
}

// settingsSections are the names a template's closing settings section may take.
var settingsSections = map[string]bool{"Settings": true, "Properties": true, "Definition": true}

// docHeadingsLine matches the doc-headings marker of an assembled prompt.
var docHeadingsLine = regexp.MustCompile(`(?m)^<!-- doc-headings: (.+) -->$`)

// docHeadings returns the ordered heading list of a prompt's doc-headings marker.
func docHeadings(t *testing.T, prompt string) []string {
	t.Helper()
	m := docHeadingsLine.FindStringSubmatch(prompt)
	if m == nil {
		t.Fatal("prompt has no doc-headings line")
	}
	headings := strings.Split(m[1], " | ")
	for i := range headings {
		headings[i] = strings.TrimSpace(headings[i])
	}
	return headings
}

// TestDocumentationPromptSectionShape checks every registered type's prompt
// against the one section order — References first, then the type-specific
// sections, then Lifecycle and operations, Security and the settings section
// last — and against the shared settings-section rules every family carries.
func TestDocumentationPromptSectionShape(t *testing.T) {
	const (
		baselineListed   = "- State a recommended or best-practice value only where a best-practice baseline listed above covers the setting"
		baselineUnlisted = "- No best-practice baseline is listed for this type"
		lifecycleNotes   = "- Build on the lifecycle notes listed above and add only what the YAML itself shows."
		lifecycleNone    = "- State only what the YAML itself shows, and say that deprecation or migration status is not documented here."
	)
	required := []string{
		"- Open each block as `<details data-setting=\"<exact YAML path>\">`",
		"`data-note=\"security\"` when the property is one called out in the Security section",
		"- Do not omit a property this section covers;",
		"- Only describe what is actually present; never invent values.",
		"- Where a value is masked or redacted by the service",
		"call it out in the **Security** section as an exposed credential to rotate",
	}
	registry := NewRegistry(stubCredential{}, "sub-123", false)

	types := registry.GetAllTypes()
	if len(types) == 0 {
		t.Fatal("registry has no types")
	}

	for _, resourceType := range types {
		t.Run(resourceType, func(t *testing.T) {
			handler, err := registry.Get(resourceType)
			if err != nil {
				t.Fatalf("Get(%q) error = %v", resourceType, err)
			}
			prompt := handler.GetDocumentationPrompt()

			headings := docHeadings(t, prompt)
			n := len(headings)
			if n < 4 {
				t.Fatalf("doc-headings %v: want at least References, Lifecycle and operations, Security and a settings section", headings)
			}
			if headings[0] != "References" {
				t.Errorf("doc-headings %v: first = %q, want References", headings, headings[0])
			}
			if !settingsSections[headings[n-1]] {
				t.Errorf("doc-headings %v: last = %q, want Settings, Properties or Definition", headings, headings[n-1])
			}
			if headings[n-2] != "Security" {
				t.Errorf("doc-headings %v: second to last = %q, want Security", headings, headings[n-2])
			}
			if headings[n-3] != "Lifecycle and operations" {
				t.Errorf("doc-headings %v: third to last = %q, want Lifecycle and operations", headings, headings[n-3])
			}
			for _, heading := range headings {
				if !browserHeadings[heading] {
					t.Errorf("doc-headings %v: %q is not a heading the browser styles", headings, heading)
				}
			}

			runRequired(t, prompt, required)
			baseline := baselineListed
			if !strings.Contains(prompt, baselineListed) {
				baseline = baselineUnlisted
			}
			assertRuleOrder(t, prompt, []string{
				"- Open each block as `<details data-setting=",
				"- Do not omit a property this section covers;",
				baseline,
				"- When this section would hold more than 30 top-level blocks",
				"- Document a referenced object",
				"- Only describe what is actually present;",
				"- Where a value is masked or redacted by the service",
				"**credential-shaped**",
			})
			if strings.Contains(prompt, baselineListed) == strings.Contains(prompt, baselineUnlisted) {
				t.Error("prompt must carry exactly one of the two baseline-rule variants")
			}
			if strings.Contains(prompt, lifecycleNotes) == strings.Contains(prompt, lifecycleNone) {
				t.Error("prompt must carry exactly one of the two lifecycle-rule variants")
			}
			for _, unwanted := range []string{"cross-reference their YAML directories", "(a record has no Security section)"} {
				if strings.Contains(prompt, unwanted) {
					t.Errorf("prompt unexpectedly contains %q", unwanted)
				}
			}
		})
	}
}

// runRequired reports every string of want that prompt does not contain.
func runRequired(t *testing.T, prompt string, want []string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt missing %q", w)
		}
	}
}

// assertRuleOrder reports every text of ordered that is absent from prompt or
// does not appear after the previous one, so the rules keep their order.
func assertRuleOrder(t *testing.T, prompt string, ordered []string) {
	t.Helper()
	prev, prevText := -1, ""
	for _, text := range ordered {
		idx := strings.Index(prompt, text)
		if idx < 0 {
			t.Errorf("prompt missing %q", text)
			continue
		}
		if idx <= prev {
			t.Errorf("rule %q must come after %q", text, prevText)
		}
		prev, prevText = idx, text
	}
}
