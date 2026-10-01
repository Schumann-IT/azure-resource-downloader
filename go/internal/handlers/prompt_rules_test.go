package handlers

import (
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
